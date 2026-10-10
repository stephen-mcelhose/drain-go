package drain

import (
	"container/list"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type compiledMask struct {
	re          *regexp.Regexp
	name        string
	replacement string
}

// Parser is safe for concurrent calls. Train learns templates; Match only classifies.
type Parser struct {
	mu                 sync.RWMutex
	leadingTokens      int
	maxNodeDepth       int
	maxChildren        int
	similarity         float64
	maxClusters        int
	parametrizeNumeric bool
	extraDelimiters    []string
	masks              []compiledMask
	root               *node
	clusters           map[int]*Cluster
	lru                *list.List
	lruIndex           map[int]*list.Element
	nextID             int
}

const (
	DefaultLeadingTokens       = 1
	DefaultSimilarityThreshold = 0.4
	DefaultMaxChildren         = 100
)

var (
	ErrDrainLeadingTokensMustBeNonNegative = errors.New("drain: leading tokens must be non-negative")
	ErrDrainSimilarityThresholdConstraints = errors.New("drain: similarity threshold must be in [0, 1]")
	ErrMaxChildrenContraints               = errors.New("drain: max children must be positive")
	ErrMaskNameConstraints                 = errors.New("drain: mask name cannot be empty")
)

// NewReplacement constructs a replacement Tag
func NewReplacement(name string) string {
	return fmt.Sprintf("<%s>", name)
}

// New constructs a streaming Drain parser.
func New(cfg Config) (*Parser, error) {
	if cfg.LeadingTokens == 0 {
		cfg.LeadingTokens = DefaultLeadingTokens
	}
	if cfg.LeadingTokens < 0 {
		return nil, ErrDrainLeadingTokensMustBeNonNegative
	}
	if cfg.SimilarityThreshold == 0 {
		cfg.SimilarityThreshold = DefaultSimilarityThreshold
	}
	if cfg.SimilarityThreshold < 0 || cfg.SimilarityThreshold > 1 {
		return nil, ErrDrainSimilarityThresholdConstraints
	}
	if cfg.MaxChildren == 0 {
		cfg.MaxChildren = DefaultMaxChildren
	}
	if cfg.MaxChildren < 1 {
		return nil, ErrMaxChildrenContraints
	}

	parametrize := true
	if cfg.ParametrizeNumericTokens != nil {
		parametrize = *cfg.ParametrizeNumericTokens
	}

	masks := make([]compiledMask, 0, len(cfg.Masks))
	for _, m := range cfg.Masks {
		if m.Name == "" {
			return nil, ErrMaskNameConstraints
		}
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			return nil, err
		}
		masks = append(masks, compiledMask{re: re, name: m.Name, replacement: NewReplacement(m.Name)})
	}

	return &Parser{
		leadingTokens:      cfg.LeadingTokens,
		maxNodeDepth:       cfg.LeadingTokens + 1,
		maxChildren:        cfg.MaxChildren,
		similarity:         cfg.SimilarityThreshold,
		maxClusters:        cfg.MaxClusters,
		parametrizeNumeric: parametrize,
		extraDelimiters:    append([]string(nil), cfg.ExtraDelimiters...),
		masks:              masks,
		root:               newNode(),
		clusters:           make(map[int]*Cluster),
		lru:                list.New(),
		lruIndex:           make(map[int]*list.Element),
	}, nil
}

// Train classifies line, creating or generalizing a template when necessary.
func (p *Parser) Train(line string) Result {
	p.mu.Lock()
	defer p.mu.Unlock()

	tokens := p.tokenize(line)
	c := p.treeSearch(tokens, p.similarity, false)
	change := NoChange

	if c == nil {
		p.nextID++
		c = &Cluster{ID: p.nextID, Size: 1, Template: append([]string(nil), tokens...)}
		p.evictIfNeeded()
		p.clusters[c.ID] = c
		p.touch(c.ID)
		p.addToTree(c)
		change = ClusterCreated
	} else {
		updated := makeTemplate(tokens, c.Template)
		if !sameTokens(updated, c.Template) {
			c.Template = updated
			change = ClusterTemplateChanged
		}
		c.Size++
		p.touch(c.ID)
	}

	return p.result(c, change)
}

// Match classifies line against learned templates without changing parser state.
// By default it requires a perfect template match using FullSearchFallback.
// Use MatchOption values to adjust the search strategy or similarity threshold:
//
//	r, ok := parser.Match(line, drain.WithStrategy(drain.TreeOnly))
//	r, ok := parser.Match(line, drain.WithSimilarityThreshold(0.7))
func (p *Parser) Match(line string, opts ...MatchOption) (Result, bool) {
	cfg := defaultMatchConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return p.matchWith(line, cfg.strategy, cfg.threshold)
}

// MatchWithStrategy runs inference-only matching with an explicit search strategy.
//
// Deprecated: Use Match with WithStrategy instead:
//
//	parser.Match(line, drain.WithStrategy(strategy))
func (p *Parser) MatchWithStrategy(line string, strategy SearchStrategy) (Result, bool) {
	return p.Match(line, WithStrategy(strategy))
}

// matchWith is the internal implementation shared by Match and MatchWithStrategy.
func (p *Parser) matchWith(line string, strategy SearchStrategy, threshold float64) (Result, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tokens := p.tokenize(line)
	var c *Cluster

	switch strategy {
	case FullSearchAlways:
		c = p.fullSearch(tokens, threshold)
	default:
		c = p.treeSearch(tokens, threshold, true)
		if c == nil && strategy == FullSearchFallback {
			c = p.fullSearch(tokens, threshold)
		}
	}

	if c == nil {
		return Result{}, false
	}
	return p.result(c, NoChange), true
}

// Clusters returns stable snapshots ordered by ID.
func (p *Parser) Clusters() []Cluster {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]Cluster, 0, len(p.clusters))
	for _, c := range p.clusters {
		out = append(out, Cluster{c.ID, c.Size, append([]string(nil), c.Template...)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (p *Parser) result(c *Cluster, change ChangeType) Result {
	return Result{
		ClusterID:    c.ID,
		Size:         c.Size,
		ClusterCount: len(p.clusters),
		Template:     strings.Join(c.Template, " "),
		Tokens:       append([]string(nil), c.Template...),
		Change:       change,
	}
}

func (p *Parser) tokenize(s string) []string {
	for _, mask := range p.masks {
		s = mask.re.ReplaceAllString(s, mask.replacement)
	}
	for _, delimiter := range p.extraDelimiters {
		s = strings.ReplaceAll(s, delimiter, " ")
	}
	return strings.Fields(s)
}

// tokenizeRaw applies extra delimiters but skips mask substitution, giving
// the original tokens for alignment in ExtractParameters.
func (p *Parser) tokenizeRaw(s string) []string {
	for _, delimiter := range p.extraDelimiters {
		s = strings.ReplaceAll(s, delimiter, " ")
	}
	return strings.Fields(s)
}

func isVariable(token string) bool {
	return len(token) > 2 && token[0] == '<' && token[len(token)-1] == '>'
}

func variableName(token string) string {
	return token[1 : len(token)-1]
}

// ExtractParameters recovers the raw values that filled each variable position
// in template when it was matched against line.
//
// template is typically [Result.Tokens] from a prior [Parser.Train] or
// [Parser.Match] call. The second return value is false when the token count
// of line (after applying extra delimiters) does not equal len(template),
// which can happen if a mask regex matches a span that contains spaces.
//
// Only variable positions — wildcards ([Wildcard]) and named masks — appear
// in the returned slice; static tokens are skipped.
//
// ExtractParameters is safe to call concurrently with Train and Match.
func (p *Parser) ExtractParameters(template []string, line string) ([]ExtractedParameter, bool) {
	p.mu.RLock()
	raw := p.tokenizeRaw(line)
	p.mu.RUnlock()

	if len(raw) != len(template) {
		return nil, false
	}

	var out []ExtractedParameter
	for i, tok := range template {
		if isVariable(tok) {
			out = append(out, ExtractedParameter{
				Value: raw[i],
				Name:  variableName(tok),
			})
		}
	}
	return out, true
}

func (p *Parser) evictIfNeeded() {
	if p.maxClusters <= 0 || len(p.clusters) < p.maxClusters {
		return
	}
	if e := p.lru.Back(); e != nil {
		id := e.Value.(int)
		delete(p.clusters, id)
		delete(p.lruIndex, id)
		p.lru.Remove(e)
	}
}

func (p *Parser) touch(id int) {
	if e := p.lruIndex[id]; e != nil {
		p.lru.MoveToFront(e)
		return
	}
	p.lruIndex[id] = p.lru.PushFront(id)
}

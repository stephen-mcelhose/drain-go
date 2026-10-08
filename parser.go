package drain

import (
	"container/list"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type compiledMask struct {
	re          *regexp.Regexp
	replacement string
}

// Parser is safe for concurrent calls. Train learns templates; Match only classifies.
type Parser struct {
	mu                 sync.RWMutex
	depth              int
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

// New constructs a streaming Drain parser.
func New(cfg Config) (*Parser, error) {
	if cfg.Depth == 0 {
		cfg.Depth = 4
	}
	if cfg.Depth < 3 {
		return nil, errors.New("drain: depth must be at least 3")
	}
	if cfg.SimilarityThreshold == 0 {
		cfg.SimilarityThreshold = 0.4
	}
	if cfg.SimilarityThreshold < 0 || cfg.SimilarityThreshold > 1 {
		return nil, errors.New("drain: similarity threshold must be in [0, 1]")
	}
	if cfg.MaxChildren == 0 {
		cfg.MaxChildren = 100
	}
	if cfg.MaxChildren < 1 {
		return nil, errors.New("drain: max children must be positive")
	}

	parametrize := true
	if cfg.ParametrizeNumericTokens != nil {
		parametrize = *cfg.ParametrizeNumericTokens
	}

	masks := make([]compiledMask, 0, len(cfg.Masks))
	for _, m := range cfg.Masks {
		if m.Name == "" {
			return nil, errors.New("drain: mask name cannot be empty")
		}
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			return nil, err
		}
		masks = append(masks, compiledMask{re, "<" + m.Name + ">"})
	}

	return &Parser{
		depth:              cfg.Depth,
		maxNodeDepth:       cfg.Depth - 2,
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
// It requires a perfect match when wildcard fields are included in similarity.
func (p *Parser) Match(line string) (Result, bool) {
	return p.MatchWithStrategy(line, FullSearchFallback)
}

// MatchWithStrategy runs inference-only matching. FullSearchFallback is a safe default;
// it preserves tree speed in normal cases but scans same-length clusters after a miss.
func (p *Parser) MatchWithStrategy(line string, strategy SearchStrategy) (Result, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tokens := p.tokenize(line)
	var c *Cluster

	switch strategy {
	case FullSearchAlways:
		c = p.fullSearch(tokens, 1)
	default:
		c = p.treeSearch(tokens, 1, true)
		if c == nil && strategy == FullSearchFallback {
			c = p.fullSearch(tokens, 1)
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

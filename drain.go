// Package drain implements Drain, a fixed-depth online log-template miner.
//
// It is based on the algorithm described in:
// P. He, J. Zhu, Z. Zheng, and M. R. Lyu, "Drain: An Online Log Parsing
// Approach with Fixed Depth Tree", ICWS 2017.
package drain

// Wildcard is the default template token used for variable fields.
const Wildcard = "<*>"

// Config controls the parser. Zero-valued fields receive documented defaults.
type Config struct {
	// LeadingTokens is the number of leading message tokens used as routing
	// keys in the parse tree. Must be non-negative; the default is 1.
	// Higher values narrow candidates faster but require more tree memory.
	LeadingTokens int
	// SimilarityThreshold is the fraction of equal, non-wildcard tokens required
	// to join an existing cluster. The default is 0.4.
	SimilarityThreshold float64
	// MaxChildren bounds branching at a token node. The default is 100.
	MaxChildren int
	// MaxClusters bounds retained clusters using LRU eviction. Zero means unlimited.
	MaxClusters int
	// ExtraDelimiters are additional strings treated as token separators.
	ExtraDelimiters []string
	// ParametrizeNumericTokens routes tokens containing digits through the
	// wildcard branch. Defaults to true when nil.
	//
	// To explicitly disable, pass a pointer to false:
	//
	//	f := false
	//	cfg.ParametrizeNumericTokens = &f
	ParametrizeNumericTokens *bool
	// Masks are ordered RE2 patterns replaced by <Name> before tokenization.
	Masks []Mask
}

// Mask replaces a regular-expression pattern with a named placeholder before tokenization.
type Mask struct {
	// Name is used as the placeholder: <Name>.
	Name string `json:"name"`
	// Pattern is a Go RE2 regular expression.
	Pattern string `json:"pattern"`
}

// ChangeType describes how a training call affected the cluster set.
type ChangeType string

const (
	ClusterCreated         ChangeType = "cluster_created"
	ClusterTemplateChanged ChangeType = "cluster_template_changed"
	NoChange               ChangeType = "none"
)

// Result is returned by Train and Match.
type Result struct {
	// ClusterID is the unique identifier of the matched or created cluster.
	ClusterID int
	// Size is the number of training messages absorbed by the cluster so far.
	Size int
	// ClusterCount is the total number of live clusters in the parser at the
	// time of the call.
	ClusterCount int
	// Template is the cluster's current template as a space-joined string,
	// with variable positions represented by the Wildcard constant.
	Template string
	// Tokens is the same template as a string slice. It is a copy; mutations
	// do not affect the parser.
	Tokens []string
	// Change describes how this call affected the cluster.
	Change ChangeType
}

// Cluster is a learned log-event template. Values returned by Clusters are
// deep copies; callers may freely mutate the Template slice.
type Cluster struct {
	ID       int
	Size     int
	Template []string
}

// SearchStrategy controls recovery from a parse-tree false negative in Match.
type SearchStrategy uint8

const (
	TreeOnly           SearchStrategy = iota
	FullSearchFallback                // safe default: tree first, linear scan on miss
	FullSearchAlways                  // always scan all same-length clusters
)

// MatchOption configures a single Match call.
//
// Available options:
//   - [WithStrategy] — choose the tree search strategy (default: FullSearchFallback)
//   - [WithSimilarityThreshold] — set the minimum similarity for acceptance (default: 1.0)
type MatchOption func(*matchConfig)

type matchConfig struct {
	strategy  SearchStrategy
	threshold float64
}

func defaultMatchConfig() matchConfig {
	return matchConfig{strategy: FullSearchFallback, threshold: 1.0}
}

// WithStrategy sets the tree-search strategy for the Match call.
// The default is FullSearchFallback.
func WithStrategy(s SearchStrategy) MatchOption {
	return func(c *matchConfig) { c.strategy = s }
}

// WithSimilarityThreshold sets the minimum fraction of matching tokens required
// to accept a cluster during inference. The default is 1.0 (exact template
// match, with wildcard positions counting as matches). Values below 1.0 allow
// soft matching at inference time.
func WithSimilarityThreshold(t float64) MatchOption {
	return func(c *matchConfig) { c.threshold = t }
}

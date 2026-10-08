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
	// Depth includes the root and token-count levels. It must be at least 3.
	// The default (4) indexes the first two message tokens.
	Depth int
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
	ParametrizeNumericTokens *bool
	// Masks are ordered RE2 patterns replaced by <Name> before tokenization.
	Masks []Mask
}

// Mask replaces a regular-expression pattern with a named placeholder before tokenization.
type Mask struct {
	// Name is used as the placeholder: <Name>.
	Name string
	// Pattern is a Go RE2 regular expression.
	Pattern string
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
	ClusterID    int
	Size         int
	ClusterCount int
	Template     string
	Tokens       []string
	Change       ChangeType
}

// Cluster is a learned log event. Treat returned fields as read-only.
type Cluster struct {
	ID       int
	Size     int
	Template []string
}

// SearchStrategy controls recovery from a parse-tree false negative.
type SearchStrategy uint8

const (
	TreeOnly           SearchStrategy = iota
	FullSearchFallback                // safe default: tree first, linear scan on miss
	FullSearchAlways                  // always scan all same-length clusters
)

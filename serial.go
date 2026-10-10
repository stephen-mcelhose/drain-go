package drain

import (
	"container/list"
	"encoding/json"
	"fmt"
)

const serializationVersion = 1

// serializedParser is the on-disk/wire representation produced by Serialize.
type serializedParser struct {
	Version  int              `json:"version"`
	Config   serializedConfig `json:"config"`
	NextID   int              `json:"next_id"`
	Clusters []serialCluster  `json:"clusters"`
	Tree     *serialNode      `json:"tree"`
	// LRUOrder lists cluster IDs from most-recently-used (front) to
	// least-recently-used (back). Load restores eviction order exactly.
	LRUOrder []int `json:"lru_order"`
}

type serializedConfig struct {
	LeadingTokens            int      `json:"leading_tokens"`
	SimilarityThreshold      float64  `json:"similarity_threshold"`
	MaxChildren              int      `json:"max_children"`
	MaxClusters              int      `json:"max_clusters"`
	ExtraDelimiters          []string `json:"extra_delimiters,omitempty"`
	ParametrizeNumericTokens bool     `json:"parametrize_numeric_tokens"`
	Masks                    []Mask   `json:"masks,omitempty"`
}

type serialCluster struct {
	ID       int      `json:"id"`
	Size     int      `json:"size"`
	Template []string `json:"template"`
}

type serialNode struct {
	Children   map[string]*serialNode `json:"children,omitempty"`
	ClusterIDs []int                  `json:"cluster_ids,omitempty"`
}

// Serialize encodes the full parser state — clusters, tree, LRU order, and
// configuration — as a versioned JSON byte slice. The result can be stored in
// any byte-oriented sink and restored with Load.
//
// Serialize is safe to call concurrently with Train and Match.
func (p *Parser) Serialize() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	clusters := make([]serialCluster, 0, len(p.clusters))
	for _, c := range p.clusters {
		clusters = append(clusters, serialCluster{
			ID:       c.ID,
			Size:     c.Size,
			Template: append([]string(nil), c.Template...),
		})
	}

	lruOrder := make([]int, 0, p.lru.Len())
	for e := p.lru.Front(); e != nil; e = e.Next() {
		lruOrder = append(lruOrder, e.Value.(int))
	}

	sp := serializedParser{
		Version: serializationVersion,
		Config: serializedConfig{
			LeadingTokens:            p.leadingTokens,
			SimilarityThreshold:      p.similarity,
			MaxChildren:              p.maxChildren,
			MaxClusters:              p.maxClusters,
			ExtraDelimiters:          append([]string(nil), p.extraDelimiters...),
			ParametrizeNumericTokens: p.parametrizeNumeric,
			Masks:                    masksFromCompiled(p.masks),
		},
		NextID:   p.nextID,
		Clusters: clusters,
		Tree:     serializeNode(p.root, p.clusters),
		LRUOrder: lruOrder,
	}

	return json.Marshal(sp)
}

// Load restores a Parser from data produced by Serialize. The returned parser
// is immediately usable; subsequent Train calls continue assigning cluster IDs
// from where the original left off, and LRU eviction order is preserved.
func Load(data []byte) (*Parser, error) {
	var sp serializedParser
	if err := json.Unmarshal(data, &sp); err != nil {
		return nil, fmt.Errorf("drain: unmarshal failed: %w", err)
	}
	if sp.Version != serializationVersion {
		return nil, fmt.Errorf("drain: unsupported serialization version %d (want %d)",
			sp.Version, serializationVersion)
	}

	// Reconstruct via New so config validation is re-run.
	b := sp.Config.ParametrizeNumericTokens
	cfg := Config{
		LeadingTokens:            sp.Config.LeadingTokens,
		SimilarityThreshold:      sp.Config.SimilarityThreshold,
		MaxChildren:              sp.Config.MaxChildren,
		MaxClusters:              sp.Config.MaxClusters,
		ExtraDelimiters:          append([]string(nil), sp.Config.ExtraDelimiters...),
		ParametrizeNumericTokens: &b,
		Masks:                    append([]Mask(nil), sp.Config.Masks...),
	}
	p, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("drain: invalid serialized config: %w", err)
	}

	// Restore clusters.
	p.clusters = make(map[int]*Cluster, len(sp.Clusters))
	for _, sc := range sp.Clusters {
		p.clusters[sc.ID] = &Cluster{
			ID:       sc.ID,
			Size:     sc.Size,
			Template: append([]string(nil), sc.Template...),
		}
	}
	p.nextID = sp.NextID

	// Restore tree.
	if sp.Tree != nil {
		p.root = deserializeNode(sp.Tree)
	}

	// Restore LRU order (front = MRU, back = LRU).
	p.lru = list.New()
	p.lruIndex = make(map[int]*list.Element, len(sp.LRUOrder))
	for _, id := range sp.LRUOrder {
		if _, ok := p.clusters[id]; ok {
			e := p.lru.PushBack(id)
			p.lruIndex[id] = e
		}
	}

	return p, nil
}

// serializeNode converts a tree node into its serializable form, filtering out
// cluster IDs that have already been evicted from p.clusters.
func serializeNode(n *node, clusters map[int]*Cluster) *serialNode {
	if n == nil {
		return nil
	}
	sn := &serialNode{}
	for _, id := range n.clusters {
		if clusters[id] != nil {
			sn.ClusterIDs = append(sn.ClusterIDs, id)
		}
	}
	for k, child := range n.children {
		sc := serializeNode(child, clusters)
		if sc != nil {
			if sn.Children == nil {
				sn.Children = make(map[string]*serialNode)
			}
			sn.Children[k] = sc
		}
	}
	if len(sn.ClusterIDs) == 0 && len(sn.Children) == 0 {
		return nil
	}
	return sn
}

func deserializeNode(sn *serialNode) *node {
	n := newNode()
	if len(sn.ClusterIDs) > 0 {
		n.clusters = append([]int(nil), sn.ClusterIDs...)
	}
	for k, child := range sn.Children {
		n.children[k] = deserializeNode(child)
	}
	return n
}

// masksFromCompiled recovers the original Mask values from compiled masks.
func masksFromCompiled(masks []compiledMask) []Mask {
	out := make([]Mask, len(masks))
	for i, m := range masks {
		out[i] = Mask{Pattern: m.re.String(), Name: m.name}
	}
	return out
}

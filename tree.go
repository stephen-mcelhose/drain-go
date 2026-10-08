package drain

import "strconv"

type node struct {
	children map[string]*node
	clusters []int
}

func newNode() *node {
	return &node{children: make(map[string]*node)}
}

func (p *Parser) treeSearch(tokens []string, threshold float64, includeParams bool) *Cluster {
	cur := p.root.children[strconv.Itoa(len(tokens))]
	if cur == nil {
		return nil
	}
	if len(tokens) == 0 && len(cur.clusters) != 0 {
		return p.clusters[cur.clusters[0]]
	}
	for i, token := range tokens {
		if i+1 >= p.maxNodeDepth || i+1 >= len(tokens) {
			break
		}
		next := cur.children[token]
		if next == nil {
			next = cur.children[Wildcard]
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return p.fastMatch(cur.clusters, tokens, threshold, includeParams)
}

func (p *Parser) fullSearch(tokens []string, threshold float64) *Cluster {
	ids := make([]int, 0)
	for _, c := range p.clusters {
		if len(c.Template) == len(tokens) {
			ids = append(ids, c.ID)
		}
	}
	return p.fastMatch(ids, tokens, threshold, true)
}

func (p *Parser) fastMatch(ids []int, tokens []string, threshold float64, includeParams bool) *Cluster {
	var best *Cluster
	bestSimilarity, bestParams := -1.0, -1

	for _, id := range ids {
		c := p.clusters[id]
		if c == nil {
			continue
		}
		similarity, params := sequenceDistance(c.Template, tokens, includeParams)
		if similarity > bestSimilarity || (similarity == bestSimilarity && params > bestParams) {
			best, bestSimilarity, bestParams = c, similarity, params
		}
	}

	if bestSimilarity >= threshold {
		return best
	}
	return nil
}

func (p *Parser) addToTree(c *Cluster) {
	countKey := strconv.Itoa(len(c.Template))
	cur := p.root.children[countKey]
	if cur == nil {
		cur = newNode()
		p.root.children[countKey] = cur
	}

	if len(c.Template) == 0 {
		cur.clusters = []int{c.ID}
		return
	}

	for i, token := range c.Template {
		if i+1 >= p.maxNodeDepth || i+1 >= len(c.Template) {
			cur.clusters = appendLive(cur.clusters, p.clusters)
			cur.clusters = append(cur.clusters, c.ID)
			return
		}

		next := cur.children[token]
		if next == nil {
			if p.parametrizeNumeric && hasDigit(token) {
				next = cur.children[Wildcard]
				if next == nil {
					next = newNode()
					cur.children[Wildcard] = next
				}
			} else if wildcard := cur.children[Wildcard]; wildcard != nil {
				if len(cur.children) < p.maxChildren {
					next = newNode()
					cur.children[token] = next
				} else {
					next = wildcard
				}
			} else if len(cur.children)+1 < p.maxChildren {
				next = newNode()
				cur.children[token] = next
			} else {
				next = newNode()
				cur.children[Wildcard] = next
			}
		}
		cur = next
	}
}

// appendLive filters out cluster IDs that have been evicted.
func appendLive(ids []int, clusters map[int]*Cluster) []int {
	out := ids[:0]
	for _, id := range ids {
		if clusters[id] != nil {
			out = append(out, id)
		}
	}
	return out
}

// sequenceDistance returns the fraction of matching positions and the wildcard count.
func sequenceDistance(template, tokens []string, includeParams bool) (float64, int) {
	if len(template) == 0 {
		return 1, 0
	}
	equal, params := 0, 0
	for i, token := range template {
		if token == Wildcard {
			params++
			continue
		}
		if token == tokens[i] {
			equal++
		}
	}
	if includeParams {
		equal += params
	}
	return float64(equal) / float64(len(template)), params
}

func makeTemplate(tokens, existing []string) []string {
	out := make([]string, len(tokens))
	for i := range tokens {
		if tokens[i] == existing[i] {
			out[i] = tokens[i]
		} else {
			out[i] = Wildcard
		}
	}
	return out
}

func sameTokens(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

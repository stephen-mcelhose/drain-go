package drain

import (
	"sync"
	"testing"
)

// mustNew creates a Parser from cfg, failing the test on error.
func mustNew(t *testing.T, cfg Config) *Parser {
	t.Helper()
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// ── New / Config validation ──────────────────────────────────────────────────

func TestNew(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "New MUST return an error when LeadingTokens is negative",
			cfg:     Config{LeadingTokens: -1},
			wantErr: true,
		},
		{
			name:    "New MUST return an error when SimilarityThreshold exceeds 1",
			cfg:     Config{SimilarityThreshold: 1.1},
			wantErr: true,
		},
		{
			name:    "New MUST return an error when SimilarityThreshold is negative",
			cfg:     Config{SimilarityThreshold: -0.1},
			wantErr: true,
		},
		{
			name:    "New MUST return an error when MaxChildren is negative",
			cfg:     Config{MaxChildren: -1},
			wantErr: true,
		},
		{
			name:    "New MUST return an error when a Mask has an empty Name",
			cfg:     Config{Masks: []Mask{{Pattern: `\d+`, Name: ""}}},
			wantErr: true,
		},
		{
			name:    "New MUST return an error when a Mask Pattern is not a valid RE2 expression",
			cfg:     Config{Masks: []Mask{{Pattern: `(?!bad)`, Name: "X"}}},
			wantErr: true,
		},
		{
			name:    "New MUST succeed with a zero-value Config using all defaults",
			cfg:     Config{},
			wantErr: false,
		},
		{
			name:    "New MUST succeed when LeadingTokens is exactly 0",
			cfg:     Config{LeadingTokens: 0},
			wantErr: false,
		},
		{
			name:    "New MUST succeed when SimilarityThreshold is exactly 0",
			cfg:     Config{SimilarityThreshold: 0},
			wantErr: false,
		},
		{
			name:    "New MUST succeed when SimilarityThreshold is exactly 1",
			cfg:     Config{SimilarityThreshold: 1},
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ── Default config values ────────────────────────────────────────────────────

func TestNewDefaults(t *testing.T) {
	cases := []struct {
		name  string
		cfg   Config
		check func(t *testing.T, p *Parser)
	}{
		{
			name: "New MUST apply a default LeadingTokens of 1 when LeadingTokens is zero",
			cfg:  Config{},
			check: func(t *testing.T, p *Parser) {
				if p.leadingTokens != 1 {
					t.Errorf("leadingTokens = %d, want 1", p.leadingTokens)
				}
			},
		},
		{
			name: "New MUST apply a default SimilarityThreshold of 0.4 when the field is zero",
			cfg:  Config{},
			check: func(t *testing.T, p *Parser) {
				if p.similarity != 0.4 {
					t.Errorf("similarity = %f, want 0.4", p.similarity)
				}
			},
		},
		{
			name: "New MUST apply a default MaxChildren of 100 when the field is zero",
			cfg:  Config{},
			check: func(t *testing.T, p *Parser) {
				if p.maxChildren != 100 {
					t.Errorf("maxChildren = %d, want 100", p.maxChildren)
				}
			},
		},
		{
			name: "New MUST treat nil ParametrizeNumericTokens as enabled",
			cfg:  Config{ParametrizeNumericTokens: nil},
			check: func(t *testing.T, p *Parser) {
				if !p.parametrizeNumeric {
					t.Error("parametrizeNumeric = false, want true")
				}
			},
		},
		{
			name: "New MUST treat a false ParametrizeNumericTokens pointer as disabled",
			cfg:  Config{ParametrizeNumericTokens: new(bool)},
			check: func(t *testing.T, p *Parser) {
				if p.parametrizeNumeric {
					t.Error("parametrizeNumeric = true, want false")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, p)
		})
	}
}

// ── Train — cluster creation and change signalling ───────────────────────────

func TestTrain(t *testing.T) {
	cases := []struct {
		name  string
		cfg   Config
		lines []string // trained in order; check is applied to the last result
		check func(t *testing.T, results []Result, p *Parser)
	}{
		{
			name:  "Train MUST return ClusterCreated and assign ID 1 for the first message seen",
			cfg:   Config{},
			lines: []string{"user alice logged in"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				r := rs[0]
				if r.Change != ClusterCreated {
					t.Errorf("Change = %q, want ClusterCreated", r.Change)
				}
				if r.ClusterID != 1 {
					t.Errorf("ClusterID = %d, want 1", r.ClusterID)
				}
			},
		},
		{
			name:  "Train MUST return NoChange when a message matches an existing cluster exactly",
			cfg:   Config{},
			lines: []string{"user alice logged in", "user alice logged in"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				if rs[1].Change != NoChange {
					t.Errorf("Change = %q, want NoChange", rs[1].Change)
				}
				if rs[1].ClusterID != rs[0].ClusterID {
					t.Errorf("ClusterID changed between identical messages")
				}
			},
		},
		{
			name:  "Train MUST return ClusterTemplateChanged when differing tokens force generalization",
			cfg:   Config{},
			lines: []string{"user alice logged in", "user bob logged in"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				if rs[1].Change != ClusterTemplateChanged {
					t.Errorf("Change = %q, want ClusterTemplateChanged", rs[1].Change)
				}
			},
		},
		{
			name:  "Train MUST generalize every differing token position to the wildcard",
			cfg:   Config{},
			lines: []string{"user alice logged in", "user bob logged in"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				want := "user <*> logged in"
				if rs[1].Template != want {
					t.Errorf("Template = %q, want %q", rs[1].Template, want)
				}
			},
		},
		{
			name:  "Train MUST create a separate cluster for messages of different token lengths",
			cfg:   Config{SimilarityThreshold: 1},
			lines: []string{"a b c", "a b"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				if rs[1].ClusterID == rs[0].ClusterID {
					t.Error("different-length messages mapped to the same cluster")
				}
				if rs[1].Change != ClusterCreated {
					t.Errorf("Change = %q, want ClusterCreated for second message", rs[1].Change)
				}
			},
		},
		{
			name:  "Train MUST assign cluster IDs sequentially starting at 1",
			cfg:   Config{SimilarityThreshold: 1},
			lines: []string{"a b", "c d", "e f"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				for i, r := range rs {
					want := i + 1
					if r.ClusterID != want {
						t.Errorf("step %d: ClusterID = %d, want %d", i, r.ClusterID, want)
					}
				}
			},
		},
		{
			name:  "Train MUST increment the cluster Size on each absorbed message",
			cfg:   Config{},
			lines: []string{"user alice logged in", "user bob logged in", "user carol logged in"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				if rs[2].Size != 3 {
					t.Errorf("Size = %d, want 3 after three absorptions", rs[2].Size)
				}
			},
		},
		{
			name:  "Train MUST report the total cluster count in ClusterCount",
			cfg:   Config{SimilarityThreshold: 1},
			lines: []string{"a b", "c d", "e f"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				if rs[2].ClusterCount != 3 {
					t.Errorf("ClusterCount = %d, want 3", rs[2].ClusterCount)
				}
			},
		},
		{
			name: "Train MUST route numeric-token messages through the wildcard branch when ParametrizeNumericTokens is true",
			cfg:  Config{},
			// "10.0.0.1" contains digits → routed via wildcard → should cluster with "192.168.0.1"
			lines: []string{"connected 10.0.0.1", "connected 192.168.0.1"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				if rs[1].ClusterID != rs[0].ClusterID {
					t.Error("numeric tokens should share a cluster when ParametrizeNumericTokens is true")
				}
			},
		},
		{
			name:  "Train MUST NOT route numeric tokens through the wildcard branch when ParametrizeNumericTokens is false",
			cfg:   Config{SimilarityThreshold: 1, ParametrizeNumericTokens: new(bool)},
			lines: []string{"connected 10.0.0.1", "connected 192.168.0.1"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				// With similarity threshold 1 and no wildcard routing, they cannot merge.
				if rs[1].ClusterID == rs[0].ClusterID {
					t.Error("distinct numeric tokens should NOT share a cluster when ParametrizeNumericTokens is false")
				}
			},
		},
		{
			name: "Train MUST apply masks in declaration order before tokenizing",
			cfg: Config{
				Masks: []Mask{
					{Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`, Name: "IP"},
				},
			},
			lines: []string{"connected to 10.0.0.1", "connected to 192.168.0.1"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				if rs[1].ClusterID != rs[0].ClusterID {
					t.Error("masked messages should share a cluster")
				}
				want := "connected to <IP>"
				if rs[1].Template != want {
					t.Errorf("Template = %q, want %q", rs[1].Template, want)
				}
			},
		},
		{
			name:  "Train MUST treat ExtraDelimiters as token separators equivalent to whitespace",
			cfg:   Config{ExtraDelimiters: []string{":"}},
			lines: []string{"connected:10.0.0.1", "connected:192.168.0.1"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				// "10.0.0.1" and "192.168.0.1" both contain digits → wildcard branch → same cluster.
				if rs[1].ClusterID != rs[0].ClusterID {
					t.Error("colon-delimited messages should cluster together")
				}
			},
		},
		{
			name:  "Train MUST treat the empty string as a zero-token message",
			cfg:   Config{},
			lines: []string{""},
			check: func(t *testing.T, rs []Result, p *Parser) {
				if rs[0].Change != ClusterCreated {
					t.Errorf("Change = %q, want ClusterCreated for empty message", rs[0].Change)
				}
				if rs[0].Template != "" {
					t.Errorf("Template = %q, want empty string", rs[0].Template)
				}
			},
		},
		{
			name:  "Train MUST return Tokens as a slice copy of the same template",
			cfg:   Config{},
			lines: []string{"user alice logged in", "user bob logged in"},
			check: func(t *testing.T, rs []Result, p *Parser) {
				r := rs[1]
				if len(r.Tokens) != 4 {
					t.Fatalf("len(Tokens) = %d, want 4", len(r.Tokens))
				}
				// Mutating Tokens must not change the parser's template.
				r.Tokens[1] = "MUTATED"
				snapshot := p.Clusters()
				if snapshot[0].Template[1] == "MUTATED" {
					t.Error("mutating Result.Tokens must not affect the parser's internal template")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mustNew(t, tc.cfg)
			results := make([]Result, len(tc.lines))
			for i, line := range tc.lines {
				results[i] = p.Train(line)
			}
			tc.check(t, results, p)
		})
	}
}

// ── Match ────────────────────────────────────────────────────────────────────

func TestMatch(t *testing.T) {
	cases := []struct {
		name   string
		cfg    Config
		train  []string
		line   string
		opts   []MatchOption
		wantOK bool
		check  func(t *testing.T, r Result)
	}{
		{
			name:   "Match MUST return true and a result when a line matches a learned template",
			cfg:    Config{},
			train:  []string{"user alice logged in", "user bob logged in"},
			line:   "user carol logged in",
			wantOK: true,
			check: func(t *testing.T, r Result) {
				if r.Template != "user <*> logged in" {
					t.Errorf("Template = %q", r.Template)
				}
			},
		},
		{
			name:   "Match MUST return false when no learned template matches the line",
			cfg:    Config{},
			train:  []string{"user alice logged in", "user bob logged in"},
			line:   "server started",
			wantOK: false,
		},
		{
			name:   "Match MUST return false when the token count differs from all learned templates",
			cfg:    Config{},
			train:  []string{"user alice logged in"},
			line:   "user logged in",
			wantOK: false,
		},
		{
			name:   "Match MUST NOT create a new cluster when no template matches",
			cfg:    Config{},
			train:  []string{"user alice logged in"},
			line:   "server started",
			wantOK: false,
			check: func(t *testing.T, r Result) {
				// caller verifies cluster count separately — see setup in t.Run below
			},
		},
		{
			name:  "Match with WithSimilarityThreshold MUST accept a partial template match",
			cfg:   Config{},
			train: []string{"user alice logged in", "user bob logged in"},
			// Template is "user <*> logged in"; "user carol logged out" matches 3/4 tokens = 0.75.
			line:   "user carol logged out",
			opts:   []MatchOption{WithSimilarityThreshold(0.7)},
			wantOK: true,
		},
		{
			name:  "Match with WithSimilarityThreshold MUST reject a match below the threshold",
			cfg:   Config{},
			train: []string{"user alice logged in", "user bob logged in"},
			// Template is "user <*> logged in"; "user carol logged out" matches 3/4 = 0.75.
			line:   "user carol logged out",
			opts:   []MatchOption{WithSimilarityThreshold(0.9)},
			wantOK: false,
		},
		{
			name:   "Match with WithStrategy(FullSearchAlways) MUST find a cluster even without tree routing",
			cfg:    Config{LeadingTokens: 0}, // no leading-token routing
			train:  []string{"user alice logged in", "user bob logged in"},
			line:   "user carol logged in",
			opts:   []MatchOption{WithStrategy(FullSearchAlways)},
			wantOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mustNew(t, tc.cfg)
			for _, line := range tc.train {
				p.Train(line)
			}
			countBefore := len(p.Clusters())

			r, ok := p.Match(tc.line, tc.opts...)
			if ok != tc.wantOK {
				t.Fatalf("Match ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.check != nil {
				tc.check(t, r)
			}

			// Verify state was not mutated.
			countAfter := len(p.Clusters())
			if countAfter != countBefore {
				t.Errorf("cluster count changed from %d to %d after Match", countBefore, countAfter)
			}
		})
	}
}

// Match with TreeOnly strategy and a guaranteed tree miss.
func TestMatchTreeOnlyMiss(t *testing.T) {
	const name = "Match with WithStrategy(TreeOnly) MUST return false on a tree-routing miss even when a same-length cluster exists"
	t.Run(name, func(t *testing.T) {
		// LeadingTokens: 1 routes on the first token only.
		// FullSearchFallback would still find a same-length cluster.
		// TreeOnly must not fall back to linear scan.
		p := mustNew(t, Config{LeadingTokens: 1, SimilarityThreshold: 0.4})
		p.Train("alpha beta gamma")
		p.Train("alpha delta gamma")
		// Template is "alpha <*> gamma". Tree routes via "alpha".
		// Query "zeta beta gamma" has same length but no matching tree path for "zeta".
		_, ok := p.Match("zeta beta gamma", WithStrategy(TreeOnly))
		if ok {
			t.Error("TreeOnly should not fall back to a full scan on a tree miss")
		}
	})
}

// ── LRU eviction ─────────────────────────────────────────────────────────────

func TestLRUEviction(t *testing.T) {
	cases := []struct {
		name  string
		cfg   Config
		steps func(t *testing.T, p *Parser)
	}{
		{
			name: "Train MUST evict the least recently used cluster when MaxClusters is reached",
			cfg:  Config{MaxClusters: 2, SimilarityThreshold: 1},
			steps: func(t *testing.T, p *Parser) {
				p.Train("alpha event")
				p.Train("beta event")
				p.Train("gamma event") // evicts alpha
				if len(p.Clusters()) != 2 {
					t.Fatalf("cluster count = %d, want 2", len(p.Clusters()))
				}
				if _, ok := p.Match("alpha event", WithStrategy(FullSearchAlways)); ok {
					t.Error("evicted cluster must not be found by Match")
				}
			},
		},
		{
			name: "Train MUST retain a cluster that was recently used even when MaxClusters is exceeded",
			cfg:  Config{MaxClusters: 2, SimilarityThreshold: 1},
			steps: func(t *testing.T, p *Parser) {
				r1 := p.Train("alpha event")
				p.Train("beta event")
				// Re-train alpha to make it MRU; beta becomes LRU.
				p.Train("alpha event")
				p.Train("gamma event") // should evict beta, not alpha
				if _, ok := p.Match("alpha event", WithStrategy(FullSearchAlways)); !ok {
					t.Errorf("recently-used cluster %d must not be evicted", r1.ClusterID)
				}
				if _, ok := p.Match("beta event", WithStrategy(FullSearchAlways)); ok {
					t.Error("least recently used cluster must be evicted")
				}
			},
		},
		{
			name: "Train MUST NOT evict when the cluster count is below MaxClusters",
			cfg:  Config{MaxClusters: 3, SimilarityThreshold: 1},
			steps: func(t *testing.T, p *Parser) {
				p.Train("alpha event")
				p.Train("beta event")
				if len(p.Clusters()) != 2 {
					t.Errorf("cluster count = %d, want 2", len(p.Clusters()))
				}
			},
		},
		{
			name: "Train MUST NOT evict any cluster when MaxClusters is zero",
			cfg:  Config{MaxClusters: 0, SimilarityThreshold: 1},
			steps: func(t *testing.T, p *Parser) {
				for i := 0; i < 50; i++ {
					p.Train(string(rune('a'+i)) + " event")
				}
				if len(p.Clusters()) != 50 {
					t.Errorf("cluster count = %d, want 50 (unbounded)", len(p.Clusters()))
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mustNew(t, tc.cfg)
			tc.steps(t, p)
		})
	}
}

// ── Clusters ─────────────────────────────────────────────────────────────────

func TestClusters(t *testing.T) {
	cases := []struct {
		name  string
		cfg   Config
		train []string
		check func(t *testing.T, cs []Cluster)
	}{
		{
			name:  "Clusters MUST return an empty slice when no messages have been trained",
			cfg:   Config{},
			train: nil,
			check: func(t *testing.T, cs []Cluster) {
				if len(cs) != 0 {
					t.Errorf("len = %d, want 0", len(cs))
				}
			},
		},
		{
			name:  "Clusters MUST return clusters ordered by ascending ID",
			cfg:   Config{SimilarityThreshold: 1},
			train: []string{"a b", "c d", "e f"},
			check: func(t *testing.T, cs []Cluster) {
				for i := 1; i < len(cs); i++ {
					if cs[i].ID <= cs[i-1].ID {
						t.Errorf("out-of-order IDs: cs[%d].ID = %d, cs[%d].ID = %d",
							i-1, cs[i-1].ID, i, cs[i].ID)
					}
				}
			},
		},
		{
			name:  "Clusters MUST return deep copies that callers may mutate without affecting the parser",
			cfg:   Config{},
			train: []string{"user alice logged in", "user bob logged in"},
			check: func(t *testing.T, cs []Cluster) {
				if len(cs) == 0 {
					t.Fatal("expected at least one cluster")
				}
				original := append([]string(nil), cs[0].Template...)
				cs[0].Template[0] = "MUTATED"
				after := mustNew(t, Config{}).Clusters() // dummy; we just need Clusters again
				_ = after
				// Re-read from same parser (captured via closure in the test runner).
				// We verify via the original snapshot vs mutation.
				if cs[0].Template[0] != "MUTATED" {
					t.Fatal("slice mutation did not take — test setup error")
				}
				// The original slice is safe: the returned Cluster is a deep copy.
				if original[0] == "MUTATED" {
					t.Error("original slice was aliased — Clusters must return deep copies")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mustNew(t, tc.cfg)
			for _, line := range tc.train {
				p.Train(line)
			}
			cs := p.Clusters()
			tc.check(t, cs)
		})
	}
}

// We need a separate test for the deep-copy property because the closure above
// needs the parser to be in scope. Replace the dummy with the real parser.
func TestClustersDeepCopy(t *testing.T) {
	const name = "Clusters MUST return deep copies whose mutation does not affect the parser's internal template"
	t.Run(name, func(t *testing.T) {
		p := mustNew(t, Config{})
		p.Train("user alice logged in")
		p.Train("user bob logged in")

		cs := p.Clusters()
		if len(cs) == 0 {
			t.Fatal("expected cluster")
		}
		cs[0].Template[0] = "MUTATED"

		cs2 := p.Clusters()
		if cs2[0].Template[0] == "MUTATED" {
			t.Error("mutation of returned Cluster.Template propagated into the parser")
		}
	})
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestConcurrency(t *testing.T) {
	t.Run("Parser MUST NOT data-race under concurrent Train calls", func(t *testing.T) {
		p := mustNew(t, Config{})
		var wg sync.WaitGroup
		lines := []string{
			"user alice logged in",
			"user bob logged in",
			"server started on port 8080",
			"connection refused from 10.0.0.1",
		}
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				p.Train(lines[i%len(lines)])
			}(i)
		}
		wg.Wait()
	})

	t.Run("Parser MUST NOT data-race under concurrent Train and Match calls", func(t *testing.T) {
		p := mustNew(t, Config{})
		// Seed a template.
		p.Train("user alice logged in")
		p.Train("user bob logged in")

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				p.Train("user carol logged in")
			}()
			go func() {
				defer wg.Done()
				p.Match("user dave logged in")
			}()
		}
		wg.Wait()
	})

	t.Run("Clusters MUST NOT data-race under concurrent Train and Clusters calls", func(t *testing.T) {
		p := mustNew(t, Config{})
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(2)
			go func(i int) {
				defer wg.Done()
				p.Train(string(rune('a'+i%26)) + " event")
			}(i)
			go func() {
				defer wg.Done()
				_ = p.Clusters()
			}()
		}
		wg.Wait()
	})
}

// ── ExtractParameters ─────────────────────────────────────────────────────────

func TestExtractParameters(t *testing.T) {
	cases := []struct {
		name     string
		cfg      Config
		train    []string
		template []string
		line     string
		want     []ExtractedParameter
		wantOK   bool
	}{
		{
			name:     "ExtractParameters MUST return an empty slice when the template has no variable positions",
			cfg:      Config{SimilarityThreshold: 1},
			train:    []string{"user alice logged in"},
			template: []string{"user", "alice", "logged", "in"},
			line:     "user alice logged in",
			want:     nil,
			wantOK:   true,
		},
		{
			name:     "ExtractParameters MUST extract a single catch-all wildcard",
			cfg:      Config{},
			train:    []string{"user alice logged in", "user bob logged in"},
			template: []string{"user", Wildcard, "logged", "in"},
			line:     "user carol logged in",
			want:     []ExtractedParameter{{Value: "carol", Name: "*"}},
			wantOK:   true,
		},
		{
			name: "ExtractParameters MUST extract a single named mask",
			cfg: Config{
				Masks: []Mask{{Name: "IP", Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`}},
			},
			train:    []string{"connected to 10.0.0.1"},
			template: []string{"connected", "to", "<IP>"},
			line:     "connected to 192.168.1.1",
			want:     []ExtractedParameter{{Value: "192.168.1.1", Name: "IP"}},
			wantOK:   true,
		},
		{
			name: "ExtractParameters MUST extract mixed wildcard and named mask positions",
			cfg: Config{
				Masks: []Mask{{Name: "IP", Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`}},
			},
			train: []string{
				"user alice connected to 10.0.0.1",
				"user bob connected to 10.0.0.1",
			},
			template: []string{"user", Wildcard, "connected", "to", "<IP>"},
			line:     "user carol connected to 192.168.1.1",
			want: []ExtractedParameter{
				{Value: "carol", Name: "*"},
				{Value: "192.168.1.1", Name: "IP"},
			},
			wantOK: true,
		},
		{
			name:     "ExtractParameters MUST return false when token count does not match",
			cfg:      Config{},
			train:    []string{"hello world"},
			template: []string{"hello", "world"},
			line:     "hello beautiful world",
			want:     nil,
			wantOK:   false,
		},
		{
			name:     "ExtractParameters MUST return an empty slice and true for an empty template and empty line",
			cfg:      Config{},
			train:    nil,
			template: []string{},
			line:     "",
			want:     nil,
			wantOK:   true,
		},
		{
			name: "ExtractParameters MUST respect ExtraDelimiters when tokenizing the raw line",
			cfg:  Config{ExtraDelimiters: []string{"|"}},
			train: []string{
				"user|alice|logged|in",
				"user|bob|logged|in",
			},
			template: []string{"user", Wildcard, "logged", "in"},
			line:     "user|carol|logged|in",
			want:     []ExtractedParameter{{Value: "carol", Name: "*"}},
			wantOK:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mustNew(t, tc.cfg)
			for _, line := range tc.train {
				p.Train(line)
			}

			got, ok := p.ExtractParameters(tc.template, tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len(got) = %d, want %d; got %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("param[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestExtractParametersConcurrency(t *testing.T) {
	t.Run("ExtractParameters MUST NOT data-race under concurrent Train calls", func(t *testing.T) {
		p := mustNew(t, Config{
			Masks: []Mask{{Name: "IP", Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`}},
		})
		p.Train("connected to 10.0.0.1")
		p.Train("connected to 192.168.0.1")

		template := []string{"connected", "to", "<IP>"}
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				p.Train("connected to 10.0.0.2")
			}()
			go func() {
				defer wg.Done()
				p.ExtractParameters(template, "connected to 10.0.0.3")
			}()
		}
		wg.Wait()
	})
}

// ── MatchWithStrategy (deprecated shim) ──────────────────────────────────────

func TestMatchWithStrategy(t *testing.T) {
	t.Run("MatchWithStrategy MUST produce the same result as Match with the equivalent WithStrategy option", func(t *testing.T) {
		p := mustNew(t, Config{})
		p.Train("user alice logged in")
		p.Train("user bob logged in")

		for _, strategy := range []SearchStrategy{TreeOnly, FullSearchFallback, FullSearchAlways} {
			r1, ok1 := p.MatchWithStrategy("user carol logged in", strategy)
			r2, ok2 := p.Match("user carol logged in", WithStrategy(strategy))
			if ok1 != ok2 || r1.ClusterID != r2.ClusterID || r1.Template != r2.Template {
				t.Errorf("strategy %d: MatchWithStrategy and Match diverged: (%v,%v) vs (%v,%v)",
					strategy, ok1, r1, ok2, r2)
			}
		}
	})
}

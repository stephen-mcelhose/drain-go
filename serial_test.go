package drain

import (
	"encoding/json"
	"testing"
)

// trainN seeds a parser with a canonical set of lines.
func trainN(p *Parser) {
	lines := []string{
		"user alice logged in",
		"user bob logged in",
		"connected to 10.0.0.1",
		"connected to 192.168.0.1",
		"server started on port 8080",
		"server started on port 9090",
	}
	for _, l := range lines {
		p.Train(l)
	}
}

func TestSerializeLoad(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) *Parser
		check func(t *testing.T, original, loaded *Parser, data []byte)
	}{
		{
			name: "Serialize and Load MUST produce a parser that classifies the same messages identically",
			setup: func(t *testing.T) *Parser {
				p := mustNew(t, Config{})
				trainN(p)
				return p
			},
			check: func(t *testing.T, orig, loaded *Parser, _ []byte) {
				probes := []string{
					"user carol logged in",
					"connected to 172.16.0.1",
					"server started on port 443",
					"unknown message here",
				}
				for _, probe := range probes {
					r1, ok1 := orig.Match(probe, WithStrategy(FullSearchAlways))
					r2, ok2 := loaded.Match(probe, WithStrategy(FullSearchAlways))
					if ok1 != ok2 {
						t.Errorf("probe %q: Match ok diverged: orig=%v loaded=%v", probe, ok1, ok2)
						continue
					}
					if ok1 && r1.Template != r2.Template {
						t.Errorf("probe %q: Template diverged: orig=%q loaded=%q",
							probe, r1.Template, r2.Template)
					}
				}
			},
		},
		{
			name: "Serialize and Load MUST preserve cluster IDs and sizes exactly",
			setup: func(t *testing.T) *Parser {
				p := mustNew(t, Config{})
				trainN(p)
				return p
			},
			check: func(t *testing.T, orig, loaded *Parser, _ []byte) {
				cs1 := orig.Clusters()
				cs2 := loaded.Clusters()
				if len(cs1) != len(cs2) {
					t.Fatalf("cluster count: orig=%d loaded=%d", len(cs1), len(cs2))
				}
				for i := range cs1 {
					if cs1[i].ID != cs2[i].ID {
						t.Errorf("[%d] ID: orig=%d loaded=%d", i, cs1[i].ID, cs2[i].ID)
					}
					if cs1[i].Size != cs2[i].Size {
						t.Errorf("[%d] Size: orig=%d loaded=%d", i, cs1[i].Size, cs2[i].Size)
					}
				}
			},
		},
		{
			name: "Serialize and Load MUST assign new cluster IDs that continue from the original next ID",
			setup: func(t *testing.T) *Parser {
				p := mustNew(t, Config{SimilarityThreshold: 1})
				p.Train("alpha bravo")
				p.Train("charlie delta")
				// nextID is now 2.
				return p
			},
			check: func(t *testing.T, orig, loaded *Parser, _ []byte) {
				r := loaded.Train("echo foxtrot")
				if r.ClusterID != 3 {
					t.Errorf("first new cluster after Load: ID = %d, want 3", r.ClusterID)
				}
			},
		},
		{
			name: "Serialize and Load MUST preserve LRU eviction order",
			setup: func(t *testing.T) *Parser {
				p := mustNew(t, Config{MaxClusters: 2, SimilarityThreshold: 1})
				p.Train("alpha event")
				p.Train("beta event")
				// alpha is LRU; beta is MRU.
				return p
			},
			check: func(t *testing.T, _, loaded *Parser, _ []byte) {
				// Adding a third cluster should evict alpha (LRU).
				loaded.Train("gamma event")
				if _, ok := loaded.Match("alpha event", WithStrategy(FullSearchAlways)); ok {
					t.Error("LRU cluster (alpha) should have been evicted after Load")
				}
				if _, ok := loaded.Match("beta event", WithStrategy(FullSearchAlways)); !ok {
					t.Error("MRU cluster (beta) should survive after Load")
				}
			},
		},
		{
			name: "Serialize and Load MUST preserve mask configuration",
			setup: func(t *testing.T) *Parser {
				p := mustNew(t, Config{
					Masks: []Mask{
						{Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`, Name: "IP"},
					},
				})
				p.Train("connected to 10.0.0.1")
				p.Train("connected to 192.168.0.1")
				return p
			},
			check: func(t *testing.T, _, loaded *Parser, _ []byte) {
				// A new IP address should match the <IP> template.
				r, ok := loaded.Match("connected to 172.16.0.1")
				if !ok {
					t.Fatal("mask was not preserved: new IP address did not match the template")
				}
				want := "connected to <IP>"
				if r.Template != want {
					t.Errorf("Template = %q, want %q", r.Template, want)
				}
			},
		},
		{
			name: "Serialize and Load MUST work correctly for a parser with no trained clusters",
			setup: func(t *testing.T) *Parser {
				return mustNew(t, Config{})
			},
			check: func(t *testing.T, _, loaded *Parser, _ []byte) {
				if cs := loaded.Clusters(); len(cs) != 0 {
					t.Errorf("cluster count = %d, want 0", len(cs))
				}
				r := loaded.Train("first message after load")
				if r.Change != ClusterCreated || r.ClusterID != 1 {
					t.Errorf("Train after load: Change=%q ClusterID=%d, want ClusterCreated/1",
						r.Change, r.ClusterID)
				}
			},
		},
		{
			name: "Serialize and Load MUST preserve ExtraDelimiters",
			setup: func(t *testing.T) *Parser {
				p := mustNew(t, Config{ExtraDelimiters: []string{":"}})
				p.Train("connected:10.0.0.1")
				p.Train("connected:192.168.0.1")
				return p
			},
			check: func(t *testing.T, _, loaded *Parser, _ []byte) {
				r := loaded.Train("connected:172.16.0.1")
				if r.Change == ClusterCreated {
					t.Error("ExtraDelimiter was not preserved: new message created a new cluster instead of joining existing one")
				}
			},
		},
		{
			name: "Serialize MUST produce valid JSON with the expected version field",
			setup: func(t *testing.T) *Parser {
				return mustNew(t, Config{})
			},
			check: func(t *testing.T, _, _ *Parser, data []byte) {
				var raw map[string]interface{}
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatalf("output is not valid JSON: %v", err)
				}
				v, ok := raw["version"]
				if !ok {
					t.Fatal("JSON missing 'version' field")
				}
				if int(v.(float64)) != serializationVersion {
					t.Errorf("version = %v, want %d", v, serializationVersion)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := tc.setup(t)

			data, err := orig.Serialize()
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}

			loaded, err := Load(data)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			tc.check(t, orig, loaded, data)
		})
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{
			name: "Load MUST return an error when the input is not valid JSON",
			data: []byte(`not json at all`),
		},
		{
			name: "Load MUST return an error when the version field is unrecognized",
			data: func() []byte {
				b, _ := json.Marshal(map[string]interface{}{
					"version": 999,
					"config":  map[string]interface{}{"depth": 4, "similarity_threshold": 0.4, "max_children": 100},
				})
				return b
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(tc.data)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

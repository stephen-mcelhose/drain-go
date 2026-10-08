package drain

import "testing"

func TestStreamingTrainingAndInference(t *testing.T) {
	p, err := New(Config{Depth: 4, SimilarityThreshold: 0.4})
	if err != nil {
		t.Fatal(err)
	}

	first := p.Train("user alice logged in")
	if first.Change != ClusterCreated || first.ClusterID != 1 {
		t.Fatalf("first = %#v", first)
	}
	second := p.Train("user bob logged in")
	if second.ClusterID != first.ClusterID || second.Change != ClusterTemplateChanged {
		t.Fatalf("second = %#v", second)
	}
	if got, want := second.Template, "user <*> logged in"; got != want {
		t.Fatalf("template = %q, want %q", got, want)
	}

	matched, ok := p.Match("user carol logged in")
	if !ok || matched.ClusterID != first.ClusterID {
		t.Fatalf("match = %#v, %v", matched, ok)
	}
	if _, ok := p.Match("user carol logged out"); ok {
		t.Fatal("unexpected inference match")
	}
	if got := len(p.Clusters()); got != 1 {
		t.Fatalf("clusters after inference = %d, want 1", got)
	}
}

func TestMasksAndDelimiters(t *testing.T) {
	p, err := New(Config{ExtraDelimiters: []string{":"}, Masks: []Mask{{Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`, Name: "IP"}}})
	if err != nil {
		t.Fatal(err)
	}
	first := p.Train("connected:10.0.0.1")
	second := p.Train("connected:192.168.0.1")
	if first.ClusterID != second.ClusterID {
		t.Fatalf("mask did not group messages: %d != %d", first.ClusterID, second.ClusterID)
	}
	if got, want := second.Template, "connected <IP>"; got != want {
		t.Fatalf("template = %q, want %q", got, want)
	}
}

func TestLRUEviction(t *testing.T) {
	p, err := New(Config{MaxClusters: 2, SimilarityThreshold: 1})
	if err != nil {
		t.Fatal(err)
	}
	p.Train("alpha event")
	p.Train("beta event")
	p.Train("gamma event")
	clusters := p.Clusters()
	if len(clusters) != 2 {
		t.Fatalf("cluster count = %d, want 2", len(clusters))
	}
	if _, ok := p.MatchWithStrategy("alpha event", FullSearchAlways); ok {
		t.Fatal("evicted cluster is still matched")
	}
}

func TestInvalidConfig(t *testing.T) {
	if _, err := New(Config{Depth: 2}); err == nil {
		t.Fatal("expected depth error")
	}
	if _, err := New(Config{SimilarityThreshold: 1.1}); err == nil {
		t.Fatal("expected threshold error")
	}
}

package drain_test

import (
	"fmt"
	"log"

	"github.com/stephen-mcelhose/drain-go"
)

func Example() {
	parser, err := drain.New(drain.Config{
		LeadingTokens:       1,
		SimilarityThreshold: 0.4,
		MaxClusters:         10_000,
		Masks: []drain.Mask{
			{Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`, Name: "IP"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	for _, line := range []string{
		"connected to 10.0.0.1",
		"connected to 192.168.0.1",
		"user alice logged in",
		"user bob logged in",
	} {
		result := parser.Train(line)
		fmt.Printf("id=%d change=%s template=%q\n", result.ClusterID, result.Change, result.Template)
	}

	// Inference-only: a non-match does not create a cluster.
	if result, ok := parser.Match("user carol logged in"); ok {
		fmt.Printf("classified as template %d: %s\n", result.ClusterID, result.Template)
	}

	// Masks: named masks can be supplied
	if result, ok := parser.Match("connected to 192.168.0.1"); ok {
		fmt.Printf("classified as template %d: %s\n", result.ClusterID, result.Template)
	}

	// Output:
	// id=1 change=cluster_created template="connected to <IP>"
	// id=1 change=none template="connected to <IP>"
	// id=2 change=cluster_created template="user alice logged in"
	// id=2 change=cluster_template_changed template="user <*> logged in"
	// classified as template 2: user <*> logged in
	// classified as template 1: connected to <IP>
}

func ExampleParser_Serialize() {
	parser, err := drain.New(drain.Config{})
	if err != nil {
		log.Fatal(err)
	}

	parser.Train("user alice logged in")
	parser.Train("user bob logged in")

	data, err := parser.Serialize()
	if err != nil {
		log.Fatal(err)
	}

	restored, err := drain.Load(data)
	if err != nil {
		log.Fatal(err)
	}

	// Restored parser classifies without retraining.
	if result, ok := restored.Match("user carol logged in"); ok {
		fmt.Printf("template=%q clusters=%d\n", result.Template, result.ClusterCount)
	}

	// Output:
	// template="user <*> logged in" clusters=1
}

func ExampleParser_ExtractParameters() {
	parser, err := drain.New(drain.Config{
		Masks: []drain.Mask{
			{Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`, Name: "IP"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	parser.Train("connected to 10.0.0.1")
	parser.Train("connected to 192.168.0.1")

	result, ok := parser.Match("connected to 10.0.0.2")
	if !ok {
		log.Fatal("no match")
	}

	params, ok := parser.ExtractParameters(result.Tokens, "connected to 10.0.0.2")
	if !ok {
		log.Fatal("extraction failed")
	}

	for _, p := range params {
		fmt.Printf("name=%s value=%s\n", p.Name, p.Value)
	}

	// Output:
	// name=IP value=10.0.0.2
}

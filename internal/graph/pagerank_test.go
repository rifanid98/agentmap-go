package graph_test

import (
	"math"
	"testing"

	"github.com/rifanid98/agentmap-go/internal/graph"
)

func TestPageRankEmpty(t *testing.T) {
	r := graph.PageRank(nil, nil, nil)
	if len(r) != 0 {
		t.Errorf("expected empty result for empty nodes, got %v", r)
	}
}

func TestPageRankSumToOne(t *testing.T) {
	nodes := []string{"a", "b", "c"}
	edges := []graph.Edge{
		{From: "a", To: "b", Weight: 1},
		{From: "b", To: "c", Weight: 1},
	}
	r := graph.PageRank(nodes, edges, nil)
	var sum float64
	for _, v := range r {
		sum += v
	}
	if math.Abs(sum-1.0) > 1e-4 {
		t.Errorf("PageRank scores should sum to ~1, got %f", sum)
	}
}

func TestPageRankHubRanksHigher(t *testing.T) {
	// "hub" is imported by both "a" and "b"; it should rank highest.
	nodes := []string{"a", "b", "hub"}
	edges := []graph.Edge{
		{From: "a", To: "hub", Weight: 3},
		{From: "b", To: "hub", Weight: 3},
	}
	r := graph.PageRank(nodes, edges, nil)
	if r["hub"] <= r["a"] || r["hub"] <= r["b"] {
		t.Errorf("hub should rank highest: hub=%f a=%f b=%f", r["hub"], r["a"], r["b"])
	}
}

func TestPageRankDeterministic(t *testing.T) {
	nodes := []string{"x", "y", "z"}
	edges := []graph.Edge{{From: "x", To: "y", Weight: 2}, {From: "y", To: "z", Weight: 1}}
	r1 := graph.PageRank(nodes, edges, nil)
	r2 := graph.PageRank(nodes, edges, nil)
	for k := range r1 {
		if r1[k] != r2[k] {
			t.Errorf("non-deterministic: %s = %f vs %f", k, r1[k], r2[k])
		}
	}
}

func TestPageRankSelfLoopSkipped(t *testing.T) {
	// Self-loops should be ignored; result should still be valid.
	nodes := []string{"a", "b"}
	edges := []graph.Edge{
		{From: "a", To: "a", Weight: 10}, // self-loop
		{From: "a", To: "b", Weight: 1},
	}
	r := graph.PageRank(nodes, edges, nil)
	if r["b"] == 0 {
		t.Error("b should have non-zero rank")
	}
}

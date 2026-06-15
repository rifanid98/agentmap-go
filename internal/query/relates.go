package query

import (
	"sort"

	"github.com/rifanid98/agentmap-go/internal/graph"
	"github.com/rifanid98/agentmap-go/internal/model"
)

const relatedLimit = 10

// RelatesResult is the result of --relates.
type RelatesResult struct {
	Package    string            `json:"package"`
	PageRank   float64           `json:"pagerank"`
	Exports    []model.Symbol    `json:"exports"`
	Imports    []string          `json:"imports"`
	Dependents []string          `json:"dependents"`
	Related    []RelatedPkg      `json:"related"`
	Error      string            `json:"error,omitempty"`
	Candidates []string          `json:"candidates,omitempty"`
}

// RelatedPkg is one entry in the related list.
type RelatedPkg struct {
	Package string  `json:"package"`
	Score   float64 `json:"score"`
}

// Relates computes the blast-radius view for a package: its direct
// exports/imports/dependents plus a random-walk relevance ranking of the
// transitively related packages (bidirectional PageRank personalised to the
// target). Zero results → caller exits 1.
func Relates(m *model.Map, q string) RelatesResult {
	key, candidates := ResolvePackage(m, q)
	if key == "" {
		return RelatesResult{Error: "no match", Candidates: candidates}
	}
	pkg := m.Packages[key]

	keys := sortedPackageKeys(m)
	// bidirectional graph
	var biEdges []graph.Edge
	for _, p := range keys {
		pp := m.Packages[p]
		for _, tp := range pp.Imports {
			if m.Packages[tp] == nil {
				continue
			}
			biEdges = append(biEdges, graph.Edge{From: p, To: tp, Weight: 1})
			biEdges = append(biEdges, graph.Edge{From: tp, To: p, Weight: 1})
		}
	}
	rel := graph.PageRank(keys, biEdges, map[string]float64{key: 1})
	type scored struct {
		k string
		v float64
	}
	var scores []scored
	for _, k := range keys {
		if k == key {
			continue
		}
		scores = append(scores, scored{k, rel[k]})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].v != scores[j].v {
			return scores[i].v > scores[j].v
		}
		return scores[i].k < scores[j].k
	})
	var related []RelatedPkg
	for i, s := range scores {
		if i >= relatedLimit {
			break
		}
		related = append(related, RelatedPkg{Package: s.k, Score: round6(s.v)})
	}
	return RelatesResult{
		Package:    key,
		PageRank:   pkg.PageRank,
		Exports:    pkg.Exports,
		Imports:    pkg.Imports,
		Dependents: pkg.Dependents,
		Related:    related,
	}
}

func round6(f float64) float64 {
	// reuse math from build; inline here to keep import cycle free
	rounded := f * 1e6
	if rounded < 0 {
		rounded -= 0.5
	} else {
		rounded += 0.5
	}
	return float64(int64(rounded)) / 1e6
}

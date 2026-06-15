package query

import (
	"strings"

	"github.com/rifanid/agentmap/internal/model"
)

const contentLinesLimit = 40

// AnyKind is the resolution kind for --any.
type AnyKind string

const (
	AnyKindPackage    AnyKind = "package"
	AnyKindStructure  AnyKind = "structure"
	AnyKindCandidates AnyKind = "candidates"
	AnyKindContent    AnyKind = "content"
	AnyKindEmpty      AnyKind = "empty"
)

// AnyResult is the result of --any.
type AnyResult struct {
	Query      string        `json:"query"`
	Kind       AnyKind       `json:"kind"`
	// package hit
	Package    string        `json:"package,omitempty"`
	PageRank   float64       `json:"pagerank,omitempty"`
	Exports    []model.Symbol `json:"exports,omitempty"`
	Imports    []string      `json:"imports,omitempty"`
	Dependents []string      `json:"dependents,omitempty"`
	// symbol/feature hits (also present alongside package hit)
	Symbols  []SymbolMatch   `json:"symbols,omitempty"`
	Features []FeatMatch     `json:"features,omitempty"`
	// candidates
	Candidates []string      `json:"candidates,omitempty"`
	// content
	Total int               `json:"total,omitempty"`
	Lines []string          `json:"lines,omitempty"`
}

// FeatMatch is a feature name + package count.
type FeatMatch struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Any is the unified --any router: package → symbol → feature → live git-grep.
func Any(m *model.Map, cwd, q string) (AnyResult, bool) {
	ql := strings.ToLower(q)

	// Collect symbol + feature hits regardless (surfaced alongside a package hit too).
	var symHits []SymbolMatch
	for _, dir := range sortedPackageKeys(m) {
		pkg := m.Packages[dir]
		for _, e := range pkg.Exports {
			if strings.Contains(strings.ToLower(e.Name), ql) {
				symHits = append(symHits, SymbolMatch{Package: dir, File: e.File, Name: e.Name, Kind: e.Kind})
			}
		}
	}
	var featHits []FeatMatch
	for name, files := range m.Features {
		if strings.Contains(strings.ToLower(name), ql) {
			featHits = append(featHits, FeatMatch{Name: name, Count: len(files)})
		}
	}

	pkgKey, candidates := ResolvePackage(m, q)
	if pkgKey != "" {
		pkg := m.Packages[pkgKey]
		return AnyResult{
			Query: q, Kind: AnyKindPackage,
			Package: pkgKey, PageRank: pkg.PageRank,
			Exports: pkg.Exports, Imports: pkg.Imports, Dependents: pkg.Dependents,
			Symbols: symHits, Features: featHits,
		}, true
	}

	if len(symHits) > 0 || len(featHits) > 0 {
		return AnyResult{Query: q, Kind: AnyKindStructure, Symbols: symHits, Features: featHits}, true
	}

	if len(candidates) > 1 {
		return AnyResult{Query: q, Kind: AnyKindCandidates, Candidates: candidates}, true
	}

	// Live git-grep content fallback.
	raw := ContentSearch(cwd, q)
	if raw == "" {
		return AnyResult{Query: q, Kind: AnyKindEmpty}, false
	}
	lines := strings.Split(raw, "\n")
	shown := lines
	if len(shown) > contentLinesLimit {
		shown = shown[:contentLinesLimit]
	}
	return AnyResult{Query: q, Kind: AnyKindContent, Total: len(lines), Lines: shown}, true
}

package query

import (
	"strings"

	"github.com/rifanid98/agentmap-go/internal/model"
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
	// For natural-language queries (multi-word with stopwords), try each
	// meaningful keyword independently before giving up.
	raw := ContentSearch(cwd, q)
	if raw == "" && isNaturalLanguage(q) {
		for _, kw := range keywords(q) {
			if r, ok := Any(m, cwd, kw); ok {
				r.Query = q // keep original query in result
				return r, true
			}
		}
		return AnyResult{Query: q, Kind: AnyKindEmpty}, false
	}
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

// isNaturalLanguage returns true when q looks like a sentence rather than an
// identifier: multiple words where at least one is a common stopword.
func isNaturalLanguage(q string) bool {
	words := strings.Fields(q)
	if len(words) < 2 {
		return false
	}
	for _, w := range words {
		if stopwords[strings.ToLower(w)] {
			return true
		}
	}
	return false
}

// keywords strips stopwords from q and returns the remaining tokens longest-first,
// so the most specific term is tried first.
func keywords(q string) []string {
	words := strings.Fields(q)
	var out []string
	for _, w := range words {
		if !stopwords[strings.ToLower(w)] && len(w) >= 3 {
			out = append(out, w)
		}
	}
	// longest first — more specific identifiers rank higher
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if len(out[j]) > len(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"do": true, "does": true, "did": true,
	"has": true, "have": true, "had": true,
	"can": true, "will": true, "would": true, "could": true, "should": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"and": true, "or": true, "but": true, "not": true, "with": true, "by": true,
	"from": true, "as": true, "into": true, "that": true, "this": true, "it": true,
	"where": true, "what": true, "which": true, "who": true, "how": true, "when": true,
	"why": true, "get": true, "set": true, "use": true, "used": true, "using": true,
	"call": true, "called": true, "handle": true, "handled": true, "made": true,
}

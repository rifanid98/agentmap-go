package query

import (
	"strings"

	"github.com/rifanid/agentmap-go/internal/model"
)

// FindResult is the result of --find.
type FindResult struct {
	Query   string        `json:"query"`
	Matches []SymbolMatch `json:"matches"`
}

// SymbolMatch is one matching symbol.
type SymbolMatch struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
}

// Find searches all package exports for symbols whose name contains q
// (case-insensitive substring). Returns 0 matches → caller exits 1.
func Find(m *model.Map, q string) FindResult {
	ql := strings.ToLower(q)
	var matches []SymbolMatch
	for _, dir := range sortedPackageKeys(m) {
		pkg := m.Packages[dir]
		for _, e := range pkg.Exports {
			if strings.Contains(strings.ToLower(e.Name), ql) {
				matches = append(matches, SymbolMatch{
					Package: dir, File: e.File, Name: e.Name, Kind: e.Kind,
				})
			}
		}
	}
	return FindResult{Query: q, Matches: matches}
}

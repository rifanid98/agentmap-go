package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rifanid98/agentmap-go/internal/model"
	"github.com/rifanid98/agentmap-go/internal/rank"
)

const (
	defaultBudget = 8192
	focusBudget   = 1024
	symsPerPkg    = 8
)

// MapResult is the result of --map.
type MapResult struct {
	Focus  string        `json:"focus"`
	Budget int           `json:"budget"`
	Tokens int           `json:"tokens"`
	Files  []PkgDigest   `json:"files"`
}

// PkgDigest is one package entry in the --map digest.
type PkgDigest struct {
	Package string        `json:"package"`
	Symbols []SymDigest   `json:"symbols"`
}

// SymDigest is a symbol entry within a package digest.
type SymDigest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// tokEst matches agentmap's chars/4 estimate.
func tokEst(s string) int {
	return (len(s) + 3) / 4
}

// MapDigest builds a token-budgeted digest. focus (package dir or "") and
// budget (0 = default). When focus is given, symbol ranking is personalized
// toward that package.
func MapDigest(m *model.Map, focusArg string, budgetArg int) (MapResult, string) {
	focusLabel := "global"
	var focusSet map[string]bool
	var warnMsg string

	if focusArg != "" {
		key, candidates := ResolvePackage(m, focusArg)
		if key != "" {
			focusSet = map[string]bool{key: true}
			focusLabel = key
		} else {
			warnMsg = fmt.Sprintf("--focus %q matched %d packages — using global ranking", focusArg, len(candidates))
		}
	}

	budget := budgetArg
	if budget <= 0 {
		if focusSet != nil {
			budget = focusBudget
		} else {
			budget = defaultBudget
		}
	}

	ranked := rank.Symbols(m.Packages, focusSet)
	if len(ranked) == 0 {
		// Fallback: build digest from file PageRank order when symbol graph is sparse.
		keys := sortedPackageKeys(m)
		sort.Slice(keys, func(i, j int) bool {
			return m.Packages[keys[i]].PageRank > m.Packages[keys[j]].PageRank
		})
		for _, dir := range keys {
			for _, e := range m.Packages[dir].Exports {
				ranked = append(ranked, model.RankedSymbol{File: e.File, Name: e.Name, Kind: e.Kind, Rank: m.Packages[dir].PageRank})
			}
		}
	}

	// Group by package (preserving rank order of first occurrence).
	type entry struct{ pkg string; syms []model.RankedSymbol }
	seen := map[string]int{}
	var byPkg []entry
	for _, s := range ranked {
		// Derive package dir from file path (strip filename).
		pkgDir := pkgOfFile(s.File, m)
		if pkgDir == "" {
			pkgDir = dirOf(s.File)
		}
		if i, ok := seen[pkgDir]; ok {
			byPkg[i].syms = append(byPkg[i].syms, s)
		} else {
			seen[pkgDir] = len(byPkg)
			byPkg = append(byPkg, entry{pkg: pkgDir, syms: []model.RankedSymbol{s}})
		}
	}

	lineOf := func(pkg string, syms []model.RankedSymbol) string {
		var sb strings.Builder
		sb.WriteString("\n")
		sb.WriteString(pkg)
		sb.WriteString(":\n")
		for _, s := range syms {
			fmt.Fprintf(&sb, "  %s (%s)\n", s.Name, s.Kind)
		}
		return sb.String()
	}

	var shown []PkgDigest
	used := 0
	for i, e := range byPkg {
		capped := e.syms
		if len(capped) > symsPerPkg {
			capped = capped[:symsPerPkg]
		}
		line := lineOf(e.pkg, capped)
		t := tokEst(line)
		if used+t > budget {
			if i == 0 && budget > 0 {
				// Never wholly omit the top package — try progressively fewer symbols.
				for k := len(capped) - 1; k >= 1; k-- {
					partial := capped[:k]
					pt := tokEst(lineOf(e.pkg, partial))
					if used+pt <= budget {
						used += pt
						syms := make([]SymDigest, len(partial))
						for i, s := range partial {
							syms[i] = SymDigest{Name: s.Name, Kind: s.Kind}
						}
						shown = append(shown, PkgDigest{Package: e.pkg, Symbols: syms})
						break
					}
				}
			}
			continue
		}
		used += t
		syms := make([]SymDigest, len(capped))
		for i, s := range capped {
			syms[i] = SymDigest{Name: s.Name, Kind: s.Kind}
		}
		shown = append(shown, PkgDigest{Package: e.pkg, Symbols: syms})
	}

	return MapResult{Focus: focusLabel, Budget: budget, Tokens: used, Files: shown}, warnMsg
}

func pkgOfFile(file string, m *model.Map) string {
	dir := dirOf(file)
	if _, ok := m.Packages[dir]; ok {
		return dir
	}
	return ""
}

func dirOf(file string) string {
	if i := strings.LastIndexByte(file, '/'); i >= 0 {
		return file[:i]
	}
	return file
}

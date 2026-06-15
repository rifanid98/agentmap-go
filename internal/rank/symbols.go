package rank

import (
	"math"
	"sort"
	"strings"

	"github.com/rifanid/agentmap-go/internal/graph"
	"github.com/rifanid/agentmap-go/internal/model"
)

// Symbols builds the Aider-style identifier graph from the package map and
// returns a ranked list of exported symbols. focus (set of package dirs), when
// non-empty, personalizes the ranking toward those packages. The graph nodes
// are package dirs; each ranked entry still reports the symbol's defining file.
func Symbols(pkgs map[string]*model.Package, focus map[string]bool) []model.RankedSymbol {
	defines := map[string]map[string]bool{}  // ident → set(pkgDir)
	references := map[string][]string{}       // ident → [pkgDir...] (multiplicity)
	definition := map[string]model.Symbol{}   // "pkgDir|ident" → Symbol
	dirOfKey := map[string]string{}           // "pkgDir|ident" → pkgDir

	for dir, pkg := range pkgs {
		for _, e := range pkg.Exports {
			if defines[e.Name] == nil {
				defines[e.Name] = map[string]bool{}
			}
			defines[e.Name][dir] = true
			key := dir + "|" + e.Name
			definition[key] = e
			dirOfKey[key] = dir
		}
	}
	for dir, pkg := range pkgs {
		for _, tp := range pkg.Imports {
			for _, name := range pkg.ImportedSymbols[tp] {
				references[name] = append(references[name], dir)
			}
		}
	}

	// mentioned idents from focus packages' exports + their dir basenames.
	var mentioned map[string]bool
	if len(focus) > 0 {
		mentioned = map[string]bool{}
		for dir := range focus {
			if pkg := pkgs[dir]; pkg != nil {
				for _, e := range pkg.Exports {
					mentioned[e.Name] = true
				}
			}
			mentioned[lastSeg(dir)] = true
		}
	}

	nodes := sortedKeys(pkgs)

	type symEdge struct {
		from, to, ident string
		weight          float64
	}
	var symEdges []symEdge

	idents := make([]string, 0, len(defines))
	for ident := range defines {
		idents = append(idents, ident)
	}
	sort.Strings(idents)

	for _, ident := range idents {
		refs, ok := references[ident]
		if !ok {
			continue
		}
		mul := identMul(ident, len(defines[ident]), mentioned)
		counts := map[string]int{}
		for _, refFile := range refs {
			counts[refFile]++
		}
		refFiles := make([]string, 0, len(counts))
		for rf := range counts {
			refFiles = append(refFiles, rf)
		}
		sort.Strings(refFiles)
		defFiles := sortedSet(defines[ident])
		for _, refFile := range refFiles {
			n := counts[refFile]
			for _, defFile := range defFiles {
				if refFile == defFile {
					continue
				}
				useMul := mul
				if focus != nil && focus[refFile] {
					useMul *= FocusBoost
				}
				symEdges = append(symEdges, symEdge{
					from: refFile, to: defFile, ident: ident,
					weight: useMul * math.Sqrt(float64(n)),
				})
			}
		}
	}

	// personalization seeds: focus packages + packages whose path matches a mention.
	var pers map[string]float64
	if len(focus) > 0 {
		pers = map[string]float64{}
		unit := 100.0 / float64(len(nodes))
		for _, p := range nodes {
			v := 0.0
			if focus[p] {
				v += unit
			}
			if mentioned != nil {
				parts := map[string]bool{lastSeg(p): true}
				for _, seg := range strings.Split(p, "/") {
					parts[seg] = true
				}
				for part := range parts {
					if mentioned[part] {
						v += unit
						break
					}
				}
			}
			if v > 0 {
				pers[p] = v
			}
		}
		if len(pers) == 0 {
			pers = nil
		}
	}

	gEdges := make([]graph.Edge, len(symEdges))
	for i, e := range symEdges {
		gEdges[i] = graph.Edge{From: e.from, To: e.to, Weight: e.weight}
	}
	r := graph.PageRank(nodes, gEdges, pers)

	// redistribute each node's rank across its out-edges onto (defFile, ident).
	out := map[string]float64{}
	totalW := map[string]float64{}
	for _, e := range symEdges {
		totalW[e.from] += e.weight
	}
	for _, e := range symEdges {
		tw := totalW[e.from]
		if tw == 0 {
			tw = 1
		}
		share := r[e.from] * e.weight / tw
		out[e.to+"|"+e.ident] += share
	}

	type entry struct {
		key  string
		rank float64
	}
	entries := make([]entry, 0, len(out))
	for k, v := range out {
		entries = append(entries, entry{key: k, rank: v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].rank != entries[j].rank {
			return entries[i].rank > entries[j].rank
		}
		return entries[i].key < entries[j].key
	})

	var ranked []model.RankedSymbol
	present := map[string]bool{}
	for _, e := range entries {
		dir := dirOfKey[e.key]
		if focus != nil && focus[dir] {
			continue
		}
		sym, ok := definition[e.key]
		if !ok {
			continue
		}
		present[e.key] = true
		ranked = append(ranked, model.RankedSymbol{
			File: sym.File, Name: sym.Name, Kind: sym.Kind, Rank: round6(e.rank),
		})
	}

	// Aider parity: keep exported symbols nothing imports, with a tiny baseline
	// rank below the lowest real rank, so public-API entry points still surface.
	lowest := 0.0
	if len(ranked) > 0 {
		lowest = ranked[len(ranked)-1].Rank
	}
	baseline := round6(1e-6)
	if lowest-1e-6 > 0 {
		baseline = round6(lowest - 1e-6)
	}
	tailKeys := make([]string, 0, len(definition))
	for k := range definition {
		tailKeys = append(tailKeys, k)
	}
	sort.Strings(tailKeys)
	var tail []model.RankedSymbol
	for _, k := range tailKeys {
		if present[k] {
			continue
		}
		dir := dirOfKey[k]
		if focus != nil && focus[dir] {
			continue
		}
		sym := definition[k]
		tail = append(tail, model.RankedSymbol{File: sym.File, Name: sym.Name, Kind: sym.Kind, Rank: baseline})
	}
	sort.Slice(tail, func(i, j int) bool {
		if tail[i].File != tail[j].File {
			return tail[i].File < tail[j].File
		}
		return tail[i].Name < tail[j].Name
	})

	return append(ranked, tail...)
}

func round6(f float64) float64 { return math.Round(f*1e6) / 1e6 }

func sortedKeys(m map[string]*model.Package) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSet(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func lastSeg(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

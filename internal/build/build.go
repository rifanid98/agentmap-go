// Package build orchestrates a full map build: walk the workspace, parse every
// Go file, resolve imports to local packages across modules, compute package
// PageRank + Aider symbol ranking, group features, and assemble a model.Map.
package build

import (
	"fmt"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rifanid/agentmap/internal/cache"
	"github.com/rifanid/agentmap/internal/feature"
	"github.com/rifanid/agentmap/internal/graph"
	"github.com/rifanid/agentmap/internal/model"
	"github.com/rifanid/agentmap/internal/parse"
	"github.com/rifanid/agentmap/internal/rank"
	"github.com/rifanid/agentmap/internal/resolve"
)

const (
	hubsLimit = 15
)

// Build parses the workspace rooted at (or above) startDir and returns the map.
// Diagnostics (parse skips, dead replaces) are written to stderr, never fatal.
func Build(startDir string) (*model.Map, error) {
	ws := resolve.Discover(startDir)
	for _, w := range ws.Warnings {
		fmt.Fprintf(os.Stderr, "# agentmap: warning: %s\n", w)
	}

	fset := token.NewFileSet()
	// abs dir → rel dir for every package we actually parsed.
	scanned := map[string]string{}
	// rel dir → []*parse.FileInfo
	byDir := map[string][]*parse.FileInfo{}
	seen := map[string]bool{} // abs file path, dedupe overlapping module roots

	for _, modDir := range ws.ModDirs {
		filepath.WalkDir(modDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if skipDir(d.Name()) && path != modDir {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			abs, _ := filepath.Abs(path)
			if seen[abs] {
				return nil
			}
			seen[abs] = true
			relPath := relTo(ws.Root, abs)
			relDir := relTo(ws.Root, filepath.Dir(abs))
			fi := parse.File(fset, abs, relPath, relDir)
			if fi.PkgName == "" && fi.ParseErr != nil {
				fmt.Fprintf(os.Stderr, "# agentmap: skipped %s (parse error: %v)\n", relPath, fi.ParseErr)
				return nil
			}
			if fi.ParseErr != nil {
				fmt.Fprintf(os.Stderr, "# agentmap: partial parse %s (%v)\n", relPath, fi.ParseErr)
			}
			byDir[relDir] = append(byDir[relDir], fi)
			scanned[filepath.Dir(abs)] = relDir
			return nil
		})
	}

	pkgs := map[string]*model.Package{}
	dependents := map[string]map[string]bool{}

	for relDir, files := range byDir {
		pkg := &model.Package{ImportedSymbols: map[string][]string{}}
		exportSeen := map[string]bool{}
		importSet := map[string]bool{}
		for _, fi := range files {
			for _, e := range fi.Exports {
				if exportSeen[e.Name] {
					continue
				}
				exportSeen[e.Name] = true
				pkg.Exports = append(pkg.Exports, e)
			}
			// resolve every import to a local package dir (if any).
			for _, imp := range fi.Imports {
				absTarget, local := ws.ResolveDir(imp.Path)
				if !local {
					continue
				}
				targetRel, ok := scanned[absTarget]
				if !ok || targetRel == relDir {
					continue // not a parsed package, or self-import
				}
				importSet[targetRel] = true
				if names := fi.Refs[imp.Path]; len(names) > 0 {
					pkg.ImportedSymbols[targetRel] = append(pkg.ImportedSymbols[targetRel], names...)
				}
			}
		}
		sort.Slice(pkg.Exports, func(i, j int) bool {
			if pkg.Exports[i].Name != pkg.Exports[j].Name {
				return pkg.Exports[i].Name < pkg.Exports[j].Name
			}
			return pkg.Exports[i].File < pkg.Exports[j].File
		})
		for tp := range importSet {
			pkg.Imports = append(pkg.Imports, tp)
			if dependents[tp] == nil {
				dependents[tp] = map[string]bool{}
			}
			dependents[tp][relDir] = true
		}
		sort.Strings(pkg.Imports)
		pkgs[relDir] = pkg
	}

	for relDir, pkg := range pkgs {
		for dep := range dependents[relDir] {
			pkg.Dependents = append(pkg.Dependents, dep)
		}
		sort.Strings(pkg.Dependents)
	}

	// File/package PageRank: importer→imported, weight = #symbols crossed.
	nodes := make([]string, 0, len(pkgs))
	for d := range pkgs {
		nodes = append(nodes, d)
	}
	sort.Strings(nodes)
	var edges []graph.Edge
	for _, p := range nodes {
		pkg := pkgs[p]
		for _, tp := range pkg.Imports {
			if pkgs[tp] == nil {
				continue
			}
			w := float64(len(pkg.ImportedSymbols[tp]))
			if w == 0 {
				w = 1
			}
			edges = append(edges, graph.Edge{From: p, To: tp, Weight: w})
		}
	}
	pr := graph.PageRank(nodes, edges, nil)
	for _, p := range nodes {
		pkgs[p].PageRank = round6(pr[p])
	}

	rankedSymbols := rank.Symbols(pkgs, nil)
	if len(rankedSymbols) > rank.RankedSymbolsLimit {
		rankedSymbols = rankedSymbols[:rank.RankedSymbolsLimit]
	}

	// hubs: top by PageRank, with raw dependent count shown.
	hubNodes := append([]string(nil), nodes...)
	sort.Slice(hubNodes, func(i, j int) bool {
		a, b := pkgs[hubNodes[i]], pkgs[hubNodes[j]]
		if a.PageRank != b.PageRank {
			return a.PageRank > b.PageRank
		}
		return hubNodes[i] < hubNodes[j]
	})
	var hubs []string
	for i, p := range hubNodes {
		if i >= hubsLimit {
			break
		}
		pkg := pkgs[p]
		hubs = append(hubs, fmt.Sprintf("%s (deg %d, pr %g)", p, len(pkg.Dependents), pkg.PageRank))
	}

	features := map[string][]string{}
	for _, p := range nodes {
		if f := feature.Of(p); f != "" {
			features[f] = append(features[f], p)
		}
	}
	for f := range features {
		sort.Strings(features[f])
	}

	sha := cache.CurrentSha(ws.Root)
	m := &model.Map{
		Schema:        model.SchemaVersion,
		GeneratedSha:  sha,
		Dirty:         cache.DirtyCount(ws.Root),
		PackageCount:  len(pkgs),
		Hubs:          hubs,
		Features:      features,
		RankedSymbols: rankedSymbols,
		Packages:      pkgs,
	}
	if sha == "" {
		m.Fingerprint = cache.Fingerprint(ws.Root)
	}
	return m, nil
}

func round6(f float64) float64 { return math.Round(f*1e6) / 1e6 }

func relTo(root, abs string) string {
	r, err := filepath.Rel(root, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(r)
}

func skipDir(name string) bool {
	switch name {
	case "vendor", "testdata", ".git", "node_modules":
		return true
	}
	return strings.HasPrefix(name, ".") && name != "."
}

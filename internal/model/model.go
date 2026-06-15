// Package model holds the shared data types for the agentmap map and its
// on-disk JSON schema. Keeping them in one dependency-free package avoids
// import cycles between parse / resolve / build / cache / query.
package model

// SchemaVersion is bumped whenever the on-disk map.json shape changes so a
// stale cache from an older binary is ignored and rebuilt. Go port starts at 1
// (the schema diverges from the JS tool's schema 3 — see README parity notes).
const SchemaVersion = 1

// Symbol is one exported declaration. File is the repo-relative path of the
// file that defines it (so --find / --symbols / --map stay file-precise even
// though the graph node is a package). Recv is the receiver type name for
// methods (empty otherwise); Name for a method is "Recv.Method".
type Symbol struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	File string `json:"file"`
	Recv string `json:"recv,omitempty"`
}

// Package is a graph node: a Go package directory (repo-relative). Imports and
// Dependents are other package dirs. ImportedSymbols maps an imported package
// dir to the exported names this package references from it (multiplicity
// preserved), which feeds edge weights and the symbol-rank identifier graph.
type Package struct {
	Exports         []Symbol            `json:"exports"`
	Imports         []string            `json:"imports"`
	ImportedSymbols map[string][]string `json:"importedSymbols"`
	Dependents      []string            `json:"dependents"`
	PageRank        float64             `json:"pagerank"`
}

// RankedSymbol is one entry of the Aider-style globally ranked symbol list.
type RankedSymbol struct {
	File string  `json:"file"`
	Name string  `json:"name"`
	Kind string  `json:"kind"`
	Rank float64 `json:"rank"`
}

// Map is the full cached artifact persisted to .claude/agentmap/map.json.
type Map struct {
	Schema        int                 `json:"schema"`
	GeneratedSha  string              `json:"generatedSha"`
	Dirty         int                 `json:"dirty"`
	PackageCount  int                 `json:"packageCount"`
	Fingerprint   string              `json:"fingerprint,omitempty"`
	Hubs          []string            `json:"hubs"`
	Features      map[string][]string `json:"features"`
	RankedSymbols []RankedSymbol      `json:"rankedSymbols"`
	Packages      map[string]*Package `json:"packages"`
}

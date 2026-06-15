// Package parse extracts, per Go source file, the data agentmap needs to build
// its graph: exported symbols, imports (alias → import path), and references to
// other packages' exported symbols (qualified pkg.Symbol selectors).
//
// It uses go/parser + go/ast only — no type checking, no build. A malformed
// file yields a partial AST plus an error; the caller keeps the partial result
// and logs the error, preserving agentmap's graceful-degradation contract.
package parse

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"

	"github.com/rifanid/agentmap-go/internal/model"
)

// ImportSpec is one import line: its local binding (Alias) and the import path.
// Alias is "" when none was written (binding = the package's declared name),
// "." for a dot import, or "_" for a blank import.
type ImportSpec struct {
	Alias string
	Path  string
}

// FileInfo is the parse result for a single .go file.
type FileInfo struct {
	Path     string              // repo-relative file path
	Dir      string              // repo-relative directory (the package node key)
	PkgName  string              // declared package name (file.Name.Name)
	Exports  []model.Symbol      // exported declarations defined in this file
	Imports  []ImportSpec        // import block entries
	Refs     map[string][]string // import path → referenced exported names (multiplicity kept)
	ParseErr error               // non-nil if the file failed to fully parse
}

// File parses one file. relPath/relDir are the repo-relative path + dir used as
// keys throughout the map. Returns a FileInfo even on parse error (partial).
func File(fset *token.FileSet, absPath, relPath, relDir string) *FileInfo {
	src, err := os.ReadFile(absPath)
	if err != nil {
		return &FileInfo{Path: relPath, Dir: relDir, ParseErr: err}
	}
	f, perr := parser.ParseFile(fset, absPath, src, parser.ParseComments|parser.SkipObjectResolution)
	if f == nil {
		return &FileInfo{Path: relPath, Dir: relDir, ParseErr: perr}
	}
	info := &FileInfo{
		Path:     relPath,
		Dir:      relDir,
		PkgName:  f.Name.Name,
		Refs:     map[string][]string{},
		ParseErr: perr,
	}

	// alias → import path, for resolving selector references below.
	aliasToPath := map[string]string{}
	for _, imp := range f.Imports {
		path := importPath(imp)
		if path == "" {
			continue
		}
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		info.Imports = append(info.Imports, ImportSpec{Alias: alias, Path: path})
		switch alias {
		case "", "_", ".":
			// "" handled below (defer to last path segment); blank/dot have no qualifier
		default:
			aliasToPath[alias] = path
		}
		// For an unaliased import the qualifier is the package's declared name.
		// We don't know it without parsing the target, so default to the last
		// path segment — correct for the overwhelming majority of packages.
		if alias == "" {
			aliasToPath[lastSegment(path)] = path
		}
	}

	// Exported declarations.
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			collectFunc(info, d)
		case *ast.GenDecl:
			collectGen(info, d)
		}
	}

	// Qualified references: pkg.Name where pkg is an imported alias and Name is
	// exported. Feeds edge weights + the symbol-rank reference graph.
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		path, ok := aliasToPath[x.Name]
		if !ok {
			return true
		}
		if ast.IsExported(sel.Sel.Name) {
			info.Refs[path] = append(info.Refs[path], sel.Sel.Name)
		}
		return true
	})

	return info
}

func importPath(imp *ast.ImportSpec) string {
	if imp.Path == nil {
		return ""
	}
	v := imp.Path.Value
	if len(v) >= 2 && (v[0] == '"' || v[0] == '`') {
		return v[1 : len(v)-1]
	}
	return v
}

func lastSegment(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

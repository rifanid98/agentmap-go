package parse

import (
	"go/ast"
	"go/token"

	"github.com/rifanid98/agentmap-go/internal/model"
)

// collectFunc appends an exported func/method symbol. A method (Recv != nil) is
// named "Recv.Method" and carries Recv so --find/--symbols disambiguate methods
// that share a name across types.
func collectFunc(info *FileInfo, d *ast.FuncDecl) {
	if d.Name == nil || !ast.IsExported(d.Name.Name) {
		return
	}
	if d.Recv == nil || len(d.Recv.List) == 0 {
		info.Exports = append(info.Exports, model.Symbol{Name: d.Name.Name, Kind: "function", File: info.Path})
		return
	}
	recv := receiverName(d.Recv.List[0].Type)
	name := d.Name.Name
	if recv != "" {
		name = recv + "." + d.Name.Name
	}
	info.Exports = append(info.Exports, model.Symbol{Name: name, Kind: "method", File: info.Path, Recv: recv})
}

// receiverName unwraps *T / T and returns the type name (generic params dropped).
func receiverName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // generic receiver: T[P]
		return receiverName(t.X)
	case *ast.IndexListExpr: // generic receiver: T[P, Q]
		return receiverName(t.X)
	}
	return ""
}

// collectGen appends exported type/const/var symbols from a GenDecl.
func collectGen(info *FileInfo, d *ast.GenDecl) {
	switch d.Tok {
	case token.TYPE:
		for _, spec := range d.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name == nil || !ast.IsExported(ts.Name.Name) {
				continue
			}
			info.Exports = append(info.Exports, model.Symbol{Name: ts.Name.Name, Kind: typeKind(ts), File: info.Path})
		}
	case token.CONST, token.VAR:
		kind := "const"
		if d.Tok == token.VAR {
			kind = "var"
		}
		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name != nil && ast.IsExported(name.Name) {
					info.Exports = append(info.Exports, model.Symbol{Name: name.Name, Kind: kind, File: info.Path})
				}
			}
		}
	}
}

func typeKind(ts *ast.TypeSpec) string {
	if ts.Assign.IsValid() { // type X = Y
		return "alias"
	}
	switch ts.Type.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	}
	return "type"
}

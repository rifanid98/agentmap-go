package parse_test

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/rifanid/agentmap-go/internal/parse"
)

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseExports(t *testing.T) {
	dir := t.TempDir()
	src := `package orders

type Order struct { ID string }
type Repo interface { Save(*Order) error }
func NewOrder(id string) *Order { return &Order{ID: id} }
func (o *Order) Cancel() error { return nil }
const StatusPending = "pending"
var DefaultTimeout = 30
`
	path := writeFile(t, filepath.Join(dir, "order.go"), src)
	fset := token.NewFileSet()
	fi := parse.File(fset, path, "orders/order.go", "orders")
	if fi.ParseErr != nil {
		t.Fatalf("unexpected parse error: %v", fi.ParseErr)
	}
	byName := map[string]string{}
	for _, e := range fi.Exports {
		byName[e.Name] = e.Kind
	}
	cases := []struct{ name, kind string }{
		{"Order", "struct"},
		{"Repo", "interface"},
		{"NewOrder", "function"},
		{"Order.Cancel", "method"},
		{"StatusPending", "const"},
		{"DefaultTimeout", "var"},
	}
	for _, c := range cases {
		if got := byName[c.name]; got != c.kind {
			t.Errorf("Export %q: got kind %q, want %q", c.name, got, c.kind)
		}
	}
}

func TestParseMalformed(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, filepath.Join(dir, "bad.go"), "package bad\nfunc Broken( {}\n")
	fset := token.NewFileSet()
	fi := parse.File(fset, path, "bad/bad.go", "bad")
	// Must not return nil; partial parse is acceptable.
	if fi == nil {
		t.Fatal("expected non-nil FileInfo even on parse error")
	}
	// ParseErr should be set.
	if fi.ParseErr == nil {
		t.Error("expected ParseErr to be non-nil for malformed source")
	}
}

func TestParseImportRefs(t *testing.T) {
	dir := t.TempDir()
	src := `package handler
import (
	"example.com/go-core/orders/usecase"
	entity "example.com/go-core/orders/domain/entity"
)
type Handler struct{}
func (h *Handler) Do() *entity.Order { return usecase.NewService().Create("x") }
`
	path := writeFile(t, filepath.Join(dir, "handler.go"), src)
	fset := token.NewFileSet()
	fi := parse.File(fset, path, "handler/handler.go", "handler")
	if fi.ParseErr != nil {
		t.Fatalf("unexpected parse error: %v", fi.ParseErr)
	}
	// Should have 2 imports
	if len(fi.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(fi.Imports))
	}
	// Refs to entity package should include "Order"
	entityRefs := fi.Refs["example.com/go-core/orders/domain/entity"]
	found := false
	for _, r := range entityRefs {
		if r == "Order" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ref to 'Order' in entity package refs, got %v", entityRefs)
	}
}

func TestParseUnexported(t *testing.T) {
	dir := t.TempDir()
	src := `package util
func helper() {}
type internal struct{}
const privateConst = 1
`
	path := writeFile(t, filepath.Join(dir, "util.go"), src)
	fset := token.NewFileSet()
	fi := parse.File(fset, path, "util/util.go", "util")
	if len(fi.Exports) != 0 {
		t.Errorf("expected 0 exports for unexported symbols, got %v", fi.Exports)
	}
}

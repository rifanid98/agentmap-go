package resolve_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rifanid/agentmap-go/internal/resolve"
)

// makeFixture creates a minimal go.work monorepo in a temp dir.
func makeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// module A: go-core
	modA := filepath.Join(dir, "go-core")
	os.MkdirAll(filepath.Join(modA, "orders/entity"), 0o755)
	os.WriteFile(filepath.Join(modA, "go.mod"), []byte("module example.com/go-core\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(modA, "orders/entity/order.go"), []byte("package entity\n"), 0o644)

	// module B: service, replaces go-core with local path
	modB := filepath.Join(dir, "svc")
	os.MkdirAll(filepath.Join(modB, "handler"), 0o755)
	os.WriteFile(filepath.Join(modB, "go.mod"), []byte(
		"module example.com/svc\n\ngo 1.21\n\nrequire example.com/go-core v0.0.0\n\nreplace example.com/go-core => ../go-core\n",
	), 0o644)
	os.WriteFile(filepath.Join(modB, "handler/h.go"), []byte("package handler\n"), 0o644)

	// go.work
	os.WriteFile(filepath.Join(dir, "go.work"), []byte("go 1.21\n\nuse (\n\t./go-core\n\t./svc\n)\n"), 0o644)
	return dir
}

func TestDiscoverWork(t *testing.T) {
	dir := makeFixture(t)
	ws := resolve.Discover(dir)
	if !ws.IsWork {
		t.Fatal("expected IsWork=true")
	}
	if ws.Root != dir {
		t.Errorf("Root=%q want %q", ws.Root, dir)
	}
}

func TestResolveCrossModule(t *testing.T) {
	dir := makeFixture(t)
	ws := resolve.Discover(dir)

	// Resolving go-core's import path from inside the service.
	resolvedDir, local := ws.ResolveDir("example.com/go-core/orders/entity")
	if !local {
		t.Fatal("expected local=true for go-core import")
	}
	want := filepath.Join(dir, "go-core", "orders", "entity")
	if resolvedDir != want {
		t.Errorf("ResolveDir=%q want %q", resolvedDir, want)
	}
}

func TestResolveStdlib(t *testing.T) {
	if !resolve.IsStdlib("fmt") {
		t.Error("fmt should be stdlib")
	}
	if !resolve.IsStdlib("net/http") {
		t.Error("net/http should be stdlib")
	}
	if resolve.IsStdlib("github.com/foo/bar") {
		t.Error("github.com/... should not be stdlib")
	}
}

func TestResolveMissingReplaceTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(
		"module example.com/svc\n\ngo 1.21\n\nrequire example.com/gone v0.0.0\n\nreplace example.com/gone => ../nonexistent\n",
	), 0o644)
	ws := resolve.Discover(dir)
	if len(ws.Warnings) == 0 {
		t.Error("expected warning for dead replace target")
	}
	_, local := ws.ResolveDir("example.com/gone/pkg")
	if local {
		t.Error("dead replace target should resolve as external, not local")
	}
}

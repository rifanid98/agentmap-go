// Package resolve discovers a Go workspace (go.work + go.mod, including replace
// directives) and resolves import paths to local package directories across
// modules. This cross-module resolution is agentmap-go's headline feature: it
// is what lets --relates compute blast radius across a monorepo of services
// wired together by replace directives.
package resolve

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

// Module is one locally-available module: its declared module path and the
// absolute directory of its root.
type Module struct {
	Path string
	Dir  string
}

// Workspace is the resolved view of the repo.
type Workspace struct {
	Root     string   // absolute workspace root (go.work dir, or the single go.mod dir)
	IsWork   bool     // true if a go.work was found
	ModDirs  []string // absolute module root dirs to scan for .go files
	mods     []Module // module-path → dir, sorted longest-path-first
	Warnings []string // non-fatal issues (e.g. dead replace targets)
}

// Discover finds the workspace starting at startDir (typically cwd). It looks
// upward for a go.work; failing that, for a go.mod. Returns a usable Workspace
// even when neither is found (Root = startDir, no modules) so callers degrade.
func Discover(startDir string) *Workspace {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		abs = startDir
	}
	if workFile := findUp(abs, "go.work"); workFile != "" {
		if ws := fromWork(workFile); ws != nil {
			return ws
		}
	}
	if modFile := findUp(abs, "go.mod"); modFile != "" {
		if ws := fromMod(modFile); ws != nil {
			return ws
		}
	}
	return &Workspace{Root: abs}
}

func fromWork(workFile string) *Workspace {
	data, err := os.ReadFile(workFile)
	if err != nil {
		return nil
	}
	wf, err := modfile.ParseWork(workFile, data, nil)
	if err != nil {
		return nil
	}
	root := filepath.Dir(workFile)
	ws := &Workspace{Root: root, IsWork: true}
	modByPath := map[string]string{} // module path → dir (later writes win = higher precedence)
	dirSet := map[string]bool{}

	// 1) use directives → module dirs
	for _, u := range wf.Use {
		dir := absJoin(root, u.Path)
		if !exists(dir) {
			ws.Warnings = append(ws.Warnings, "go.work use path missing: "+u.Path)
			continue
		}
		dirSet[dir] = true
		if mp := moduleName(dir); mp != "" {
			modByPath[mp] = dir
		}
		// 2) per-module replaces of the used module
		applyModReplaces(dir, modByPath, dirSet, ws)
	}
	// 3) go.work replaces override everything
	for _, r := range wf.Replace {
		applyReplace(root, r.Old.Path, r.New.Path, modByPath, dirSet, ws)
	}
	finalize(ws, modByPath, dirSet)
	return ws
}

func fromMod(modFile string) *Workspace {
	dir := filepath.Dir(modFile)
	ws := &Workspace{Root: dir}
	modByPath := map[string]string{}
	dirSet := map[string]bool{dir: true}
	if mp := moduleName(dir); mp != "" {
		modByPath[mp] = dir
	}
	applyModReplaces(dir, modByPath, dirSet, ws)
	finalize(ws, modByPath, dirSet)
	return ws
}

// applyModReplaces reads dir/go.mod and records any replace whose target is a
// local path (the directive.sh wiring HappyKids services use to point at a
// local go-core worktree).
func applyModReplaces(dir string, modByPath map[string]string, dirSet map[string]bool, ws *Workspace) {
	mf := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(mf)
	if err != nil {
		return
	}
	f, err := modfile.Parse(mf, data, nil)
	if err != nil {
		return
	}
	for _, r := range f.Replace {
		applyReplace(dir, r.Old.Path, r.New.Path, modByPath, dirSet, ws)
	}
}

func applyReplace(base, oldPath, newPath string, modByPath map[string]string, dirSet map[string]bool, ws *Workspace) {
	if !isLocalPath(newPath) {
		return // version replace (e.g. => v1.2.3), not a local dir
	}
	target := absJoin(base, newPath)
	if !exists(target) {
		ws.Warnings = append(ws.Warnings, "replace target missing: "+oldPath+" => "+newPath)
		return
	}
	dirSet[target] = true
	modByPath[oldPath] = target
}

func finalize(ws *Workspace, modByPath map[string]string, dirSet map[string]bool) {
	for p, d := range modByPath {
		ws.mods = append(ws.mods, Module{Path: p, Dir: d})
	}
	// longest module path first so ResolveDir picks the most specific module.
	sort.Slice(ws.mods, func(i, j int) bool {
		if len(ws.mods[i].Path) != len(ws.mods[j].Path) {
			return len(ws.mods[i].Path) > len(ws.mods[j].Path)
		}
		return ws.mods[i].Path < ws.mods[j].Path
	})
	for d := range dirSet {
		ws.ModDirs = append(ws.ModDirs, d)
	}
	sort.Strings(ws.ModDirs)
}

// ResolveDir maps an import path to an absolute local package directory. local
// is false for stdlib / third-party (module-cache) imports.
func (ws *Workspace) ResolveDir(importPath string) (dir string, local bool) {
	for _, m := range ws.mods {
		if importPath == m.Path {
			return m.Dir, true
		}
		if strings.HasPrefix(importPath, m.Path+"/") {
			suffix := importPath[len(m.Path)+1:]
			return filepath.Join(m.Dir, filepath.FromSlash(suffix)), true
		}
	}
	return "", false
}

// IsStdlib reports whether an import path is a standard-library package (its
// first path segment contains no dot — the same heuristic the go tool uses).
func IsStdlib(importPath string) bool {
	first := importPath
	if i := strings.IndexByte(importPath, '/'); i >= 0 {
		first = importPath[:i]
	}
	return !strings.Contains(first, ".")
}

func moduleName(dir string) string {
	mf := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(mf)
	if err != nil {
		return ""
	}
	return modfile.ModulePath(data)
}

func isLocalPath(p string) bool {
	return strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") ||
		p == "." || p == ".." || filepath.IsAbs(p)
}

func absJoin(base, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(base, filepath.FromSlash(p)))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// findUp walks up from dir looking for a file named name; returns its full path
// or "" if not found before the filesystem root.
func findUp(dir, name string) string {
	for {
		candidate := filepath.Join(dir, name)
		if exists(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

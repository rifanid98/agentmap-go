// Package cache handles the on-disk map (.claude/agentmap/map.json): atomic
// load/save plus the git-based freshness signals (short SHA + dirty count) and
// the non-git source fingerprint. Freshness semantics mirror agentmap exactly.
package cache

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rifanid/agentmap/internal/model"
)

// MapPath is the cache location relative to the workspace root.
const MapPath = ".claude/agentmap/map.json"

const maxBuf = 64 * 1024 * 1024

// CurrentSha returns `git rev-parse --short HEAD` run in root, or "" (non-git).
func CurrentSha(root string) string {
	return gitOut(root, "rev-parse", "--short", "HEAD")
}

// DirtyCount counts uncommitted source (.go) changes, mirroring agentmap's
// porcelain parse (rename/copy handling, unquoting). vendor/ and testdata/ are
// excluded; _test.go IS counted so a test edit still invalidates the cache.
func DirtyCount(root string) int {
	out := gitOut(root, "status", "--porcelain", "--untracked-files=all")
	if out == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		xy := line[:2]
		p := line[3:]
		if (strings.ContainsAny(xy, "RC")) && strings.Contains(p, " -> ") {
			parts := strings.Split(p, " -> ")
			p = parts[len(parts)-1]
		}
		p = strings.Trim(p, `"`)
		if isSourcePath(p) {
			count++
		}
	}
	return count
}

func isSourcePath(p string) bool {
	if !strings.HasSuffix(p, ".go") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "vendor" || seg == "testdata" {
			return false
		}
	}
	return true
}

// Fingerprint is a best-effort content signature for non-git repos: sha1 over
// sorted "path:mtimeNano:size" of every .go file (skips vendor/.git/symlinks).
func Fingerprint(root string) string {
	var entries []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 40 {
			return
		}
		names, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range names {
			name := e.Name()
			if name == "vendor" || name == ".git" || name == "node_modules" || name == "testdata" {
				continue
			}
			full := filepath.Join(dir, name)
			info, err := os.Lstat(full)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if info.IsDir() {
				walk(full, depth+1)
			} else if strings.HasSuffix(name, ".go") {
				entries = append(entries, fmt.Sprintf("%s:%d:%d", full, info.ModTime().UnixNano(), info.Size()))
			}
		}
	}
	walk(root, 0)
	sort.Strings(entries)
	sum := sha1.Sum([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

// Load reads and decodes the cached map from root, or returns (nil, err).
func Load(root string) (*model.Map, error) {
	data, err := os.ReadFile(filepath.Join(root, MapPath))
	if err != nil {
		return nil, err
	}
	var m model.Map
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes the map atomically (tmp + rename) under root.
func Save(root string, m *model.Map) error {
	dir := filepath.Join(root, ".claude", "agentmap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := filepath.Join(root, MapPath+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(root, MapPath))
}

func gitOut(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var buf bytes.Buffer
	buf.Grow(4096)
	cmd.Stdout = &limitedWriter{buf: &buf, max: maxBuf}
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

type limitedWriter struct {
	buf *bytes.Buffer
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		p = p[:w.max-w.buf.Len()]
	}
	return w.buf.Write(p)
}

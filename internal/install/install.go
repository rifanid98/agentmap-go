// Package install provides shared utilities for --install-hooks, --install-skill,
// and --setup-mcp: atomic file writes, JSONC parsing, and docs block merging.
package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AtomicWrite writes data to path via a tmp file + rename (prevents torn reads).
// mode is only applied on creation; chmod is called explicitly to bypass umask.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// ReadJSONC parses a JSONC file (JSON with // and /* */ comments). Returns nil
// map + error if the file doesn't exist or is invalid JSON after stripping.
func ReadJSONC(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	clean := StripJSONComments(string(data))
	var m map[string]any
	if err := json.Unmarshal([]byte(clean), &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON — fix or remove it, then re-run: %w", path, err)
	}
	return m, nil
}

// StripJSONComments removes // line comments and /* */ block comments from a
// JSONC string without touching comment-like text inside double-quoted strings.
// Single-pass state machine, ported from agentmap's stripJsonComments.
func StripJSONComments(src string) string {
	out := make([]byte, 0, len(src))
	inStr, esc, inLine, inBlock := false, false, false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		var n byte
		if i+1 < len(src) {
			n = src[i+1]
		}
		if inLine {
			if c == '\n' {
				inLine = false
				out = append(out, c)
			}
			continue
		}
		if inBlock {
			if c == '*' && n == '/' {
				inBlock = false
				i++
			}
			continue
		}
		if inStr {
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			out = append(out, c)
			continue
		}
		if c == '/' && n == '/' {
			inLine = true
			i++
			continue
		}
		if c == '/' && n == '*' {
			inBlock = true
			i++
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

var (
	docsBeginRe = regexp.MustCompile(`(?m)^<!-- agentmap:begin -->.*?<!-- agentmap:end -->`)
	docsBegin   = "<!-- agentmap:begin -->"
	docsEnd     = "<!-- agentmap:end -->"
)

// MergeDocsBlock merges content between agentmap marker comments into the
// dest file. If the markers exist they are replaced; otherwise the block is
// appended. Idempotent.
func MergeDocsBlock(destPath string, content []byte) error {
	block := docsBegin + "\n" + string(content) + "\n" + docsEnd
	existing, err := os.ReadFile(destPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var result string
	if len(existing) > 0 {
		s := docsBeginRe.ReplaceAllString(string(existing), block)
		if s == string(existing) {
			// markers not found — append
			result = strings.TrimRight(s, "\n") + "\n\n" + block + "\n"
		} else {
			result = s
		}
	} else {
		result = block + "\n"
	}
	return os.WriteFile(destPath, []byte(result), 0o644)
}

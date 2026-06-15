// Package query implements all agentmap query commands (--any, --find,
// --relates, --map, --symbols, --feature, --features, --hubs, --print).
package query

import (
	"strings"

	"github.com/rifanid/agentmap-go/internal/model"
)

// ResolvePackage maps a query string to a package key in PREFERENCE order:
// (a) exact key, (b) unique case-insensitive basename match,
// (c) unique case-insensitive substring match, (d) multiple candidates.
// Mirrors agentmap's resolveFile logic exactly.
func ResolvePackage(m *model.Map, q string) (key string, candidates []string) {
	ql := strings.ToLower(q)
	keys := sortedPackageKeys(m)

	// (a) exact
	if _, ok := m.Packages[q]; ok {
		return q, nil
	}
	// (b) case-insensitive basename
	var baseMatch []string
	for _, k := range keys {
		base := lastSeg(k)
		if strings.ToLower(base) == ql {
			baseMatch = append(baseMatch, k)
		}
	}
	if len(baseMatch) == 1 {
		return baseMatch[0], nil
	}
	// (c) substring
	var subs []string
	for _, k := range keys {
		if strings.Contains(strings.ToLower(k), ql) {
			subs = append(subs, k)
		}
	}
	if len(subs) == 1 {
		return subs[0], nil
	}
	return "", subs
}

func lastSeg(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func sortedPackageKeys(m *model.Map) []string {
	keys := make([]string, 0, len(m.Packages))
	for k := range m.Packages {
		keys = append(keys, k)
	}
	// stable sort for deterministic results
	sortStrings(keys)
	return keys
}

func sortStrings(s []string) {
	// inline to avoid importing sort everywhere
	n := len(s)
	for i := 1; i < n; i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

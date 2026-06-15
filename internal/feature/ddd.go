// Package feature groups packages into "features" by DDD domain. A feature is
// the path segment that owns a DDD layer subtree — e.g. given
// "go-core/orders/domain/entity" or "go-core/orders/usecase", the segment
// before the first DDD-layer marker ("domain"/"usecase") is "orders", so both
// belong to feature "orders". This replaces agentmap's Next.js app/ route
// segments (see README parity notes).
package feature

import "strings"

// layerMarkers are the DDD/clean-architecture layer directory names whose
// PARENT segment is the domain feature name.
var layerMarkers = map[string]bool{
	"domain":         true,
	"usecase":        true,
	"usecases":       true,
	"repository":     true,
	"repositories":   true,
	"delivery":       true,
	"handler":        true,
	"handlers":       true,
	"port":           true,
	"ports":          true,
	"infrastructure": true,
	"transport":      true,
	"adapter":        true,
	"adapters":       true,
	"service":        true,
	"services":       true,
}

// Of returns the DDD domain feature for a package dir, or "" if none applies.
// It finds the first layer marker in the path and returns the segment before it.
func Of(dir string) string {
	segs := strings.Split(dir, "/")
	for i, s := range segs {
		if layerMarkers[s] && i > 0 {
			prev := segs[i-1]
			// Skip generic container segments so we don't name a feature
			// "internal" or "pkg"; walk further back if needed.
			if isGenericContainer(prev) && i >= 2 {
				return segs[i-2]
			}
			return prev
		}
	}
	return ""
}

func isGenericContainer(s string) bool {
	switch s {
	case "internal", "pkg", "src", "app", "cmd":
		return true
	}
	return false
}

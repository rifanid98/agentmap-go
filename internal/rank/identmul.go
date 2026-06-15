// Package rank builds the Aider-style identifier graph over the package map and
// produces a ranked list of exported symbols. Ported from agentmap's
// rankSymbols/identMul with identical tuning constants.
package rank

import "strings"

// Edge-weight multipliers (Aider parity).
const (
	IdentBoost         = 10.0
	RarePenalty        = 0.1
	UnderscorePenalty  = 0.1
	MinIdentLen        = 8
	RareDefiners       = 5
	FocusBoost         = 50.0
	RankedSymbolsLimit = 80
)

// identMul returns the edge-weight multiplier for an identifier. mentioned =
// focus/query idents (boosted). Rarity is approximated by the >RareDefiners
// penalty. The receiver-qualified portion of a method name ("Recv.Method") is
// reduced to its final segment so the camel/length heuristics apply to the
// method identifier itself.
func identMul(ident string, defineCount int, mentioned map[string]bool) float64 {
	mul := 1.0
	bare := ident
	if i := strings.LastIndexByte(bare, '.'); i >= 0 {
		bare = bare[i+1:]
	}
	hasAlpha := strings.ContainsFunc(bare, isAlpha)
	isSnake := strings.Contains(bare, "_") && hasAlpha
	isKebab := strings.Contains(bare, "-") && hasAlpha
	isCamel := strings.ContainsFunc(bare, isLower) && strings.ContainsFunc(bare, isUpper)
	if mentioned != nil && mentioned[ident] {
		mul *= IdentBoost
	}
	if (isSnake || isKebab || isCamel) && len(bare) >= MinIdentLen {
		mul *= IdentBoost
	}
	if strings.HasPrefix(bare, "_") {
		mul *= UnderscorePenalty
	}
	if defineCount > RareDefiners {
		mul *= RarePenalty
	}
	return mul
}

func isAlpha(r rune) bool { return isLower(r) || isUpper(r) }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

// Package hooks implements agentmap's agent-hook integrations: the nudge
// subcommand (PreToolUse / BeforeTool) and the install/status helpers.
package hooks

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
)

// Nudge reads a PreToolUse (or Gemini BeforeTool) JSON event from r and writes
// the hook response to w. gemini=true uses the Gemini BeforeTool shape.
// Always exits 0; never blocks the tool call.
func Nudge(r io.Reader, w io.Writer, gemini bool) {
	data, err := io.ReadAll(r)
	if err != nil || len(data) == 0 {
		if gemini {
			w.Write([]byte("{}\n"))
		}
		return
	}
	var ev map[string]any
	if err := json.Unmarshal(data, &ev); err != nil {
		if gemini {
			w.Write([]byte("{}\n"))
		}
		return
	}

	toolName := strings.ToLower(strField(ev, "tool_name"))
	inp, _ := ev["tool_input"].(map[string]any)

	var pattern, command string
	if inp != nil {
		pattern = coalesce(strField(inp, "pattern"), strField(inp, "query"), strField(inp, "content"))
		command = coalesce(strField(inp, "command"), strField(inp, "cmd"))
	}

	fire := false
	if gemini {
		isGrepLike := reGeminiGrep.MatchString(toolName)
		isBashLike := reGeminiBash.MatchString(toolName)
		q := coalesce(pattern, command)
		if isGrepLike && shouldNudgeGrep(q) {
			fire = true
		}
		if isBashLike && shouldNudgeBash(command) {
			fire = true
		}
	} else {
		switch toolName {
		case "grep":
			fire = shouldNudgeGrep(pattern)
		case "bash":
			fire = shouldNudgeBash(command)
		}
	}

	if !fire {
		if gemini {
			w.Write([]byte("{}\n"))
		}
		return
	}

	eventName := "PreToolUse"
	if gemini {
		eventName = "BeforeTool"
	}
	msg := nudgeMessage()
	resp := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     eventName,
			"additionalContext": msg,
		},
	}
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	w.Write(b)
}

// ---- heuristics (ported from agentmap-nudge.mjs) ----

var (
	reDep = regexp.MustCompile(`(?i)\b(import|require|imported by|depends|dependency)\b|from ["']|(^|\|) export\b`)
	// Go-specific exported-identifier signal (replaces JSX COMPONENT_TAG_RE):
	// a multi-hump PascalCase or UpperCamel identifier like OrderService or NewOrder.
	reGoIdent   = regexp.MustCompile(`\b[A-Z][a-z0-9]+[A-Z][A-Za-z0-9]*\b`)
	reIntent    = regexp.MustCompile(`(?i)\bwhere is\b|\bwho (imports|uses|calls)\b|\breuse\b|\b(existing|shared) (util|package|interface|handler|repo|usecase)\b`)
	reSearcher  = regexp.MustCompile(`(^|[;&]\s*)(rg|ripgrep|grep|egrep|fgrep|ag|ack)\b`)
	reDataFile  = regexp.MustCompile(`\.(log|txt|json|md|yaml|yml|csv)\b`)
	reGeminiGrep = regexp.MustCompile(`(?i)grep|search|ripgrep`)
	reGeminiBash = regexp.MustCompile(`(?i)shell|bash|terminal|command`)
)

func shouldNudgeGrep(pattern string) bool {
	if len(pattern) > 2000 || pattern == "" {
		return false
	}
	return reDep.MatchString(pattern) || reGoIdent.MatchString(pattern) || reIntent.MatchString(pattern)
}

func shouldNudgeBash(cmd string) bool {
	if cmd == "" {
		return false
	}
	// Only fire on a primary grep command (not piped).
	if !reSearcher.MatchString(cmd) {
		return false
	}
	// Data-file guard: looks like log filtering, stay silent.
	if reDataFile.MatchString(cmd) {
		return false
	}
	return reDep.MatchString(cmd) || reGoIdent.MatchString(cmd) || reIntent.MatchString(cmd) || reSearcher.MatchString(cmd)
}

func nudgeMessage() string {
	return "This looks like a dependency / blast-radius / symbol-location search. " +
		"Use agentmap FIRST — it's faster and uses far fewer tokens.\n\n" +
		"  agentmap --any <query>       # package → symbol → feature → git-grep\n" +
		"  agentmap --relates <pkg>     # blast radius (imports + dependents)\n" +
		"  agentmap --find <symbol>     # exported symbol by name\n" +
		"  agentmap --map               # token-budgeted digest\n\n" +
		"Run `agentmap --help` for the full command list."
}

// RunNudge is called from main for the `nudge [--gemini]` subcommand.
func RunNudge(args []string) int {
	gemini := false
	for _, a := range args {
		if a == "--gemini" {
			gemini = true
		}
	}
	Nudge(os.Stdin, os.Stdout, gemini)
	return 0
}

func strField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func coalesce(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

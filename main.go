// agentmap — the queryable, ranked repo map your coding agent is forced to use.
// Native Go port with Go workspace/monorepo support. See README for parity notes.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rifanid/agentmap/internal/build"
	"github.com/rifanid/agentmap/internal/hooks"
	"github.com/rifanid/agentmap/internal/install"
	"github.com/rifanid/agentmap/internal/mcp"
	"github.com/rifanid/agentmap/internal/model"
	"github.com/rifanid/agentmap/internal/query"
	"github.com/rifanid/agentmap/internal/rank"
)

const version = "0.1.0"

// knownFlags is the full set of recognised flags; anything else is a usage error.
var knownFlags = map[string]bool{
	"--json": true, "--print": true,
	"--help": true, "-h": true, "--version": true, "-v": true,
	"--install-hooks": true, "--hook-status": true, "--install-skill": true,
	"--platform": true, "--project": true, "--global": true,
	"--dry-run": true, "--setup-mcp": true, "--mcp": true,
	"--any": true, "--find": true, "--relates": true, "--map": true,
	"--focus": true, "--tokens": true,
	"--symbols": true, "--feature": true, "--features": true, "--hubs": true,
	"nudge": true, // subcommand, not a flag
}

// valueFlags consume the next token as their value (so that value is never
// mistakenly checked as an unknown flag).
var valueFlags = map[string]bool{
	"--any": true, "--find": true, "--relates": true, "--feature": true,
	"--focus": true, "--tokens": true, "--symbols": true, "--platform": true,
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	// index of tokens that are values, not flags
	valueIdx := map[int]bool{}
	for i := 0; i < len(args)-1; i++ {
		if valueFlags[args[i]] {
			valueIdx[i+1] = true
		}
	}

	has := func(f string) bool {
		for _, a := range args {
			if a == f {
				return true
			}
		}
		return false
	}
	argVal := func(f string) string {
		for i, a := range args {
			if a == f && i+1 < len(args) {
				v := args[i+1]
				if strings.HasPrefix(v, "--") {
					return ""
				}
				return v
			}
		}
		return ""
	}

	wantJSON := has("--json")
	emit := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Println(string(b))
	}

	if has("--help") || has("-h") {
		fmt.Println(usage)
		return 0
	}
	if has("--version") || has("-v") {
		fmt.Println(version)
		return 0
	}

	// nudge subcommand: read stdin PreToolUse JSON, write additionalContext.
	if len(args) > 0 && args[0] == "nudge" {
		return runNudge(args[1:])
	}

	// Check for unknown flags before any build.
	for i, a := range args {
		if strings.HasPrefix(a, "-") && !knownFlags[a] && !valueIdx[i] {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\ntry `agentmap --help` for the list of commands.\n", a)
			return 2
		}
	}

	// Maintenance commands (no map needed).
	switch {
	case has("--install-hooks"):
		return runInstallHooks(has("--dry-run"))
	case has("--hook-status"):
		return runHookStatus()
	case has("--install-skill"):
		return runInstallSkill(argVal("--platform"), has("--global"), has("--dry-run"))
	case has("--setup-mcp"):
		return runSetupMcp(has("--dry-run"))
	case has("--mcp"):
		return runMCP()
	}

	wd, _ := os.Getwd()

	// Query commands.
	switch {
	case has("--any"):
		raw := argVal("--any")
		if raw == "" {
			fmt.Fprintln(os.Stderr, `--any needs a query, e.g. --any Order`)
			return 2
		}
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		res, ok := query.Any(m, wd, raw)
		if wantJSON {
			type jsonAny struct {
				Command string `json:"command"`
				query.AnyResult
			}
			emit(jsonAny{"any", res})
		} else {
			printAny(res)
		}
		if !ok {
			return 1
		}
		return 0

	case has("--find"):
		q := argVal("--find")
		if q == "" {
			fmt.Fprintln(os.Stderr, `--find needs a symbol query, e.g. --find Order`)
			return 2
		}
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		res := query.Find(m, q)
		if wantJSON {
			type jsonFind struct {
				Command string `json:"command"`
				query.FindResult
			}
			emit(jsonFind{"find", res})
		} else {
			fmt.Printf("find %q: %d match\n", q, len(res.Matches))
			for _, mm := range res.Matches {
				fmt.Printf("  %s → %s (%s)\n", mm.Package, mm.Name, mm.Kind)
			}
		}
		if len(res.Matches) == 0 {
			return 1
		}
		return 0

	case has("--relates"):
		q := argVal("--relates")
		if q == "" {
			fmt.Fprintln(os.Stderr, `--relates needs a package path/name, e.g. --relates orders/usecase`)
			return 2
		}
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		res := query.Relates(m, q)
		if res.Error != "" {
			if wantJSON {
				type jsonErr struct {
					Command    string   `json:"command"`
					Error      string   `json:"error"`
					Query      string   `json:"query"`
					Candidates []string `json:"candidates"`
				}
				emit(jsonErr{"relates", res.Error, q, res.Candidates})
			} else {
				if len(res.Candidates) > 1 {
					fmt.Printf("relates: %q matched %d packages — narrow it:\n", q, len(res.Candidates))
					for _, c := range res.Candidates {
						fmt.Printf("  %s\n", c)
					}
				} else {
					fmt.Printf("relates: no package matching %q\n", q)
				}
			}
			return 1
		}
		if wantJSON {
			type jsonRel struct {
				Command string `json:"command"`
				query.RelatesResult
			}
			emit(jsonRel{"relates", res})
		} else {
			printRelates(res)
		}
		return 0

	case has("--map"):
		focusArg := argVal("--focus")
		if has("--focus") && focusArg == "" {
			fmt.Fprintln(os.Stderr, `--focus needs a package path/name, e.g. --map --focus orders/usecase`)
			return 2
		}
		tokArg := argVal("--tokens")
		budget, _ := strconv.Atoi(tokArg)
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		res, warn := query.MapDigest(m, focusArg, budget)
		if warn != "" {
			fmt.Fprintf(os.Stderr, "# warning: %s\n", warn)
		}
		if wantJSON {
			type jsonMap struct {
				Command string `json:"command"`
				query.MapResult
			}
			emit(jsonMap{"map", res})
		} else {
			fmt.Printf("# agentmap (%d packages, sha %s) — focus: %s, budget ~%d tok\n",
				m.PackageCount, m.GeneratedSha, res.Focus, res.Budget)
			for _, pd := range res.Files {
				fmt.Printf("\n%s:\n", pd.Package)
				for _, s := range pd.Symbols {
					fmt.Printf("  %s (%s)\n", s.Name, s.Kind)
				}
			}
			fmt.Printf("\n# ~%d tokens (%d packages shown)\n", res.Tokens, len(res.Files))
		}
		return 0

	case has("--symbols"):
		tokStr := argVal("--symbols")
		n := 30
		if v, err := strconv.Atoi(tokStr); err == nil && v > 0 {
			n = v
		}
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		syms := m.RankedSymbols
		if len(syms) > n {
			syms = syms[:n]
		}
		if wantJSON {
			type jsonSyms struct {
				Command string               `json:"command"`
				Symbols []model.RankedSymbol `json:"symbols"`
			}
			emit(jsonSyms{"symbols", syms})
		} else {
			fmt.Printf("top %d ranked symbols (Aider-style):\n", n)
			for _, s := range syms {
				fmt.Printf("  %g  %s → %s (%s)\n", s.Rank, s.File, s.Name, s.Kind)
			}
		}
		return 0

	case has("--feature"):
		q := argVal("--feature")
		if q == "" {
			fmt.Fprintln(os.Stderr, `--feature needs a name, e.g. --feature orders (run --features to list)`)
			return 2
		}
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		ql := strings.ToLower(q)
		name := ""
		for k := range m.Features {
			if strings.ToLower(k) == ql {
				name = k
				break
			}
		}
		if name == "" {
			for k := range m.Features {
				if strings.Contains(strings.ToLower(k), ql) {
					name = k
					break
				}
			}
		}
		if name == "" {
			if wantJSON {
				emit(map[string]any{"command": "feature", "error": "no match", "query": q})
			} else {
				fmt.Printf("feature: no match for %q — run --features to list them.\n", q)
			}
			return 1
		}
		fl := m.Features[name]
		set := map[string]bool{}
		for _, p := range fl {
			set[p] = true
		}
		var exts []string
		for _, p := range fl {
			for _, dep := range m.Packages[p].Dependents {
				if !set[dep] {
					exts = append(exts, dep)
				}
			}
		}
		exts = dedup(exts)
		if wantJSON {
			emit(map[string]any{"command": "feature", "name": name, "files": fl, "externalDependents": exts})
		} else {
			fmt.Printf("feature %q: %d packages\n", name, len(fl))
			for _, p := range fl {
				fmt.Printf("  %s\n", p)
			}
			extsStr := "—"
			if len(exts) > 0 {
				extsStr = strings.Join(exts, ", ")
			}
			fmt.Printf("external dependents (%d): %s\n", len(exts), extsStr)
		}
		return 0

	case has("--features"):
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		var list []featEntry
		for k, v := range m.Features {
			list = append(list, featEntry{k, len(v)})
		}
		sortByCountDesc(list)
		if wantJSON {
			out := map[string]int{}
			for _, e := range list {
				out[e.name] = e.count
			}
			emit(map[string]any{"command": "features", "features": out})
		} else {
			fmt.Printf("features (%d):\n", len(list))
			for _, e := range list {
				fmt.Printf("  %s (%d packages)\n", e.name, e.count)
			}
		}
		return 0

	case has("--hubs"):
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		if wantJSON {
			emit(map[string]any{"command": "hubs", "packageCount": m.PackageCount, "sha": m.GeneratedSha, "hubs": m.Hubs})
		} else {
			fmt.Printf("agentmap: %d packages (sha %s)\n", m.PackageCount, m.GeneratedSha)
			fmt.Println("hubs (PageRank importance):")
			for _, h := range m.Hubs {
				fmt.Printf("  %s\n", h)
			}
		}
		return 0

	case has("--print"):
		m, err := build.EnsureFresh(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		b, _ := json.Marshal(m)
		fmt.Println(string(b))
		return 0

	default:
		// Bare invocation: build + one-line summary.
		m, err := build.BuildAndSave(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentmap: %v\n", err)
			return 1
		}
		top := "—"
		if len(m.Hubs) > 0 {
			top = m.Hubs[0]
		}
		if wantJSON {
			feats := map[string]int{}
			for k, v := range m.Features {
				feats[k] = len(v)
			}
			_ = rank.RankedSymbolsLimit // suppress unused import
			emit(map[string]any{"command": "build", "packageCount": m.PackageCount, "features": feats, "topHub": top})
		} else {
			fmt.Printf("agentmap: %d packages | %d features | top hub: %s\n",
				m.PackageCount, len(m.Features), top)
		}
		return 0
	}
}

// ---- prose printers ----

func printAny(r query.AnyResult) {
	switch r.Kind {
	case query.AnyKindPackage:
		fmt.Printf("[structure:package] %s  (pr %g)\n", r.Package, r.PageRank)
		fmt.Printf("exports (%d): %s\n", len(r.Exports), fmtExports(r.Exports))
		fmt.Printf("imports (%d): %s\n", len(r.Imports), joinOr(r.Imports, "—"))
		fmt.Printf("dependents (%d): %s\n", len(r.Dependents), joinOr(r.Dependents, "—"))
		if len(r.Symbols) > 0 {
			fmt.Printf("[structure] %d symbol match for %q:\n", len(r.Symbols), r.Query)
			for _, s := range r.Symbols {
				fmt.Printf("  %s → %s (%s)\n", s.Package, s.Name, s.Kind)
			}
		}
		if len(r.Features) > 0 {
			parts := make([]string, len(r.Features))
			for i, f := range r.Features {
				parts[i] = fmt.Sprintf("%s (%d)", f.Name, f.Count)
			}
			fmt.Printf("features: %s\n", strings.Join(parts, ", "))
		}
	case query.AnyKindStructure:
		fmt.Printf("[structure] %d symbol, %d feature match for %q\n", len(r.Symbols), len(r.Features), r.Query)
		for _, s := range r.Symbols {
			fmt.Printf("  %s → %s (%s)\n", s.Package, s.Name, s.Kind)
		}
		if len(r.Features) > 0 {
			parts := make([]string, len(r.Features))
			for i, f := range r.Features {
				parts[i] = fmt.Sprintf("%s (%d)", f.Name, f.Count)
			}
			fmt.Printf("features: %s\n", strings.Join(parts, ", "))
		}
	case query.AnyKindCandidates:
		fmt.Printf("[structure] %q matched %d packages — narrow it:\n", r.Query, len(r.Candidates))
		for _, c := range r.Candidates {
			fmt.Printf("  %s\n", c)
		}
	case query.AnyKindContent:
		extra := ""
		if r.Total > len(r.Lines) {
			extra = fmt.Sprintf(" (showing %d)", len(r.Lines))
		}
		fmt.Printf("[content] %d line%s%s:\n", r.Total, plural(r.Total), extra)
		fmt.Println(strings.Join(r.Lines, "\n"))
	case query.AnyKindEmpty:
		fmt.Printf("[content] 0 match for %q (git grep, tracked + untracked)\n", r.Query)
	}
}

func printRelates(r query.RelatesResult) {
	fmt.Printf("relates: %s  (pr %g)\n", r.Package, r.PageRank)
	fmt.Printf("exports (%d): %s\n", len(r.Exports), fmtExports(r.Exports))
	fmt.Printf("imports (%d): %s\n", len(r.Imports), joinOr(r.Imports, "—"))
	fmt.Printf("dependents (%d): %s\n", len(r.Dependents), joinOr(r.Dependents, "—"))
	fmt.Println("related (random-walk relevance):")
	for _, rel := range r.Related {
		fmt.Printf("  %s (%.4f)\n", rel.Package, rel.Score)
	}
}

// ---- helpers ----

func fmtExports(exports []model.Symbol) string {
	if len(exports) == 0 {
		return "—"
	}
	parts := make([]string, len(exports))
	for i, e := range exports {
		parts[i] = fmt.Sprintf("%s(%s)", e.Name, e.Kind)
	}
	return strings.Join(parts, ", ")
}

func joinOr(ss []string, def string) string {
	if len(ss) == 0 {
		return def
	}
	return strings.Join(ss, ", ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func dedup(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

type featEntry struct{ name string; count int }

func sortByCountDesc(list []featEntry) {
	n := len(list)
	for i := 1; i < n; i++ {
		for j := i; j > 0; j-- {
			if list[j].count > list[j-1].count || (list[j].count == list[j-1].count && list[j].name < list[j-1].name) {
				list[j], list[j-1] = list[j-1], list[j]
			} else {
				break
			}
		}
	}
}

func runInstallHooks(dry bool) int {
	if err := hooks.InstallHooks(dry); err != nil {
		fmt.Fprintf(os.Stderr, "agentmap --install-hooks failed: %v\n", err)
		return 1
	}
	return 0
}

func runHookStatus() int {
	hooks.HookStatus()
	return 0
}

func runInstallSkill(platform string, global, dry bool) int {
	if err := install.InstallSkill(platform, global, dry); err != nil {
		fmt.Fprintf(os.Stderr, "agentmap --install-skill failed: %v\n", err)
		return 1
	}
	return 0
}

func runSetupMcp(dry bool) int {
	if err := install.SetupMCP(dry); err != nil {
		fmt.Fprintf(os.Stderr, "agentmap --setup-mcp failed: %v\n", err)
		return 1
	}
	return 0
}

func runMCP() int {
	if err := mcp.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "agentmap --mcp failed: %v\n", err)
		return 1
	}
	return 0
}

func runNudge(args []string) int {
	return hooks.RunNudge(args)
}

const usage = `agentmap — the queryable, ranked repo map your coding agent is forced to use.

Usage: agentmap [command] [--json]

Query commands:
  --any <q>            route a query: package → symbol → feature → live git-grep
  --find <sym>         find exported symbols by (sub)name
  --relates <path>     a package's exports/imports/dependents + related packages
  --map [--focus <p>] [--tokens <n>]
                       token-budgeted ranked digest (--focus personalizes)
  --symbols [n]        top-n Aider-style ranked symbols (default 30)
  --feature <name>     packages composing a DDD domain + external dependents
  --features           list all features (DDD domains) by size
  --hubs               top packages by PageRank importance
  --print              dump the full cached map as JSON
  (no flags)           build the map + print a one-line summary

Global modifier:
  --json               emit exactly one JSON object (no prose) for the command

Maintenance:
  --install-hooks [--dry-run]   install git post-commit + wire the PreToolUse nudge
  --hook-status                 report whether agentmap wiring is installed
  --install-skill [--platform ...] [--project|--global] [--dry-run]
  --setup-mcp [--dry-run]       configure MCP server for OpenCode & Antigravity
  --mcp                         start a stdio MCP server
  nudge [--gemini]              PreToolUse nudge (reads stdin, writes stdout)
  --help, -h                    show this help
  --version, -v                 print the version

Exit codes: 0 ok · 1 query had zero results · 2 usage error.`

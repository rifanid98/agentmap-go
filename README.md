# agentmap

A queryable, PageRank-ranked code-relationship map for Go monorepos — native Go single binary, no Node dependency.

Ported from [agentmap (raymondchins)](https://github.com/raymondchins/agentmap) with full feature parity and first-class support for **Go workspace monorepos** (`go.work` + `replace` directives). Cuts agent comprehension tokens ~98% by forcing queries against a pre-built graph instead of ad-hoc `grep`.

---

## Why

A coding agent exploring a Go monorepo (e.g. `go-core` + 6 services wired by `replace` directives) typically spends most of its token budget on filesystem discovery. `agentmap` pre-computes the import graph once, then answers blast-radius questions in milliseconds:

```
# who imports entity across all modules?
agentmap --relates go-core/orders/domain/entity

# top hub packages by PageRank importance
agentmap --hubs

# find any exported symbol named "Cancel"
agentmap --find Cancel

# token-budgeted ranked digest for an LLM prompt
agentmap --map --tokens 4096
```

---

## Install

```bash
go install github.com/rifanid/agentmap@latest
```

Or build from source:

```bash
git clone https://github.com/rifanid/agentmap
cd agentmap
go build -o agentmap .
```

Single self-contained binary. Only dependency: `golang.org/x/mod` (pure Go, no cgo).

---

## Quick start

```bash
# from your repo or go.work root:
agentmap                     # build map + one-line summary
agentmap --hubs              # top packages by PageRank
agentmap --relates <pkg>     # blast radius of a package
agentmap --find <symbol>     # locate an exported symbol
agentmap --features          # DDD domain groups
agentmap --map               # token-budgeted digest for an LLM

# wire into git + Claude Code agent hooks (recommended):
agentmap --install-hooks
```

---

## Commands

### Query

| Command | Description |
|---|---|
| `--any <q>` | Smart router: package → symbol → feature → git-grep fallback |
| `--find <sym>` | Find exported symbols by (sub)name, case-insensitive |
| `--relates <pkg>` | A package's exports, imports, dependents + random-walk related packages |
| `--map` | Token-budgeted ranked digest; `--focus <pkg>` personalizes PageRank; `--tokens <n>` sets budget (default 8192) |
| `--symbols [n]` | Top-n Aider-style ranked symbols by identifier graph (default 30) |
| `--feature <name>` | Packages composing a DDD domain + external dependents |
| `--features` | All DDD domain groups by package count |
| `--hubs` | Top packages ranked by PageRank importance |
| `--print` | Dump the full cached map as JSON |
| *(no flag)* | Build the map and print a one-line summary |

Append `--json` to any query command to emit a single JSON object instead of prose — useful for piping into scripts or LLM prompts.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Query matched zero results |
| `2` | Usage error (unknown flag, missing argument) |

### Maintenance

| Command | Description |
|---|---|
| `--install-hooks [--dry-run]` | Install git post-commit hook + wire PreToolUse nudge into `.claude/settings.json` |
| `--hook-status` | Report whether all wiring is installed |
| `--install-skill [--platform <p>] [--project\|--global] [--dry-run]` | Install the agent skill file for a coding platform |
| `--setup-mcp [--dry-run]` | Configure the MCP server for OpenCode and Antigravity |
| `--mcp` | Start a stdio MCP server (JSON-RPC 2.0, protocol `2024-11-05`) |
| `nudge [--gemini]` | PreToolUse nudge — reads hook JSON on stdin, writes `additionalContext` on stdout |
| `--version`, `-v` | Print version |
| `--help`, `-h` | Show usage |

---

## Monorepo support (headline feature)

`agentmap` resolves imports **across a Go workspace** — it parses `go.work`, `go.mod`, and `replace` directives to map cross-module import edges without needing a compilable build.

```
myproject/
  go.work            # use ./go-core; use ./order-service
  go-core/
    go.mod           # module example.com/go-core
    orders/
      domain/entity/
      usecase/
  order-service/
    go.mod           # replace example.com/go-core => ../go-core
    internal/handler/
```

```bash
cd myproject
agentmap --relates entity
# → dependents: go-core/orders/usecase, order-service/internal/handler
```

Dead `replace` targets (stale paths from `directive.sh`) emit a warning to stderr and are treated as external — the map still builds.

---

## How it works

### Graph node = package (not file)

Go `import` binds a package directory. `agentmap` models each package dir as a node; edges are import relationships. Exported symbols still record their defining file for `--find`/`--symbols`/`--map` output.

### PageRank

Power iteration (damping 0.85, tolerance 1e-6, max 100 iterations). Deterministic: all map ranges are sorted before iteration. Personalization vector drives `--relates` and `--map --focus`.

### Identifier graph (Aider-style)

`SelectorExpr` references collected per file, weighted by mention count and identifier heuristics (boost for long/rare names; penalty for underscores and single-char identifiers). Personalized PageRank on the identifier graph produces `--symbols` and seeds `--map`.

### Parsing

`go/parser` + `go/ast` — no build required, no `go/packages`. Graceful degradation: a malformed `.go` file emits `# agentmap: skipped <file>` to stderr; the rest of the map builds normally.

### Cache

Map is stored in `.claude/agentmap/map.json` (atomic write via `.tmp` + `os.Rename`). Freshness check:
- **Git repo**: trust cache when `git rev-parse --short HEAD` matches `generatedSha` and `git status` shows zero dirty `.go` files.
- **Non-git**: fingerprint via `path:mtimeNano:size` SHA-1 walk.

---

## DDD feature detection

`--features` groups packages by **DDD domain** — the path segment before the first layer marker:

```
go-core/orders/domain/entity   → feature: orders
go-core/orders/usecase         → feature: orders
go-core/transaction/domain/vo  → feature: transaction
```

Layer markers: `domain`, `usecase`, `repository`, `delivery`, `handler`, `port`, `infrastructure`, `transport`, `adapter`, `service` (and their plurals).

Generic containers (`internal`, `pkg`, `src`, `app`, `cmd`) are skipped.

---

## Hooks

### Git post-commit hook

`--install-hooks` writes a POSIX `sh` post-commit hook that runs `agentmap` in the background after every commit, keeping the cache warm. Respects `AGENTMAP_HOOK_NO_LOCAL` (skip local binary, use PATH). Guarded against rebase/merge/cherry-pick/bisect.

### PreToolUse nudge

Intercepts `Grep` and `Bash` tool calls before they execute and injects `additionalContext` directing the agent to query `agentmap` instead of scanning files. Fires on Go import-hunt patterns; silent on log-file greps and data-file searches. `--gemini` mode emits `{}` on no-fire (Gemini hook protocol).

---

## MCP server

`agentmap --mcp` starts a stdio JSON-RPC 2.0 MCP server with 8 tools:

| Tool | Description |
|---|---|
| `any` | Smart query router |
| `find` | Symbol search |
| `relates` | Package blast radius |
| `map` | Token-budgeted digest |
| `hubs` | Top PageRank packages |
| `features` | DDD domain list |
| `feature` | Single DDD domain detail |
| `symbols` | Ranked identifier list |

The server holds the cache warm in-process — no subprocess per call.

`--setup-mcp` writes the MCP entry to OpenCode (`~/.config/opencode/opencode.json`) and Antigravity (`~/.gemini/antigravity/mcp_config.json`, `~/.gemini/config/mcp_config.json`).

---

## Skill installer

`--install-skill` copies a compressed skill description into the project or global skills directory for the specified coding platform:

```bash
agentmap --install-skill                      # all platforms, project scope
agentmap --install-skill --platform claude    # Claude Code only
agentmap --install-skill --global            # install globally (~/…)
agentmap --install-skill --dry-run           # preview only
```

Supported platforms: `claude`, `cursor`, `codex`, `gemini`, `antigravity`, `copilot`, `all`.

---

## Parity with the original JS agentmap

| JS | Go | Reason |
|---|---|---|
| `files{}` keyed by file path | `packages{}` keyed by package dir | Go import edges bind packages, not files |
| `fileCount` | `packageCount` | same |
| TypeScript kinds (`ClassDeclaration`, …) | Go kinds (`struct`, `interface`, `function`, `method`, `type`, `alias`, `const`, `var`) | Go AST |
| `reExports[]` | absent | Go has no `export … from` |
| features = Next.js `app/` routes | features = DDD domain folder | explicit user decision |
| non-git fingerprint SHA format | differs (mtime precision) | mechanism identical |
| nudge wired as `.mjs` script | nudge is `agentmap nudge` subcommand | no Node dependency |

Everything else — `schema`, `generatedSha`, `dirty`, `hubs`, `rankedSymbols[{file,name,kind,rank}]`, PageRank algorithm, freshness rules, exit codes, `--json` single-object contract, MCP protocol — is shape-compatible.

---

## Development

```bash
go build ./...
go vet ./...
go test ./...           # 30 tests: 14 unit + 16 black-box integration
```

Integration tests in `test/` build the binary via `TestMain`, create a `go.work` monorepo fixture in a temp dir, and assert on stdout/stderr/exit code/JSON shape — no network, no external tooling.

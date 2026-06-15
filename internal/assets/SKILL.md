---
name: agentmap
description: Go workspace code-relationship map — PageRank hubs, blast-radius, ranked symbols, token-budgeted digest.
---

# agentmap

A queryable, ranked map of a Go monorepo (go.work / go.mod + replace directives).
Use it **before** grepping to orient yourself at a fraction of the token cost.

## Commands

```
agentmap --any <query>         # package → symbol → feature → git-grep
agentmap --find <symbol>       # find exported symbols by (sub)name
agentmap --relates <pkg>       # blast radius: imports / dependents / related
agentmap --map [--focus <pkg>] # token-budgeted digest; personalised with --focus
agentmap --hubs                # top packages by PageRank importance
agentmap --features            # list DDD domain features
agentmap --symbols [n]         # top-n Aider-style ranked symbols
agentmap --json <cmd>          # any command emits one JSON object
```

## Workflow

1. **Orient** — `agentmap --hubs` to see the most-imported packages.
2. **Locate** — `agentmap --any <query>` routes to package, symbol, or feature.
3. **Blast radius** — `agentmap --relates <pkg>` before editing anything.
4. **Digest** — `agentmap --map --focus <pkg>` for a token-efficient codebase overview.
5. **Fall back** to grep only for exact line numbers.

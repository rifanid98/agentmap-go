<!-- agentmap guidance block — do not edit manually -->
## agentmap — Go repo map

Use **agentmap** for structural questions instead of raw grep:

- `agentmap --any <query>` — routes to package, symbol, DDD feature, or content
- `agentmap --relates <pkg>` — blast radius before editing (imports + dependents)
- `agentmap --map [--focus <pkg>]` — token-budgeted digest, personalised
- `agentmap --hubs` — most-imported (hub) packages by PageRank
- `agentmap --find <symbol>` — locate an exported symbol across the workspace

Fall back to grep only for exact line content the map doesn't index.
<!-- end agentmap guidance block -->

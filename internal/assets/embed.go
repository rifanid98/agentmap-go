// Package assets embeds the static files shipped inside the agentmap binary so
// --install-hooks, --install-skill, and --setup-mcp can write them without
// requiring the source tree to be present at runtime.
package assets

import _ "embed"

//go:embed post-commit
var PostCommit []byte

//go:embed SKILL.md
var SkillMD []byte

//go:embed cursor-rule.mdc
var CursorRule []byte

//go:embed guidance.md
var GuidanceMD []byte

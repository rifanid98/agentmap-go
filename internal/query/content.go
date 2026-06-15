package query

import (
	"os/exec"
	"strings"
)

// sensitiveExcludes mirrors agentmap's SENSITIVE_EXCLUDES pathspecs so
// .env / *.key / *secret* etc. are never surfaced in content search.
var sensitiveExcludes = []string{
	":!.env", ":!.env.*", ":!**/.env", ":!**/.env.*",
	":!*.env", ":!**/*.env",
	":!*.pem", ":!*.key", ":!*.p12", ":!*.pfx", ":!*.crt", ":!id_rsa*",
	":(exclude,icase)*secret*", ":(exclude,icase)*credential*", ":(exclude,icase)*.password*",
}

// ContentSearch runs `git grep -F --untracked -n -i -I -e <q>` in the given
// working directory. Uses argv (no shell) for injection safety. Returns trimmed
// output or "" on any error / no match.
func ContentSearch(cwd, q string) string {
	args := []string{
		"grep", "-F", "--untracked", "-n", "-i", "-I",
		"-e", q, "--", ".", ":!.claude/agentmap/",
	}
	args = append(args, sensitiveExcludes...)
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

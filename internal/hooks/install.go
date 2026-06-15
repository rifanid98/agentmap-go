package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rifanid/agentmap-go/internal/assets"
	"github.com/rifanid/agentmap-go/internal/install"
)

const (
	ignoreLine   = ".claude/agentmap/"
	nudgeDestRel = ".claude/hooks/agentmap-nudge-stub" // unused file; nudge is in the binary
	settingsPath = ".claude/settings.json"
	nudgeCmd     = `agentmap nudge`
	hooksMarker  = "agentmap — git post-commit hook"
)

// InstallHooks wires the git post-commit hook + .claude/settings.json nudge
// into the current repo. Merge-safe + idempotent. dry=true previews only.
func InstallHooks(dry bool) error {
	gitDir := gitDir()
	if gitDir == "" {
		return fmt.Errorf("not a git repository (cwd has no .git) — run inside the repo you want to wire up")
	}
	hooksDir := filepath.Join(gitDir, "hooks")
	postCommitDest := filepath.Join(hooksDir, "post-commit")

	// settings.json current state
	settings, _ := install.ReadJSONC(settingsPath)
	if settings == nil {
		settings = map[string]any{}
	}
	alreadyGrep := hasNudge(settings, "Grep")
	alreadyBash := hasNudge(settings, "Bash")

	// .gitignore current state
	ignoredAlready := hasIgnoreLine()

	targets := []string{postCommitDest}
	if !ignoredAlready {
		targets = append(targets, ".gitignore")
	}
	if !alreadyGrep || !alreadyBash {
		targets = append(targets, settingsPath)
	}

	if dry {
		fmt.Println("--dry-run: would create/overwrite the following files (no changes written):")
		for _, t := range targets {
			fmt.Printf("  %s\n", t)
		}
		return nil
	}

	fmt.Printf("agentmap --install-hooks: writing %d file(s): %s\n", len(targets), strings.Join(targets, ", "))

	// 1) post-commit hook
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	if err := install.AtomicWrite(postCommitDest, assets.PostCommit, 0o755); err != nil {
		return err
	}

	// 2) .gitignore
	if !ignoredAlready {
		if err := appendIgnoreLine(); err != nil {
			return err
		}
	}

	// 3) settings.json nudge wiring
	if !alreadyGrep || !alreadyBash {
		if err := wireNudge(settings, alreadyGrep, alreadyBash); err != nil {
			return err
		}
	}
	return nil
}

// HookStatus reports the installation state of all wiring components.
func HookStatus() {
	gitDir := gitDir()
	if gitDir == "" {
		fmt.Println("post-commit: not a git repository")
		return
	}
	// post-commit
	pcPath := filepath.Join(gitDir, "hooks", "post-commit")
	pcData, err := os.ReadFile(pcPath)
	if err != nil {
		fmt.Println("post-commit: not installed")
	} else if strings.Contains(string(pcData), hooksMarker) {
		fmt.Println("post-commit: installed")
	} else {
		fmt.Println("post-commit: not installed (hook exists but agentmap marker not found)")
	}

	// settings.json
	settings, err := install.ReadJSONC(settingsPath)
	if err != nil || settings == nil {
		fmt.Println("PreToolUse(Grep): not wired (no settings.json)")
		fmt.Println("PreToolUse(Bash): not wired (no settings.json)")
	} else {
		for _, tool := range []string{"Grep", "Bash"} {
			if hasNudge(settings, tool) {
				fmt.Printf("PreToolUse(%s): wired\n", tool)
			} else {
				fmt.Printf("PreToolUse(%s): not wired\n", tool)
			}
		}
	}

	// .gitignore
	if hasIgnoreLine() {
		fmt.Printf(".gitignore (%s): ok\n", ignoreLine)
	} else {
		fmt.Printf(".gitignore (%s): missing entry\n", ignoreLine)
	}
}

// ---- helpers ----

func gitDir() string {
	out, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func hasNudge(settings map[string]any, tool string) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	ptu, _ := hooks["PreToolUse"].([]any)
	for _, entry := range ptu {
		e, _ := entry.(map[string]any)
		if e == nil || e["matcher"] != tool {
			continue
		}
		hooksArr, _ := e["hooks"].([]any)
		for _, h := range hooksArr {
			hm, _ := h.(map[string]any)
			if cmd, _ := hm["command"].(string); strings.Contains(cmd, "agentmap nudge") {
				return true
			}
		}
	}
	return false
}

func hasIgnoreLine() bool {
	data, err := os.ReadFile(".gitignore")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == ignoreLine {
			return true
		}
	}
	return false
}

func appendIgnoreLine() error {
	f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n%s\n", ignoreLine)
	return err
}

func wireNudge(settings map[string]any, alreadyGrep, alreadyBash bool) error {
	if settings["hooks"] == nil {
		settings["hooks"] = map[string]any{}
	}
	hooks := settings["hooks"].(map[string]any)
	if hooks["PreToolUse"] == nil {
		hooks["PreToolUse"] = []any{}
	}
	ptu, _ := hooks["PreToolUse"].([]any)
	hookEntry := map[string]any{"type": "command", "command": nudgeCmd}
	if !alreadyGrep {
		ptu = append(ptu, map[string]any{
			"matcher": "Grep",
			"hooks":   []any{hookEntry},
		})
	}
	if !alreadyBash {
		ptu = append(ptu, map[string]any{
			"matcher": "Bash",
			"hooks":   []any{hookEntry},
		})
	}
	hooks["PreToolUse"] = ptu
	settings["hooks"] = hooks

	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return install.AtomicWrite(settingsPath, b, 0o644)
}

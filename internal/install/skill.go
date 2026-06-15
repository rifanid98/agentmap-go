package install

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rifanid/agentmap-go/internal/assets"
)

// Platform describes where a skill file is installed.
type Platform struct {
	Name       string
	ProjectDir string // relative from repo root, or "" if project-only n/a
	GlobalDir  func() string
	SkillFile  []byte
	DocsFile   string // "" = skip docs
}

var platforms = []Platform{
	{
		Name: "claude", ProjectDir: ".claude/skills/agentmap",
		GlobalDir: func() string { return globalDir(".claude/skills/agentmap") },
		SkillFile: assets.SkillMD, DocsFile: "AGENTS.md",
	},
	{
		Name: "cursor", ProjectDir: ".cursor/rules",
		GlobalDir: nil, SkillFile: assets.CursorRule,
	},
	{
		Name: "codex", ProjectDir: ".codex/skills/agentmap",
		GlobalDir: func() string { return globalDir(".codex/skills/agentmap") },
		SkillFile: assets.SkillMD, DocsFile: ".codex/AGENTS.md",
	},
	{
		Name: "gemini", ProjectDir: ".gemini/skills/agentmap",
		GlobalDir: func() string { return globalDir(".gemini/skills/agentmap") },
		SkillFile: assets.SkillMD, DocsFile: "GEMINI.md",
	},
	{
		Name: "antigravity", ProjectDir: ".agents/skills/agentmap",
		GlobalDir: func() string { return globalDir(".gemini/config/skills/agentmap") },
		SkillFile: assets.SkillMD,
	},
	{
		Name: "copilot", ProjectDir: ".copilot/skills/agentmap",
		GlobalDir: func() string { return globalDir(".copilot/skills/agentmap") },
		SkillFile: assets.SkillMD,
	},
}

// InstallSkill installs agentmap skill files for the given platform(s). If
// platform=="all" every platform is installed. globalScope=true installs to
// the user home directory; otherwise to the current repo root.
func InstallSkill(platform string, globalScope, dry bool) error {
	var toInstall []Platform
	if platform == "all" || platform == "" {
		toInstall = platforms
	} else {
		for _, p := range platforms {
			if p.Name == platform {
				toInstall = []Platform{p}
				break
			}
		}
		if len(toInstall) == 0 {
			return fmt.Errorf("unknown platform %q — supported: claude cursor codex gemini antigravity copilot all", platform)
		}
	}

	for _, p := range toInstall {
		if err := installOne(p, globalScope, dry); err != nil {
			fmt.Fprintf(os.Stderr, "# agentmap: install-skill %s: %v\n", p.Name, err)
		}
	}
	return nil
}

func installOne(p Platform, globalScope, dry bool) error {
	var dir string
	if globalScope {
		if p.GlobalDir == nil {
			fmt.Printf("  %s: no global scope supported, skipping\n", p.Name)
			return nil
		}
		dir = p.GlobalDir()
	} else {
		if p.ProjectDir == "" {
			return nil
		}
		dir = p.ProjectDir
	}

	// skill file name: SKILL.md for most, agentmap.mdc for cursor.
	skillName := "SKILL.md"
	if p.Name == "cursor" {
		skillName = "agentmap.mdc"
	}
	destPath := filepath.Join(dir, skillName)

	if dry {
		fmt.Printf("  %s: would write %s\n", p.Name, destPath)
		if p.DocsFile != "" {
			fmt.Printf("  %s: would merge docs block into %s\n", p.Name, p.DocsFile)
		}
		return nil
	}

	if err := AtomicWrite(destPath, p.SkillFile, 0o644); err != nil {
		return err
	}
	fmt.Printf("  %s: installed %s\n", p.Name, destPath)

	if p.DocsFile != "" && !globalScope {
		if err := MergeDocsBlock(p.DocsFile, assets.GuidanceMD); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: docs merge failed: %v\n", p.Name, err)
		} else {
			fmt.Printf("  %s: merged docs into %s\n", p.Name, p.DocsFile)
		}
	}
	return nil
}

func globalDir(rel string) string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		rel = strings.ReplaceAll(rel, "/", string(filepath.Separator))
	}
	return filepath.Join(home, rel)
}

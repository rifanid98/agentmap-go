package install

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SetupMCP registers agentmap as an MCP server in OpenCode and Antigravity IDE
// global configs. Merge-safe + idempotent. dry=true previews only.
func SetupMCP(dry bool) error {
	selfPath, _ := exec.LookPath(os.Args[0])
	if selfPath == "" {
		selfPath = os.Args[0]
	}
	command := selfPath
	args := []string{"--mcp"}

	home, _ := os.UserHomeDir()
	type target struct {
		label string
		path  string
		graft func(map[string]any)
	}
	targets := []target{
		{
			label: "OpenCode",
			path:  filepath.Join(home, ".config", "opencode", "opencode.json"),
			graft: func(cfg map[string]any) {
				if cfg["mcp"] == nil {
					cfg["mcp"] = map[string]any{}
				}
				mcp := cfg["mcp"].(map[string]any)
				mcp["agentmap"] = map[string]any{
					"type": "stdio", "command": command, "args": args, "enabled": true,
				}
			},
		},
		{
			label: "Antigravity (antigravity/)",
			path:  filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"),
			graft: func(cfg map[string]any) { graftAntigravity(cfg, command, args) },
		},
		{
			label: "Antigravity (config/)",
			path:  filepath.Join(home, ".gemini", "config", "mcp_config.json"),
			graft: func(cfg map[string]any) { graftAntigravity(cfg, command, args) },
		},
	}

	if dry {
		fmt.Println("--dry-run: would configure MCP server (no changes written):")
		for _, t := range targets {
			fmt.Printf("  %s: would write to %s\n", t.label, t.path)
		}
		return nil
	}

	for _, t := range targets {
		cfg, err := ReadJSONC(t.path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "# agentmap: %s: %v\n", t.label, err)
			continue
		}
		if cfg == nil {
			cfg = map[string]any{}
		}
		t.graft(cfg)
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "# agentmap: %s: marshal error: %v\n", t.label, err)
			continue
		}
		b = append(b, '\n')
		if err := AtomicWrite(t.path, b, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "# agentmap: %s: write error: %v\n", t.label, err)
			continue
		}
		fmt.Printf("configured %s MCP server → %s\n", t.label, t.path)
	}
	return nil
}

func graftAntigravity(cfg map[string]any, command string, args []string) {
	if cfg["mcpServers"] == nil {
		cfg["mcpServers"] = map[string]any{}
	}
	servers := cfg["mcpServers"].(map[string]any)
	servers["agentmap"] = map[string]any{"command": command, "args": args}
}

// pathArgs converts a command + []string args to a flat string for display.
func pathArgs(command string, args []string) string {
	return command + " " + strings.Join(args, " ")
}

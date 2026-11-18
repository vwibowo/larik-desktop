// Package config loads layered JSON settings:
//
//	~/.config/larik/config.json      user defaults
//	./.larik/settings.json           project settings (commit this)
//	~/.config/larik/projects/<id>.json  personal project settings and approvals
//
// Later trusted files override scalars. Shared files may only tighten
// permissions; MCP servers are also read from .mcp.json (see mcp.go).
package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	DataDir string `json:"-"`
}

func DataDir() string { return dataDir() }

func dataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "larik")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "larik")
}

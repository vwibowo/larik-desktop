package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const ProtocolVersion = 1
const ShellOverrideCapability = "shell-override"

type Request struct {
	Op     string          `json:"op"`
	ID     string          `json:"id,omitempty"`
	Dir    string          `json:"dir,omitempty"`
	Shell  string          `json:"shell,omitempty"`
	Cols   int             `json:"cols,omitempty"`
	Rows   int             `json:"rows,omitempty"`
	Layout json.RawMessage `json:"layout,omitempty"`
}

type Response struct {
	Error        string          `json:"error,omitempty"`
	Session      Info            `json:"session,omitempty"`
	Sessions     []Info          `json:"sessions,omitempty"`
	Layout       json.RawMessage `json:"layout,omitempty"`
	PID          int             `json:"pid,omitempty"`
	Version      int             `json:"version,omitempty"`
	Capabilities []string        `json:"capabilities,omitempty"`
}

type Info struct {
	ID         string    `json:"id"`
	PID        int       `json:"pid"`
	Shell      string    `json:"shell"`
	Program    string    `json:"program"`
	Dir        string    `json:"dir"`
	Idle       bool      `json:"idle"`
	Output     uint64    `json:"output"`
	Bells      int64     `json:"bells"`
	LastOutput time.Time `json:"lastOutput"`
	LastInput  time.Time `json:"lastInput"`
	Exited     bool      `json:"exited"`
	ExitCode   int       `json:"exitCode"`
}

func Dir() string {
	if dir := os.Getenv("LARIK_DESKTOP_DIR"); dir != "" {
		return dir
	}
	base, _ := os.UserConfigDir()
	return filepath.Join(base, "larik-desktop")
}

func socketPath() string { return filepath.Join(Dir(), "server.sock") }

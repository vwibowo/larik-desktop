package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"larik/internal/session"
)

type Agent struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (a Agent) Key() string { return a.Name + "\x00" + a.Path }

// DetectAgent identifies a local harness from the CLI's process ancestry.
// It is for consent UX; same-user processes are not isolated from each other.
func DetectAgent() Agent {
	return detectAgent(os.Getppid())
}

func DetectAgentFromPeer(pid int) Agent {
	return detectAgent(pid)
}

func detectAgent(parent int) Agent {
	fallback := Agent{Name: "Local CLI", Path: "unknown"}
	for i := 0; i < 12 && parent > 1; i++ {
		out, err := exec.Command("/bin/ps", "-p", strconv.Itoa(parent), "-o", "ppid=", "-o", "comm=").Output()
		if err != nil {
			break
		}
		fields := strings.Fields(string(out))
		if len(fields) < 2 {
			break
		}
		path := strings.Join(fields[1:], " ")
		name := strings.ToLower(filepath.Base(path))
		if i == 0 && filepath.Base(path) != "larik-desktop" || i == 1 && fallback.Path == "unknown" {
			fallback.Path = path
		}
		for _, agent := range []string{"codex", "opencode", "claude"} {
			if name == agent || strings.HasPrefix(name, agent+"-") {
				return Agent{Name: agent, Path: path}
			}
		}
		next, err := strconv.Atoi(fields[0])
		if err != nil || next == parent {
			break
		}
		parent = next
	}
	return fallback
}

type Grants struct {
	mu   sync.Mutex
	path string
	list []Agent
}

func LoadGrants() (*Grants, error) {
	return LoadGrantsAt(filepath.Join(session.Dir(), "agent-access.json"))
}

func LoadGrantsAt(path string) (*Grants, error) {
	g := &Grants{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &g.list); err != nil {
		return nil, fmt.Errorf("agent grants: %w", err)
	}
	return g, nil
}

func (g *Grants) List() []Agent {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]Agent(nil), g.list...)
}

func (g *Grants) Has(a Agent) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, item := range g.list {
		if item.Key() == a.Key() {
			return true
		}
	}
	return false
}

func (g *Grants) Set(a Agent, allow bool) error {
	if a.Name == "" || a.Path == "" {
		return errors.New("agent identity is incomplete")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	next := make([]Agent, 0, len(g.list)+1)
	for _, item := range g.list {
		if item.Key() != a.Key() {
			next = append(next, item)
		}
	}
	if allow {
		next = append(next, a)
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(g.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(g.path), ".agent-access-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), g.path); err != nil {
		return err
	}
	g.list = next
	return nil
}

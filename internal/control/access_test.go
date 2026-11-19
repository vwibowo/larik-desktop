package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGrantPersistenceAndRevoke(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-access.json")
	g, err := LoadGrantsAt(path)
	if err != nil {
		t.Fatal(err)
	}
	a := Agent{Name: "codex", Path: "/usr/local/bin/codex"}
	if g.Has(a) {
		t.Fatal("new grant store allowed an agent")
	}
	if err := g.Set(a, true); err != nil {
		t.Fatal(err)
	}
	if err := g.Set(a, true); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadGrantsAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Has(a) || len(reloaded.List()) != 1 {
		t.Fatal("grant was not persisted idempotently")
	}
	if mode := fileMode(t, path); mode != 0o600 {
		t.Fatalf("grant file mode = %o", mode)
	}
	if err := reloaded.Set(a, false); err != nil {
		t.Fatal(err)
	}
	if reloaded.Has(a) {
		t.Fatal("revoked agent remains allowed")
	}
}

func TestGrantSaveFailureLeavesPriorValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-access.json")
	g, err := LoadGrantsAt(path)
	if err != nil {
		t.Fatal(err)
	}
	a := Agent{Name: "claude", Path: "/bin/claude"}
	if err := g.Set(a, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := g.Set(a, false); err == nil {
		t.Fatal("revoke should fail when destination is a directory")
	}
	if !g.Has(a) {
		t.Fatal("failed save changed the in-memory grant")
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	i, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return i.Mode().Perm()
}

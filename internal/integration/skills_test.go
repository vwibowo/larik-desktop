package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillInstallRemoveAndConflict(t *testing.T) {
	home := t.TempDir()
	bin := "/Applications/larik desktop.app/Contents/MacOS/larik-desktop"
	if err := Install(home, bin); err != nil {
		t.Fatal(err)
	}
	if ok, err := Status(home); err != nil || !ok {
		t.Fatalf("status %v: %v", ok, err)
	}
	if err := Install(home, bin); err != nil {
		t.Fatalf("repeat install: %v", err)
	}
	for _, name := range Names() {
		data, err := os.ReadFile(filepath.Join(home, ".agents", "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), shellQuote(bin)) || strings.Contains(string(data), "{{BIN}}") {
			t.Fatalf("%s has no installed binary path", name)
		}
	}
	if err := Remove(home); err != nil {
		t.Fatal(err)
	}
	if ok, err := Status(home); err != nil || ok {
		t.Fatalf("status after removal %v: %v", ok, err)
	}
	conflict := filepath.Join(home, ".agents", "skills", names[0], "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(conflict), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conflict, []byte("another skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(home, bin); err == nil {
		t.Fatal("install overwrote another skill")
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", names[1])); !os.IsNotExist(err) {
		t.Fatal("conflict partially installed another skill")
	}
	if err := Remove(home); err == nil {
		t.Fatal("remove deleted another skill")
	}
	if err := os.Remove(conflict); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Dir(conflict)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Dir(conflict)); err != nil {
		t.Fatal(err)
	}
	if err := Install(home, bin); err == nil {
		t.Fatal("install followed a skill-directory symlink")
	}
}

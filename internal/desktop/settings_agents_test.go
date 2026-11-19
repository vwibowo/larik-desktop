package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"larik/internal/integration"
)

func TestCustomAgentSkillSettingsTracksTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	bin := filepath.Join(home, "Applications", "larik-desktop.app", "Contents", "MacOS", "larik-desktop")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	a := &App{prefs: defaultSettings(), settingsTab: 4, chooseSkillsFolder: func() (string, error) { return root, nil }}
	view := ui.NewTester(a.settingsView, 740, 900)
	if !view.HasText("Other agent harnesses") || !view.HasText("Install in another agent…") {
		t.Fatal("custom harness Settings controls are missing")
	}
	if err := view.Click("Install in another agent…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if ok, err := integration.StatusAt(root); err != nil || !ok {
		t.Fatalf("folder picker install: %v, %v", ok, err)
	}
	if !view.HasText("Installed") || !view.HasText("Update") || !view.HasText("Remove") || !view.HasText("Forget") {
		t.Fatal("managed target actions are missing")
	}
	if err := view.Click("Update"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("Forget"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if targets, err := integration.CustomTargets(); err != nil || len(targets) != 0 {
		t.Fatalf("forget retained target: %v, %v", targets, err)
	}
	if _, err := os.Stat(filepath.Join(root, "larik-workspace", "SKILL.md")); err != nil {
		t.Fatalf("forget removed skill file: %v", err)
	}
	root = t.TempDir()
	if err := view.Click("Install in another agent…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	if ok, err := integration.StatusAt(root); err != nil || ok {
		t.Fatalf("Settings remove left skills installed: %v, %v", ok, err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("Settings remove deleted skills root: %v", err)
	}
	root = t.TempDir()
	foreign := filepath.Join(root, "larik-workspace")
	if err := os.Mkdir(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("Install in another agent…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.settingsField != "custom-skills" || a.settingsErr == "" || !view.HasText(a.settingsErr) {
		t.Fatalf("folder conflict did not appear beside control: %s: %s", a.settingsField, a.settingsErr)
	}
}

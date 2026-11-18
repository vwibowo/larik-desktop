package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"larik/internal/session"
)

func TestSettingsMigrationAndValidation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"appearance":"dark","fontSize":16}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := readSettings()
	if p.Appearance != "dark" || p.FontSize != 16 || !p.NativeBlur || !p.NotifyFinished || !p.KeepSessions || p.StartDirectory != "current" || p.LightTheme != "" || p.DarkTheme != "" {
		t.Fatalf("older settings lost defaults: %+v", p)
	}
	p.Shell = "relative-shell"
	if err := p.validate(); err == nil {
		t.Fatal("relative shell accepted")
	}
	p.Shell = "/bin/sh"
	p.StartDirectory, p.CustomDir = "custom", filepath.Join(dir, "missing")
	if err := p.validate(); err == nil {
		t.Fatal("missing starting folder accepted")
	}
	p.CustomDir = dir
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	p.LightTheme = "no such Ghostty theme"
	if err := p.validate(); err == nil {
		t.Fatal("unknown Ghostty theme accepted")
	}
}

func TestSettingsSaveRollsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	a := &App{prefs: defaultSettings()}
	a.updateSettings("shell", func(p *settings) { p.Shell = "relative-shell" })
	if a.prefs.Shell != "" || a.settingsField != "shell" {
		t.Fatal("invalid shell changed active settings")
	}
	// A directory at the target path forces the atomic rename to fail.
	if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	a.updateSettings("appearance", func(p *settings) { p.Appearance = "dark" })
	if a.prefs.Appearance != "system" || a.settingsField != "appearance" {
		t.Fatal("failed write changed active settings")
	}
}

func TestSettingsPersistFalseValues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	p := defaultSettings()
	p.NativeBlur = false
	p.NotifyFinished = false
	p.KeepSessions = false
	p.Shell = "/bin/sh"
	p.StartDirectory = "home"
	p.Cursor = "underline"
	p.OptionAsAlt = true
	p.CopyOnSelect = true
	p.LightTheme = "Catppuccin Latte"
	p.DarkTheme = "Catppuccin Mocha"
	if err := p.save(); err != nil {
		t.Fatal(err)
	}
	if got := readSettings(); got != p {
		t.Fatalf("saved settings changed after reload: got %+v, want %+v", got, p)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temporary settings file remains: %v", err)
	}
}

func TestNewTabDirectoryPreference(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	p := &Pane{Info: session.Info{Dir: "/tmp/focused"}}
	a := &App{tabs: []*Tab{{Focus: p}}, prefs: defaultSettings()}
	if got := a.newTabDir(); got != "/tmp/focused" {
		t.Fatalf("focused pane directory = %q", got)
	}
	a.prefs.StartDirectory = "home"
	if got := a.newTabDir(); got != home {
		t.Fatalf("home directory = %q", got)
	}
	a.prefs.StartDirectory, a.prefs.CustomDir = "custom", "/tmp/chosen"
	if got := a.newTabDir(); got != "/tmp/chosen" {
		t.Fatalf("chosen directory = %q", got)
	}
}

func TestThemePickerSelection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	a := &App{prefs: defaultSettings()}
	for _, step := range []struct {
		dark bool
		name string
	}{
		{false, "Catppuccin Latte"},
		{true, "Catppuccin Mocha"},
	} {
		a.openThemePicker(step.dark)
		tester := ui.NewTester(a.settingsView, 740, 560)
		if _, ok := tester.Find("Search themes"); !ok {
			t.Fatal("theme search is missing")
		}
		tester.Type(step.name)
		tester.Frame()
		if err := tester.Click(step.name); err != nil {
			t.Fatal(err)
		}
		if step.dark && a.prefs.DarkTheme != step.name || !step.dark && a.prefs.LightTheme != step.name {
			t.Fatalf("selected theme was not applied: %+v", a.prefs)
		}
	}
	if got := readSettings(); got.LightTheme != a.prefs.LightTheme || got.DarkTheme != a.prefs.DarkTheme {
		t.Fatalf("theme choices were not saved: %+v", got)
	}
	light, dark := a.prefs.terminalThemes()
	wantLight, _ := terminal.GhosttyTheme("Catppuccin Latte")
	wantDark, _ := terminal.GhosttyTheme("Catppuccin Mocha")
	if light.Background != wantLight.Background || dark.Background != wantDark.Background {
		t.Fatal("selected themes did not resolve to Ghostty colors")
	}
}

func TestSettingsSectionsAndBlur(t *testing.T) {
	a := &App{prefs: defaultSettings()}
	for section, want := range []string{"Native blur", "Login shell", "Command finished", "Keep and restore sessions"} {
		a.settingsTab = section
		tester := ui.NewTester(a.settingsView, 680, 540)
		if !tester.HasText(want) {
			t.Fatalf("section %d missing %q", section, want)
		}
	}
	a.settingsTab = 1
	a.prefs.Shell = "/bin/sh"
	terminalView := ui.NewTester(a.settingsView, 680, 540)
	if !terminalView.HasText("Pending: existing sessions use the older server") {
		t.Fatal("custom shell pending state is missing")
	}
	if bounds, ok := terminalView.Find("System"); !ok || bounds.X < 209 {
		t.Fatalf("login shell choice overlaps the sidebar: bounds=%+v found=%v", bounds, ok)
	}
	a.settingsTab = 0
	if alpha := ui.NewTester(a.settingsView, 680, 540).Image().RGBAAt(400, 5).A; alpha == 255 {
		t.Fatal("blurred settings chrome is opaque")
	}
	a.prefs.NativeBlur = false
	if alpha := ui.NewTester(a.settingsView, 680, 540).Image().RGBAAt(400, 5).A; alpha != 255 {
		t.Fatalf("solid settings chrome alpha = %d", alpha)
	}
}

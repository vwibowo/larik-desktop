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
	if p.Appearance != "dark" || p.FontSize != 16 || !p.NativeBlur || !p.NotifyFinished || !p.KeepSessions || p.StartDirectory != "current" || p.LightTheme != "" || p.DarkTheme != "" || p.BackgroundTint != "" || p.BackgroundOpacity != defaultBackgroundOpacity {
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
	p.LightTheme = ""
	p.BackgroundTint = "#bad-color"
	if err := p.validate(); err == nil {
		t.Fatal("invalid background color accepted")
	}
	p.BackgroundTint = "#4285F4"
	p.BackgroundOpacity = 1.1
	if err := p.validate(); err == nil {
		t.Fatal("invalid background opacity accepted")
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
	p.BackgroundTint = "#4285F4"
	p.BackgroundOpacity = 0.35
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

func TestBackgroundColorAndOpacity(t *testing.T) {
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	a := &App{prefs: defaultSettings()}
	a.updateSettings("background", func(p *settings) {
		p.BackgroundTint = "#167AAB"
		p.BackgroundOpacity = 0.5
	})
	if a.settingsErr != "" || readSettings().BackgroundTint != "#167AAB" || readSettings().BackgroundOpacity != 0.5 {
		t.Fatalf("background preference was not saved: %+v, %s", a.prefs, a.settingsErr)
	}
	view := ui.NewTester(a.view, 680, 540)
	pixel := view.Image().RGBAAt(400, 100)
	if pixel.B <= pixel.R || pixel.A < 120 || pixel.A > 135 {
		t.Fatalf("custom translucent background pixel = %+v", pixel)
	}
	if bar := view.Image().RGBAAt(400, 20); bar == pixel {
		t.Fatal("custom background left the toolbar without a contrasting surface")
	}
	a.prefs.BackgroundOpacity = 0
	if pixel := ui.NewTester(a.view, 680, 540).Image().RGBAAt(400, 100); pixel.A != 0 {
		t.Fatalf("zero opacity background alpha = %d", pixel.A)
	}
	a.prefs.NativeBlur = false
	if pixel := ui.NewTester(a.view, 680, 540).Image().RGBAAt(400, 100); pixel.A != 255 || pixel.B <= pixel.R {
		t.Fatalf("solid custom background pixel = %+v", pixel)
	}
	a.prefs.NativeBlur = true
	a.prefs.BackgroundOpacity = 0.5
	settingsView := ui.NewTester(a.settingsView, 740, 560)
	if !settingsView.HasText("Terminal background") || !settingsView.HasText("Background opacity") {
		t.Fatal("background controls are missing from Appearance")
	}
	defaultSettingsView := ui.NewTester((&App{prefs: defaultSettings()}).settingsView, 740, 560)
	if got, want := settingsView.Image().RGBAAt(400, 5), defaultSettingsView.Image().RGBAAt(400, 5); got != want {
		t.Fatalf("custom terminal color changed Settings chrome: got %+v, want %+v", got, want)
	}
	if err := settingsView.Click("Terminal background color"); err != nil {
		t.Fatal(err)
	}
	if !settingsView.HasText("Green") {
		t.Fatal("background color picker did not open")
	}
	if err := settingsView.Click("Green"); err != nil {
		t.Fatal(err)
	}
	if a.prefs.BackgroundTint != "#22C55E" {
		t.Fatalf("background color picker selected %q", a.prefs.BackgroundTint)
	}
	settingsView.Key(0, ui.KeyEscape)
	if err := settingsView.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	if a.prefs.BackgroundTint != "" || a.prefs.BackgroundOpacity != defaultBackgroundOpacity {
		t.Fatalf("Reset did not restore the Larik background: %+v", a.prefs)
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

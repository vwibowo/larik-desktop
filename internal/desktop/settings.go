package desktop

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"larik/internal/session"
)

type settings struct {
	Appearance        string  `json:"appearance"`
	BackgroundTint    string  `json:"backgroundTint,omitempty"`
	BackgroundOpacity float64 `json:"backgroundOpacity"`
	PaneBackground    string  `json:"paneBackground,omitempty"`
	PaneTint          string  `json:"paneTint,omitempty"`
	LightTheme        string  `json:"lightTheme,omitempty"`
	DarkTheme         string  `json:"darkTheme,omitempty"`
	FontSize          float32 `json:"fontSize"`
	NativeBlur        bool    `json:"nativeBlur"`
	Shell             string  `json:"shell,omitempty"`
	StartDirectory    string  `json:"startDirectory"`
	CustomDir         string  `json:"customDir,omitempty"`
	Cursor            string  `json:"cursor"`
	OptionAsAlt       bool    `json:"optionAsAlt"`
	CopyOnSelect      bool    `json:"copyOnSelect"`
	NotifyFinished    bool    `json:"notifyFinished"`
	NotifyBell        bool    `json:"notifyBell"`
	KeepSessions      bool    `json:"keepSessions"`
}

const defaultFontSize = 12
const defaultBackgroundOpacity = 0.24

func defaultSettings() settings {
	return settings{
		Appearance: "system", BackgroundOpacity: defaultBackgroundOpacity, PaneBackground: "larik", FontSize: defaultFontSize, NativeBlur: true,
		StartDirectory: "current", Cursor: "block", NotifyFinished: true, KeepSessions: true,
	}
}

func readSettings() settings {
	p := defaultSettings()
	if data, err := os.ReadFile(filepath.Join(session.Dir(), "settings.json")); err == nil {
		_ = json.Unmarshal(data, &p)
	}
	if p.FontSize < 8 || p.FontSize > 32 {
		p.FontSize = defaultFontSize
	}
	if p.Appearance != "light" && p.Appearance != "dark" {
		p.Appearance = "system"
	}
	if _, err := parseBackgroundTint(p.BackgroundTint); err != nil {
		p.BackgroundTint = ""
	}
	if math.IsNaN(p.BackgroundOpacity) || math.IsInf(p.BackgroundOpacity, 0) || p.BackgroundOpacity < 0 || p.BackgroundOpacity > 1 {
		p.BackgroundOpacity = defaultBackgroundOpacity
	}
	if p.PaneBackground != "terminal" && p.PaneBackground != "custom" {
		p.PaneBackground = "larik"
	}
	if _, err := parseBackgroundTint(p.PaneTint); err != nil {
		p.PaneTint = ""
	}
	if p.PaneBackground == "custom" && p.PaneTint == "" {
		p.PaneBackground = "larik"
	}
	if p.StartDirectory != "home" && p.StartDirectory != "custom" {
		p.StartDirectory = "current"
	}
	if p.Cursor != "bar" && p.Cursor != "underline" {
		p.Cursor = "block"
	}
	if p.LightTheme != "" {
		if _, err := terminal.GhosttyTheme(p.LightTheme); err != nil {
			p.LightTheme = ""
		}
	}
	if p.DarkTheme != "" {
		if _, err := terminal.GhosttyTheme(p.DarkTheme); err != nil {
			p.DarkTheme = ""
		}
	}
	return p
}

func (p settings) validate() error {
	if _, err := parseBackgroundTint(p.BackgroundTint); err != nil {
		return fmt.Errorf("background color: %w", err)
	}
	if math.IsNaN(p.BackgroundOpacity) || math.IsInf(p.BackgroundOpacity, 0) || p.BackgroundOpacity < 0 || p.BackgroundOpacity > 1 {
		return fmt.Errorf("background opacity must be between 0 and 100%%")
	}
	if p.PaneBackground != "larik" && p.PaneBackground != "terminal" && p.PaneBackground != "custom" {
		return fmt.Errorf("choose a pane background style")
	}
	if _, err := parseBackgroundTint(p.PaneTint); err != nil {
		return fmt.Errorf("pane color: %w", err)
	}
	if p.PaneBackground == "custom" && p.PaneTint == "" {
		return fmt.Errorf("choose a pane color")
	}
	if p.LightTheme != "" {
		if _, err := terminal.GhosttyTheme(p.LightTheme); err != nil {
			return fmt.Errorf("light terminal theme: %w", err)
		}
	}
	if p.DarkTheme != "" {
		if _, err := terminal.GhosttyTheme(p.DarkTheme); err != nil {
			return fmt.Errorf("dark terminal theme: %w", err)
		}
	}
	if p.Shell != "" {
		if !filepath.IsAbs(p.Shell) {
			return fmt.Errorf("shell must be an absolute path")
		}
		stat, err := os.Stat(p.Shell)
		if err != nil || stat.IsDir() || stat.Mode()&0o111 == 0 {
			return fmt.Errorf("shell must be an executable file")
		}
	}
	if p.StartDirectory == "custom" {
		if !filepath.IsAbs(p.CustomDir) {
			return fmt.Errorf("starting folder must be an absolute path")
		}
		stat, err := os.Stat(p.CustomDir)
		if err != nil || !stat.IsDir() {
			return fmt.Errorf("starting folder must exist")
		}
	}
	return nil
}

func parseBackgroundTint(value string) (ui.Color, error) {
	if value == "" {
		return ui.Color{}, nil
	}
	if len(value) != 7 || value[0] != '#' {
		return ui.Color{}, fmt.Errorf("use a six-digit hex color")
	}
	n, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return ui.Color{}, fmt.Errorf("use a six-digit hex color")
	}
	return ui.RGB(uint8(n>>16), uint8(n>>8), uint8(n)), nil
}

func (p settings) terminalThemes() (light, dark *terminal.Theme) {
	light, dark = lightTerminal, darkTerminal
	if p.LightTheme != "" {
		if selected, err := terminal.GhosttyTheme(p.LightTheme); err == nil {
			light = selected
		}
	}
	if p.DarkTheme != "" {
		if selected, err := terminal.GhosttyTheme(p.DarkTheme); err == nil {
			dark = selected
		}
	}
	return light, dark
}

func (p settings) save() error {
	if err := os.MkdirAll(session.Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(session.Dir(), "settings.json")
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func (p settings) applyAppearance() {
	switch p.Appearance {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

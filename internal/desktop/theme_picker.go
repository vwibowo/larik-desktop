package desktop

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func themeLabel(name string) string {
	if name == "" {
		return "Larik default"
	}
	return name
}

func (a *App) openThemePicker(dark bool) {
	a.themePickerDark = dark
	a.themePickerOpen = true
	a.themeSearch = ""
	a.themeNames = terminal.GhosttyThemes()
}

func (a *App) themeSetting(c *ui.Context, k *palette, dark bool) {
	title, detail, field, name := "Light terminal theme", "Used when the app is light.", "light-theme", a.prefs.LightTheme
	if dark {
		title, detail, field, name = "Dark terminal theme", "Used when the app is dark.", "dark-theme", a.prefs.DarkTheme
	}
	light, darkTheme := a.prefs.terminalThemes()
	selected := light
	if dark {
		selected = darkTheme
	}
	a.settingCard(c, k, title, detail, field, func() {
		ui.Row(c).FillWidth().Height(34).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(30, 26).Center().Radius(6).Background(selected.Background).Border(1, k.border).Children(func() {
				ui.Text(c, "Aa").FontSize(11).FontWeight(600).TextColor(selected.Foreground)
			})
			ui.Text(c, themeLabel(name)).FontSize(12.5).TextColor(k.text).Grow(1).SingleLine().Ellipsis("…")
			if ui.Button(c, "Choose…").Label("Choose " + title).Clicked() {
				a.openThemePicker(dark)
			}
		})
	})
}

func (a *App) themePicker(c *ui.Context, k *palette) {
	if !a.themePickerOpen {
		return
	}
	mode, field, name := "Light", "light-theme", a.prefs.LightTheme
	if a.themePickerDark {
		mode, field, name = "Dark", "dark-theme", a.prefs.DarkTheme
	}
	light, dark := a.prefs.terminalThemes()
	preview := light
	if a.themePickerDark {
		preview = dark
	}
	query := strings.ToLower(strings.TrimSpace(a.themeSearch))
	var names []string
	if query == "" || strings.Contains("larik default", query) {
		names = append(names, "")
	}
	for _, candidate := range a.themeNames {
		if strings.Contains(strings.ToLower(candidate), query) {
			names = append(names, candidate)
		}
	}
	ui.DialogBase(c, &a.themePickerOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.32))
		panel.Width(460).MaxHeight(500).Radius(14).Background(k.focused).Border(1, k.border).
			Shadow(0, 18, 48, 0, k.shadow).Clip()
		ui.Row(c).Height(52).Padding(0, 18).AlignItems(ui.Center).Children(func() {
			ui.Text(c, mode+" terminal theme").FontSize(16).Bold().TextColor(k.text).Grow(1)
			if ui.Button(c, "Done").Clicked() {
				a.themePickerOpen = false
			}
		})
		ui.Column(c).Padding(0, 18).Gap(10).Children(func() {
			ui.Column(c).Height(70).Padding(10, 12).Gap(4).Radius(9).Background(preview.Background).Children(func() {
				ui.Text(c, "larik ~ % echo hello").FontSize(12).TextColor(preview.Foreground)
				ui.Text(c, "hello").FontSize(12).TextColor(preview.Foreground)
			})
			in := ui.TextInputBase(c, &a.themeSearch).FillWidth().Height(32).FontSize(13).
				Placeholder("Search Ghostty themes…").AutoFocus().Label("Search themes")
			if in.Shortcut(0, ui.KeyEscape) {
				a.themePickerOpen = false
			}
			if a.settingsField == field && a.settingsErr != "" {
				ui.Text(c, a.settingsErr).FontSize(11.5).TextColor(ui.Hex("#c34b50")).Wrap()
			}
		})
		ui.Scroll(c).Height(275).Padding(8, 12, 12, 12).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				for _, candidate := range names {
					row := ui.Row(c.Key(candidate)).FillWidth().Height(34).Padding(0, 10).Radius(7).
						AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(themeLabel(candidate))
					if candidate == name {
						row.Background(k.selected)
					} else if row.Hovered() {
						row.Background(k.hover)
					}
					row.Children(func() {
						ui.Text(c, themeLabel(candidate)).FontSize(12.5).TextColor(k.text).Grow(1)
						if candidate == name {
							ui.Text(c, "✓").FontSize(12).TextColor(k.text)
						}
					})
					if row.Clicked() {
						if a.themePickerDark {
							a.updateSettings(field, func(p *settings) { p.DarkTheme = candidate })
						} else {
							a.updateSettings(field, func(p *settings) { p.LightTheme = candidate })
						}
					}
				}
				if len(names) == 0 {
					ui.Text(c, "No themes found").Padding(10).FontSize(12).TextColor(k.muted)
				}
			})
		})
		ui.Text(c, fmt.Sprintf("%d themes", len(names))).Padding(0, 18, 12, 18).FontSize(11).TextColor(k.muted)
	})
}

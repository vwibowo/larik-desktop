package desktop

import (
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/sfsymbols"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func (a *App) vibrancy() mygo.Vibrancy {
	if a.prefs.NativeBlur {
		return mygo.VibrancyUnderWindow
	}
	return mygo.VibrancyNone
}

func (a *App) keepTerminalMaterialActive() {
	// macOS fades an inactive window's visual effect toward an opaque fill.
	// Settings takes focus, but the terminal should keep its glass material.
	if a.win != nil {
		a.win.SetActiveAppearance(a.settingsWin != nil && a.prefs.NativeBlur)
	}
}

func (a *App) cursorStyle() terminal.CursorStyle {
	switch a.prefs.Cursor {
	case "bar":
		return terminal.CursorBar
	case "underline":
		return terminal.CursorUnderline
	default:
		return terminal.CursorBlock
	}
}

func (a *App) copyOnSelect() terminal.CopyOnSelect {
	if a.prefs.CopyOnSelect {
		return terminal.CopyOnSelectClipboard
	}
	return terminal.CopyOnSelectNone
}

func (a *App) updateSettings(field string, change func(*settings)) {
	next := a.prefs
	change(&next)
	if err := next.validate(); err != nil {
		a.settingsField, a.settingsErr = field, err.Error()
		return
	}
	if err := next.save(); err != nil {
		a.settingsField, a.settingsErr = field, err.Error()
		return
	}
	old := a.prefs
	a.prefs = next
	a.settingsField, a.settingsErr = "", ""
	if old.Appearance != next.Appearance {
		next.applyAppearance()
	}
	if old.FontSize != next.FontSize || old.CopyOnSelect != next.CopyOnSelect || old.LightTheme != next.LightTheme || old.DarkTheme != next.DarkTheme || old.PaneBackground != next.PaneBackground || old.PaneTint != next.PaneTint {
		lightTheme, darkTheme := next.viewThemes()
		for _, t := range a.tabs {
			for _, p := range t.panes() {
				if p.Term == nil {
					continue
				}
				if old.FontSize != next.FontSize {
					p.Term.SetFont(terminalFont(next.FontSize))
				}
				if old.CopyOnSelect != next.CopyOnSelect {
					p.Term.SetCopyOnSelect(a.copyOnSelect())
				}
				if old.LightTheme != next.LightTheme || old.DarkTheme != next.DarkTheme || old.PaneBackground != next.PaneBackground || old.PaneTint != next.PaneTint {
					p.Term.SetTheme(lightTheme, darkTheme)
				}
			}
		}
	}
	if old.NativeBlur != next.NativeBlur {
		for _, win := range []*mygo.Window{a.win, a.settingsWin} {
			if win != nil {
				win.SetVibrancy(a.vibrancy())
			}
		}
		a.keepTerminalMaterialActive()
	}
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

func (a *App) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Focus()
		return
	}
	a.shellDraft = a.prefs.Shell
	if a.shellDraft == "" {
		a.shellDraft = os.Getenv("SHELL")
		if a.shellDraft == "" {
			a.shellDraft = "/bin/zsh"
		}
	}
	a.folderDraft = a.prefs.CustomDir
	if a.folderDraft == "" {
		a.folderDraft = a.currentDir()
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: "Settings", Width: 740, Height: 560, MinWidth: 620, MinHeight: 440,
		StateKey: "settings", TitleBarStyle: mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 16, Y: 15},
		Transparent:          true, Vibrancy: a.vibrancy(), Content: ui.View(a.settingsView),
	})
	a.settingsWin = win
	a.keepTerminalMaterialActive()
	win.OnClosed(func() {
		a.settingsWin, a.themePickerOpen = nil, false
		a.keepTerminalMaterialActive()
	})
	win.Focus()
}

func (a *App) settingsView(c *ui.Context) {
	k := colors(c)
	c.Root().Background(ui.Transparent)
	sections := []struct{ name, icon string }{
		{"Appearance", "paintbrush"}, {"Terminal", "terminal"},
		{"Notifications", "bell"}, {"Sessions", "square.stack"},
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).
		Draw(func(p *ui.Painter, r ui.Rect) { gradient(p, r, k, !a.prefs.NativeBlur) }).Children(func() {
		ui.Column(c).Width(184).FillHeight().Padding(58, 10, 14, 10).Gap(4).Background(k.track).Children(func() {
			ui.Row(c).Height(38).Padding(0, 12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Larik Desktop").FontSize(13).Bold().TextColor(k.text)
			})
			for i, section := range sections {
				row := ui.Row(c.Key(i)).Height(38).Padding(0, 11).Gap(10).Radius(8).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(section.name)
				if a.settingsTab == i {
					row.Background(k.selected)
				} else if row.Hovered() {
					row.Background(k.hover)
				}
				row.Children(func() {
					sfsymbols.Symbol(c, section.icon).Size(14).Color(k.muted)
					ui.Text(c, section.name).FontSize(13).TextColor(k.text)
				})
				if row.Clicked() {
					a.settingsTab = i
				}
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			ui.Row(c).Height(64).Padding(0, 26).AlignItems(ui.Center).Children(func() {
				ui.Text(c, sections[a.settingsTab].name).FontSize(20).Bold().TextColor(k.text).Grow(1)
				if ui.Button(c, "Open Terminal").Clicked() {
					a.openTerminal()
				}
			})
			ui.Box(c).Height(1).FillWidth().Background(k.border)
			ui.Scroll(c).Grow(1).MinHeight(0).Padding(6, 26, 24, 26).Children(func() {
				switch a.settingsTab {
				case 1:
					a.terminalSettings(c, k)
				case 2:
					a.notificationSettings(c, k)
				case 3:
					a.sessionSettings(c, k)
				default:
					a.appearanceSettings(c, k)
				}
			})
		})
	})
	a.themePicker(c, k)
}

func (a *App) settingCard(c *ui.Context, k *palette, title, detail, field string, controls func()) {
	ui.Column(c).FillWidth().Padding(13, 0).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(k.text)
		if detail != "" {
			ui.Text(c, detail).FontSize(12).TextColor(k.muted).Wrap()
		}
		controls()
		if a.settingsField == field && a.settingsErr != "" {
			ui.Text(c, a.settingsErr).FontSize(11.5).TextColor(ui.Hex("#c34b50")).Wrap()
		}
	})
	ui.Box(c).Height(1).FillWidth().Background(k.border)
}

func (a *App) settingToggle(c *ui.Context, k *palette, title, detail, field string, value bool, change func(bool)) {
	ui.Row(c).FillWidth().Padding(14, 0).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(k.text)
			ui.Text(c, detail).FontSize(12).TextColor(k.muted).Wrap()
		})
		if ui.Switch(c, &value).Label(title).Changed() {
			change(value)
		}
	})
	if a.settingsField == field && a.settingsErr != "" {
		ui.Text(c, a.settingsErr).FontSize(11.5).TextColor(ui.Hex("#c34b50")).Wrap()
	}
	ui.Box(c).Height(1).FillWidth().Background(k.border)
}

type choice struct{ value, label string }

func choiceRow(c *ui.Context, k *palette, selected string, items []choice, onClick func(string)) {
	ui.Row(c).FillWidth().Gap(5).Children(func() {
		for _, item := range items {
			b := ui.Row(c.Key(item.value)).Height(29).Padding(0, 10).Radius(7).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(item.label)
			if item.value == selected {
				b.Background(k.selected).Border(1, k.focusedBorder)
			} else {
				b.Background(k.track)
			}
			b.Children(func() { ui.Text(c, item.label).FontSize(12).TextColor(k.text) })
			if b.Clicked() {
				onClick(item.value)
			}
		}
	})
}

func (a *App) appearanceSettings(c *ui.Context, k *palette) {
	ui.Column(c).Children(func() {
		a.settingCard(c, k, "Color scheme", "Follow macOS or choose an appearance.", "appearance", func() {
			choiceRow(c, k, a.prefs.Appearance, []choice{{"system", "System"}, {"light", "Light"}, {"dark", "Dark"}}, a.setAppearance)
		})
		a.settingToggle(c, k, "Native blur", "Use macOS's translucent window material.", "blur", a.prefs.NativeBlur, func(value bool) {
			a.updateSettings("blur", func(p *settings) { p.NativeBlur = value })
		})
		a.backgroundSettings(c, k)
		a.paneBackgroundSettings(c, k)
		a.themeSetting(c, k, false)
		a.themeSetting(c, k, true)
		a.settingCard(c, k, "Terminal font size", "Applies to every open pane immediately.", "font", func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "−").Disabled(a.prefs.FontSize <= 8).Clicked() {
					a.setFontSize(a.prefs.FontSize - 1)
				}
				ui.Text(c, fmt.Sprintf("%.0f pt", a.prefs.FontSize)).Width(58).FontSize(13).TextColor(k.text)
				if ui.Button(c, "+").Disabled(a.prefs.FontSize >= 32).Clicked() {
					a.setFontSize(a.prefs.FontSize + 1)
				}
			})
		})
	})
}

func (a *App) backgroundSettings(c *ui.Context, k *palette) {
	a.settingCard(c, k, "Terminal background", "Choose a color or keep Larik's gradient. Opacity applies with Native Blur.", "background", func() {
		color := k.top
		if a.prefs.BackgroundTint != "" {
			color, _ = parseBackgroundTint(a.prefs.BackgroundTint)
		}
		color.A = uint8(a.prefs.BackgroundOpacity*255 + 0.5)
		ui.Row(c).FillWidth().Height(32).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.ColorWell(c, &color).Label("Terminal background color").Changed() {
				a.updateSettings("background", func(p *settings) {
					p.BackgroundTint = fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B)
					p.BackgroundOpacity = float64(color.A) / 255
				})
			}
			label := "Larik gradient"
			if a.prefs.BackgroundTint != "" {
				label = a.prefs.BackgroundTint
			}
			ui.Text(c, label).FontSize(12.5).TextColor(k.text).Grow(1)
			if ui.Button(c, "Reset").Disabled(a.prefs.BackgroundTint == "" && a.prefs.BackgroundOpacity == defaultBackgroundOpacity).Clicked() {
				a.updateSettings("background", func(p *settings) {
					p.BackgroundTint = ""
					p.BackgroundOpacity = defaultBackgroundOpacity
				})
			}
		})
		opacity := a.prefs.BackgroundOpacity * 100
		ui.Row(c).FillWidth().Height(30).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Background opacity").FontSize(12).TextColor(k.muted).Width(130)
			if ui.Slider(c, &opacity, 0, 100).Label("Background opacity").Grow(1).Disabled(!a.prefs.NativeBlur).Changed() {
				a.updateSettings("background", func(p *settings) { p.BackgroundOpacity = opacity / 100 })
			}
			ui.Text(c, fmt.Sprintf("%.0f%%", opacity)).FontSize(12).TextColor(k.text).Width(38)
		})
		if !a.prefs.NativeBlur {
			ui.Text(c, "Turn on Native blur to adjust opacity.").FontSize(11.5).TextColor(k.muted)
		}
	})
}

func (a *App) paneBackgroundSettings(c *ui.Context, k *palette) {
	a.settingCard(c, k, "Pane background", "Use Larik's surface, follow the active terminal theme, or choose a separate color. Changes apply to open panes.", "pane-background", func() {
		choiceRow(c, k, a.prefs.PaneBackground,
			[]choice{{"larik", "Larik"}, {"terminal", "Terminal theme"}, {"custom", "Custom color"}}, func(value string) {
				a.updateSettings("pane-background", func(p *settings) {
					p.PaneBackground = value
					if value == "custom" && p.PaneTint == "" {
						color := p.paneColor(c.Theme().Dark)
						p.PaneTint = fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B)
					}
				})
			})
		if a.prefs.PaneBackground == "custom" {
			color, _ := parseBackgroundTint(a.prefs.PaneTint)
			ui.Row(c).FillWidth().Height(32).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.ColorWell(c, &color).Label("Custom pane color").Changed() {
					a.updateSettings("pane-background", func(p *settings) {
						p.PaneTint = fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B)
					})
				}
				ui.Text(c, a.prefs.PaneTint).FontSize(12.5).TextColor(k.text)
			})
		}
		if a.prefs.PaneBackground != "larik" && a.prefs.NativeBlur {
			ui.Text(c, "Pane translucency follows Background opacity above.").FontSize(11.5).TextColor(k.muted)
		}
	})
}

func (a *App) terminalSettings(c *ui.Context, k *palette) {
	ui.Column(c).Children(func() {
		a.settingCard(c, k, "Login shell", "Applies to new shells. Current shells keep running.", "shell", func() {
			choiceRow(c, k, map[bool]string{true: "custom", false: "system"}[a.prefs.Shell != ""],
				[]choice{{"system", "System"}, {"custom", "Custom"}}, func(value string) {
					if value == "system" {
						a.updateSettings("shell", func(p *settings) { p.Shell = "" })
					} else {
						a.updateSettings("shell", func(p *settings) { p.Shell = a.shellDraft })
					}
				})
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				in := ui.TextInput(c, &a.shellDraft).Grow(1).MinWidth(0).Label("Shell executable")
				if in.Submitted() {
					a.updateSettings("shell", func(p *settings) { p.Shell = a.shellDraft })
				}
				if ui.Button(c, "Apply").Clicked() {
					a.updateSettings("shell", func(p *settings) { p.Shell = a.shellDraft })
				}
				if ui.Button(c, "Choose…").Clicked() {
					if paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.settingsWin, Title: "Choose a shell"}); err == nil && len(paths) > 0 {
						a.shellDraft = paths[0]
						a.updateSettings("shell", func(p *settings) { p.Shell = paths[0] })
					}
				}
			})
			if a.prefs.Shell != "" && !a.shellOverride {
				ui.Text(c, "Pending: existing sessions use the older server. This shell starts after they end and the server restarts.").FontSize(11.5).TextColor(k.muted).Wrap()
			}
		})
		a.settingCard(c, k, "New tab starts in", "Splits always inherit the focused pane's folder.", "folder", func() {
			choiceRow(c, k, a.prefs.StartDirectory,
				[]choice{{"current", "Focused pane"}, {"home", "Home"}, {"custom", "Custom folder"}}, func(value string) {
					a.updateSettings("folder", func(p *settings) {
						p.StartDirectory = value
						if value == "custom" && p.CustomDir == "" {
							p.CustomDir = a.folderDraft
						}
					})
				})
			if a.prefs.StartDirectory == "custom" {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					in := ui.TextInput(c, &a.folderDraft).Grow(1).MinWidth(0).Label("Starting folder")
					if in.Submitted() {
						a.updateSettings("folder", func(p *settings) { p.CustomDir = a.folderDraft })
					}
					if ui.Button(c, "Apply").Clicked() {
						a.updateSettings("folder", func(p *settings) { p.CustomDir = a.folderDraft })
					}
					if ui.Button(c, "Choose…").Clicked() {
						if paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.settingsWin, Title: "Choose a starting folder", Directory: true}); err == nil && len(paths) > 0 {
							a.folderDraft = paths[0]
							a.updateSettings("folder", func(p *settings) { p.CustomDir = paths[0] })
						}
					}
				})
			}
		})
		a.settingCard(c, k, "Cursor shape", "New panes use this cursor; existing panes keep theirs.", "cursor", func() {
			choiceRow(c, k, a.prefs.Cursor, []choice{{"block", "Block"}, {"bar", "Bar"}, {"underline", "Underline"}}, func(value string) {
				a.updateSettings("cursor", func(p *settings) { p.Cursor = value })
			})
		})
		a.settingToggle(c, k, "Option as Alt", "Affects new panes only.", "alt", a.prefs.OptionAsAlt, func(value bool) {
			a.updateSettings("alt", func(p *settings) { p.OptionAsAlt = value })
		})
		a.settingToggle(c, k, "Copy on select", "Copy selected terminal text immediately.", "copy", a.prefs.CopyOnSelect, func(value bool) {
			a.updateSettings("copy", func(p *settings) { p.CopyOnSelect = value })
		})
	})
}

func (a *App) notificationSettings(c *ui.Context, k *palette) {
	ui.Column(c).Children(func() {
		a.settingToggle(c, k, "Command finished", "Notify when a background command finishes.", "notify-finished", a.prefs.NotifyFinished, func(value bool) {
			a.updateSettings("notify-finished", func(p *settings) { p.NotifyFinished = value })
		})
		a.settingToggle(c, k, "Terminal bell", "Notify when an unfocused pane rings its bell.", "notify-bell", a.prefs.NotifyBell, func(value bool) {
			a.updateSettings("notify-bell", func(p *settings) { p.NotifyBell = value })
		})
	})
}

func (a *App) sessionSettings(c *ui.Context, k *palette) {
	a.settingToggle(c, k, "Keep and restore sessions", "When off, closing the terminal window ends its shells and clears its layout.", "sessions", a.prefs.KeepSessions, func(value bool) {
		a.updateSettings("sessions", func(p *settings) { p.KeepSessions = value })
	})
	if a.settingsField == "window" && a.settingsErr != "" {
		ui.Text(c, a.settingsErr).TextColor(ui.Hex("#c34b50"))
	}
}

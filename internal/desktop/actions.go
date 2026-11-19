package desktop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

type action struct {
	name, detail, keys, accel string
	run                       func(*App)
}

func (a *App) actions() []action {
	return []action{
		{"New tab", "Choose a pane type", "⌘T", "CmdOrCtrl+T", func(a *App) {
			if a.win == nil {
				a.openTerminal()
			} else {
				_ = a.newTab(a.newTabDir())
			}
		}},
		{"Split right", "Add a pane beside this one", "⌘D", "CmdOrCtrl+D", func(a *App) { a.split(false) }},
		{"Split down", "Add a pane below this one", "⇧⌘D", "CmdOrCtrl+Shift+D", func(a *App) { a.split(true) }},
		{"Close pane", "Close the focused pane", "⌘W", "CmdOrCtrl+W", func(a *App) {
			if t := a.tab(); t != nil {
				a.closePane(t.Focus)
			}
		}},
		{"Close tab", "Close every pane in this tab", "⇧⌘W", "CmdOrCtrl+Shift+W", func(a *App) { a.closeTab(a.tab()) }},
		{"Zoom pane", "Fill the tab with the focused pane", "⇧⌘↩", "CmdOrCtrl+Shift+Enter", func(a *App) { a.toggleZoom() }},
		{"Equalize panes", "Reset all divider positions", "⌃⌘=", "CmdOrCtrl+Ctrl+=", func(a *App) { a.equalize() }},
		{"Next tab", "", "⇧⌘]", "CmdOrCtrl+Shift+]", func(a *App) { a.cycleTab(1) }},
		{"Previous tab", "", "⇧⌘[", "CmdOrCtrl+Shift+[", func(a *App) { a.cycleTab(-1) }},
		{"Rename tab", "Give this tab a name", "⇧⌘R", "CmdOrCtrl+Shift+R", func(a *App) {
			if t := a.tab(); t != nil {
				a.renaming = t
				a.renameText = t.Name
				a.focusReq = nil
			}
		}},
		{"Focus left", "", "⌥⌘←", "CmdOrCtrl+Alt+Left", func(a *App) { a.moveFocus(-1, 0) }},
		{"Focus right", "", "⌥⌘→", "CmdOrCtrl+Alt+Right", func(a *App) { a.moveFocus(1, 0) }},
		{"Focus above", "", "⌥⌘↑", "CmdOrCtrl+Alt+Up", func(a *App) { a.moveFocus(0, -1) }},
		{"Focus below", "", "⌥⌘↓", "CmdOrCtrl+Alt+Down", func(a *App) { a.moveFocus(0, 1) }},
		{"Move divider left", "", "⌃⌘←", "CmdOrCtrl+Ctrl+Left", func(a *App) { a.moveDivider(false, -0.04) }},
		{"Move divider right", "", "⌃⌘→", "CmdOrCtrl+Ctrl+Right", func(a *App) { a.moveDivider(false, 0.04) }},
		{"Move divider up", "", "⌃⌘↑", "CmdOrCtrl+Ctrl+Up", func(a *App) { a.moveDivider(true, -0.04) }},
		{"Move divider down", "", "⌃⌘↓", "CmdOrCtrl+Ctrl+Down", func(a *App) { a.moveDivider(true, 0.04) }},
		{"Clear terminal", "Clear screen and scrollback", "⌘K", "CmdOrCtrl+K", func(a *App) { a.clearFocused() }},
		{"Restart shell", "Start a new shell in this pane", "", "", func(a *App) { a.restartFocused() }},
		{"Bigger text", "", "⌘+", "CmdOrCtrl+=", func(a *App) { a.setFontSize(a.prefs.FontSize + 1) }},
		{"Smaller text", "", "⌘−", "CmdOrCtrl+-", func(a *App) { a.setFontSize(a.prefs.FontSize - 1) }},
		{"Actual size", "", "⌘0", "CmdOrCtrl+0", func(a *App) { a.setFontSize(defaultFontSize) }},
		{"Appearance: System", "Follow macOS appearance", "", "", func(a *App) { a.setAppearance("system") }},
		{"Appearance: Light", "", "", "", func(a *App) { a.setAppearance("light") }},
		{"Appearance: Dark", "", "", "", func(a *App) { a.setAppearance("dark") }},
		{"Quit and end all sessions", "Stop every background shell", "⌥⌘Q", "CmdOrCtrl+Alt+Q", func(a *App) { a.endAll() }},
		{"Settings…", "Appearance, terminal, notifications, and sessions", "⌘,", "CmdOrCtrl+,", func(a *App) { a.openSettings() }},
		{"Find in pane", "Search the focused terminal or browser page", "⌘F", "CmdOrCtrl+F", func(a *App) { a.openFind() }},
		{"Next match", "Go to the next search match", "⌘G", "CmdOrCtrl+G", func(a *App) { a.findNext() }},
		{"Previous match", "Go to the previous search match", "⇧⌘G", "CmdOrCtrl+Shift+G", func(a *App) { a.findPrevious() }},
		{"Browser address", "Focus the address of the current browser pane", "⌘L", "CmdOrCtrl+L", func(a *App) { a.browserCommand("address") }},
		{"Reload page", "Reload the current browser pane", "⌘R", "CmdOrCtrl+R", func(a *App) { a.browserCommand("reload") }},
		{"Back", "Previous browser page", "⌘[", "CmdOrCtrl+[", func(a *App) { a.browserCommand("back") }},
		{"Forward", "Next browser page", "⌘]", "CmdOrCtrl+]", func(a *App) { a.browserCommand("forward") }},
	}
}

func (a *App) openFind() {
	if t := a.tab(); t != nil && t.Focus != nil && (t.Focus.Term != nil || t.Focus.Browser != nil) {
		t.Focus.FindOpen, t.Focus.FindFocus = true, true
		if t.Focus.Browser != nil {
			t.Focus.Browser.FocusAddress = false
		}
		a.focusReq = nil
	}
}

func (a *App) closeFind(p *Pane) {
	if p == nil || !p.FindOpen {
		return
	}
	if p.Term != nil {
		p.Term.EndSearch()
	}
	if p.Browser != nil {
		a.stopBrowserFind(p)
		if p.Browser.Web != nil {
			p.Browser.Web.Focus()
		}
	}
	p.FindOpen, p.FindFocus, p.FindQuery = false, false, ""
	p.FindStatus = terminal.SearchStatus{}
	if p.Term != nil {
		a.focusReq = p
	} else {
		a.focusReq = nil
	}
}

func (a *App) findNext() {
	if t := a.tab(); t != nil && t.Focus != nil && t.Focus.FindOpen && t.Focus.FindQuery != "" {
		if t.Focus.Term != nil {
			t.Focus.Term.SearchNext()
		} else if t.Focus.Browser != nil {
			a.browserFind(t.Focus, true, false)
		}
	}
}

func (a *App) findPrevious() {
	if t := a.tab(); t != nil && t.Focus != nil && t.Focus.FindOpen && t.Focus.FindQuery != "" {
		if t.Focus.Term != nil {
			t.Focus.Term.SearchPrevious()
		} else if t.Focus.Browser != nil {
			a.browserFind(t.Focus, true, true)
		}
	}
}

func (a *App) cycleTab(step int) {
	if len(a.tabs) < 2 {
		return
	}
	a.selectTab((a.active + step + len(a.tabs)) % len(a.tabs))
}

func (a *App) clearFocused() {
	if t := a.tab(); t != nil && t.Focus != nil && t.Focus.Term != nil {
		t.Focus.Term.Feed([]byte("\x1b[H\x1b[2J\x1b[3J"))
		if t.Focus.Info.Idle {
			t.Focus.Term.Send([]byte{0x0c})
		}
	}
}

func (a *App) restartFocused() {
	t := a.tab()
	if t == nil || t.Focus == nil || t.Focus.Kind != paneTerminal {
		return
	}
	old := t.Focus
	p, err := a.newPane(a.currentDir())
	if err != nil {
		a.err = err.Error()
		return
	}
	p.Tab, p.Node = t, old.Node
	old.Node.Pane = p
	old.Closed = true
	if old.Term != nil {
		old.Term.Close()
	}
	if old.Stream != nil {
		_ = old.Stream.Close()
	}
	_ = a.client.Kill(old.SID)
	if t.Zoom == old {
		t.Zoom = p
	}
	t.Focus = p
	a.focusReq = p
	a.changed()
}

func (a *App) setFontSize(size float32) {
	a.updateSettings("font", func(p *settings) { p.FontSize = min(max(size, 8), 32) })
}

func (a *App) setAppearance(source string) {
	a.updateSettings("appearance", func(p *settings) { p.Appearance = source })
}

func (a *App) endAll() {
	a.closing = true
	go func() { _ = a.client.Shutdown(); mygo.App.Quit() }()
}

func (a *App) menu() *mygo.Menu {
	actions := a.actions()
	item := func(index int) *mygo.MenuItem {
		act := actions[index]
		return &mygo.MenuItem{Label: act.name, Accelerator: act.accel, Click: func(*mygo.MenuItem, *mygo.Window) {
			if a.win != nil {
				a.win.Update(func() { act.run(a) })
			}
		}}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout}, mygo.Separator(),
			{Label: "Settings…", Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
			mygo.Separator(), {Role: mygo.RoleServices}, mygo.Separator(),
			{Role: mygo.RoleHide}, {Role: mygo.RoleHideOthers}, {Role: mygo.RoleUnhide},
			mygo.Separator(), {Role: mygo.RoleQuit},
		}},
		{Label: "Pane", Submenu: []*mygo.MenuItem{item(0), mygo.Separator(), item(1), item(2), mygo.Separator(), item(3), item(4), mygo.Separator(), item(19), item(26)}},
		{Label: "Edit", Submenu: []*mygo.MenuItem{{Role: mygo.RoleUndo}, {Role: mygo.RoleRedo}, mygo.Separator(), {Role: mygo.RoleCopy}, {Role: mygo.RolePaste}, {Role: mygo.RoleSelectAll}, mygo.Separator(), item(28), item(29), item(30), mygo.Separator(), item(18)}},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{Label: "Command palette", Accelerator: "CmdOrCtrl+Shift+P", Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.win != nil {
					a.win.Update(func() { a.openPalette() })
				}
			}},
			mygo.Separator(), item(5), item(6), mygo.Separator(), item(10), item(11), item(12), item(13), mygo.Separator(), item(14), item(15), item(16), item(17),
			mygo.Separator(), item(20), item(21), item(22),
			{Label: "Appearance", Submenu: []*mygo.MenuItem{item(23), item(24), item(25)}},
			{Role: mygo.RoleToggleFullScreen},
		}},
		{Label: "Tabs", Submenu: []*mygo.MenuItem{item(7), item(8), item(9), mygo.Separator(),
			{Label: "Show tab 1", Accelerator: "CmdOrCtrl+1", Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.win != nil {
					a.win.Update(func() { a.selectTab(0) })
				}
			}},
			{Label: "Show tab 2", Accelerator: "CmdOrCtrl+2", Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.win != nil {
					a.win.Update(func() { a.selectTab(1) })
				}
			}},
			{Label: "Show tab 3", Accelerator: "CmdOrCtrl+3", Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.win != nil {
					a.win.Update(func() { a.selectTab(2) })
				}
			}},
		}},
		{Label: "Browser", Submenu: []*mygo.MenuItem{item(31), item(32), mygo.Separator(), item(33), item(34)}},
		{Role: mygo.RoleWindowMenu},
	})
}

func (a *App) openPalette() {
	a.paletteOpen = true
	a.paletteQuery = ""
	a.paletteIndex = 0
	a.focusReq = nil
}

type paletteEntry struct {
	label, detail, keys string
	run                 func()
}

func (a *App) paletteEntries() []paletteEntry {
	var out []paletteEntry
	query := strings.ToLower(strings.TrimSpace(a.paletteQuery))
	for _, action := range a.actions() {
		act := action
		if !matches(query, act.name+" "+act.detail) {
			continue
		}
		out = append(out, paletteEntry{act.name, act.detail, act.keys, func() { act.run(a) }})
	}
	for i, t := range a.tabs {
		tab := t
		name, _ := tab.label()
		label := fmt.Sprintf("Tab %d: %s", i+1, name)
		if matches(query, label) {
			out = append(out, paletteEntry{label, "Switch tab", "", func() { a.selectTab(slices.Index(a.tabs, tab)) }})
		}
		for _, p := range t.panes() {
			pane := p
			program, dir := p.label()
			label := program + "  " + dir
			if matches(query, label) {
				out = append(out, paletteEntry{label, "Jump to pane", "", func() { a.selectTab(slices.Index(a.tabs, tab)); a.focus(pane) }})
			}
		}
	}
	return out
}

func matches(query, text string) bool {
	if query == "" {
		return true
	}
	text = strings.ToLower(text)
	for _, r := range query {
		i := strings.IndexRune(text, r)
		if i < 0 {
			return false
		}
		text = text[i+len(string(r)):]
	}
	return true
}

func (a *App) commandPalette(c *ui.Context, k *palette) {
	if !a.paletteOpen {
		return
	}
	entries := a.paletteEntries()
	if a.paletteIndex >= len(entries) {
		a.paletteIndex = max(0, len(entries)-1)
	}
	ui.DialogBase(c, &a.paletteOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(37, 24, 39, 0.18)).Justify(ui.Start).Padding(76, 0, 0, 0)
		panel.Width(550).MaxHeight(440).Radius(15).Background(k.focused).Border(1, k.border).Shadow(0, 20, 55, 0, k.shadow).Clip()
		ui.Row(c).Padding(12, 16).Gap(9).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "⌕").FontSize(19).TextColor(k.faint)
			in := ui.TextInputBase(c, &a.paletteQuery).Grow(1).FontSize(14).Placeholder("Command or pane…").AutoFocus()
			if in.Changed() {
				a.paletteIndex = 0
			}
			if in.Shortcut(0, ui.KeyDown) {
				a.paletteIndex = min(a.paletteIndex+1, len(entries)-1)
			}
			if in.Shortcut(0, ui.KeyUp) {
				a.paletteIndex = max(a.paletteIndex-1, 0)
			}
			if in.Submitted() && len(entries) > 0 {
				a.paletteOpen = false
				entry := entries[a.paletteIndex]
				a.later(c, entry.run)
			}
		})
		ui.Scroll(c).Height(float32(min(max(len(entries), 1)*34+12, 350))).Padding(6).Children(func() {
			if len(entries) == 0 {
				ui.Text(c, "No matches").FontSize(13).TextColor(k.faint).Padding(12)
			}
			for i, entry := range entries {
				row := ui.Row(c.Key(i)).Height(34).Padding(0, 10).Gap(9).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if i == a.paletteIndex {
					row.Background(k.track)
				} else if row.Hovered() {
					row.Background(k.hover)
				}
				if row.Clicked() {
					a.paletteOpen = false
					a.later(c, entry.run)
				}
				row.Children(func() {
					ui.Text(c, entry.label).FontSize(13).TextColor(k.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
					if entry.detail != "" {
						ui.Text(c, entry.detail).FontSize(11.5).TextColor(k.faint).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
					}
					if entry.keys != "" {
						ui.Text(c, entry.keys).FontSize(11).TextColor(k.muted)
					}
				})
			}
		})
	})
	if t := a.tab(); !a.paletteOpen && t != nil && t.Focus != nil && a.focusReq == nil && !t.Focus.FindOpen {
		a.focusReq = t.Focus
	}
}

func (a *App) shortcuts(c *ui.Context) {
	if c.Shortcut(ui.Ctrl, ui.KeyTab) {
		a.cycleTab(1)
	}
	if c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyTab) {
		a.cycleTab(-1)
	}
	if c.Shortcut(ui.Cmd, ui.KeyP) || c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP) {
		a.openPalette()
	}
	for i := 1; i <= 9; i++ {
		if c.Shortcut(ui.Cmd, ui.Key0+ui.Key(i)) {
			if i == 9 {
				a.selectTab(len(a.tabs) - 1)
			} else {
				a.selectTab(i - 1)
			}
		}
	}
}

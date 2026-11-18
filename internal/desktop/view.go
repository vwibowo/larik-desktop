package desktop

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sfsymbols"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

const (
	titleHeight      = 44
	paneGap          = 8
	paneHeaderHeight = 34
)

func (a *App) view(c *ui.Context) {
	a.services = c.Services()
	a.runPending()
	k := a.windowColors(c)
	c.Root().Background(ui.Transparent)
	ui.Column(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { gradient(p, r, k, !a.prefs.NativeBlur) }).Children(func() {
		a.titleBar(c, k)
		canvas := ui.Box(c).Grow(1).MinHeight(0).Padding(0, paneGap, paneGap, paneGap)
		if k.customBackground && a.tab() != nil {
			canvas.Background(colors(c).focused)
		}
		canvas.Children(func() {
			if tab := a.tab(); tab != nil {
				if tab.Zoom != nil {
					a.paneCard(c, k, tab.Zoom).Fill()
				} else {
					a.nodeView(c, k, tab.Root).Fill()
				}
			}
		})
	})
	a.shortcuts(c)
	a.commandPalette(c, k)
	if a.err != "" {
		a.errorBanner(c, k)
	}
	if a.saveDue {
		if elapsed := time.Since(a.saveAt); elapsed > 350*time.Millisecond {
			a.saveNow()
		} else {
			c.After(350*time.Millisecond - elapsed)
		}
	}
	if t := a.tab(); t != nil && a.win != nil {
		name, detail := t.label()
		title := strings.TrimSpace(name + " " + detail)
		if title != a.lastTitle {
			a.lastTitle = title
			a.win.SetTitle(title)
		}
	}
}

func (a *App) titleBar(c *ui.Context, k *palette) {
	left := max(c.TitleBar().Left+12, 82)
	bar := ui.Row(c).Height(titleHeight).Padding(0, 12, 0, left).Gap(12).AlignItems(ui.Center).DragWindow()
	if k.customBackground {
		bar.Background(colors(c).focused)
	}
	bar.Children(func() {
		a.hostChip(c, k)
		a.tabsView(c, k)
		ui.Spacer(c).MinWidth(30)
		if chromeButton(c, k, "command", "Command palette").Clicked() {
			a.paletteOpen = true
			a.paletteQuery = ""
			a.paletteIndex = 0
		}
		if chromeButton(c, k, "plus", "New tab").Clicked() {
			a.later(c, func() { _ = a.newTab(a.newTabDir()) })
		}
	})
}

func chromeButton(c *ui.Context, k *palette, symbol, label string) ui.Element {
	b := ui.Box(c).Size(29, 29).Center().Radius(10).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(label).Tooltip(label)
	if b.Hovered() || b.Pressed() {
		b.Background(k.hover)
	}
	b.Children(func() { sfsymbols.Symbol(c, symbol).Size(17).Color(k.muted) })
	return b
}

func (a *App) tabsView(c *ui.Context, k *palette) {
	if len(a.tabs) == 0 {
		return
	}
	tabs := slices.Clone(a.tabs)
	track := ui.Row(c).Height(32).Shrink(1).MinWidth(0).Padding(2).Gap(2).Radius(16).
		Background(k.track).ClipX().Role(ui.RoleTabList).Label("Tabs")
	track.Children(func() {
		for i, tab := range tabs {
			active := i == a.active
			name, detail := tab.label()
			item := ui.Row(c.Key(tab.ID)).Height(28).Basis(170).Shrink(1).MinWidth(92).
				Padding(0, 9).Gap(7).AlignItems(ui.Center).Radius(14).Cursor(ui.CursorPointer).
				Role(ui.RoleTab).Selected(active).Label(strings.TrimSpace(name + " " + detail))
			if active {
				item.Background(k.selected).Shadow(0, 1, 4, 0, k.shadow)
			} else if item.Hovered() {
				item.Background(k.hover)
			}
			item.Drag(tab)
			if dropped, ok := ui.Drop[*Tab](item); ok && dropped != tab {
				a.later(c, func() { a.moveTab(dropped, i) })
			}
			if item.Clicked() {
				a.later(c, func() { a.selectTab(i) })
			}
			if item.DoubleClicked() {
				a.renaming = tab
				a.renameText = tab.Name
			}
			item.ContextMenu(func(m *ui.Menu) { a.tabMenu(m, tab) })
			item.Children(func() {
				for j, pane := range tab.panes() {
					if j == 2 {
						break
					}
					sfsymbols.Symbol(c, programSymbol(pane.Info.Program)).Size(13).Color(k.muted)
				}
				if a.renaming == tab {
					in := ui.TextInput(c, &a.renameText).Grow(1).MinWidth(0).Height(22).AutoFocus()
					if in.Submitted() {
						tab.Name = strings.TrimSpace(a.renameText)
						a.renaming = nil
						a.focusReq = tab.Focus
						a.changed()
					}
					if in.Shortcut(0, ui.KeyEscape) {
						a.renaming = nil
						a.focusReq = tab.Focus
					}
				} else {
					ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(k.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
					if detail != "" {
						ui.Text(c, detail).FontSize(11.5).TextColor(k.faint).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0)
					}
					if tabAttention(tab) {
						ui.Box(c).Size(7, 7).Radius(4).Background(k.attention)
					} else if tabBusy(tab) {
						ui.Box(c).Size(7, 7).Radius(4).Background(k.active)
					}
				}
			})
		}
	})
}

func tabAttention(t *Tab) bool {
	for _, p := range t.panes() {
		if p.Attention {
			return true
		}
	}
	return false
}
func tabBusy(t *Tab) bool {
	for _, p := range t.panes() {
		if !p.Info.Idle && time.Since(p.Info.LastOutput) < 1500*time.Millisecond {
			return true
		}
	}
	return false
}

func (a *App) tabMenu(m *ui.Menu, t *Tab) {
	if m.Item("Rename tab").Chosen() {
		a.renaming = t
		a.renameText = t.Name
	}
	if m.Item("Reset name").Disabled(t.Name == "").Chosen() {
		t.Name = ""
		a.changed()
	}
	m.Separator()
	i := slices.Index(a.tabs, t)
	if m.Item("Move left").Disabled(i <= 0).Chosen() {
		a.moveTab(t, i-1)
	}
	if m.Item("Move right").Disabled(i >= len(a.tabs)-1).Chosen() {
		a.moveTab(t, i+1)
	}
	m.Separator()
	if m.Item("Close tab").Chosen() {
		a.closeTab(t)
	}
}

func (a *App) nodeView(c *ui.Context, k *palette, n *Node) ui.Element {
	if n == nil {
		return ui.Box(c)
	}
	if n.Pane != nil {
		return a.paneCard(c, k, n.Pane)
	}
	var box ui.Element
	if n.Vertical {
		box = ui.Column(c.Key(n.ID))
	} else {
		box = ui.Row(c.Key(n.ID))
	}
	box.MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
	bounds := box.Bounds()
	first, second, ratio := n.A, n.B, n.Ratio
	box.Children(func() {
		a.nodeView(c, k, first).Grow(ratio).Basis(0).MinWidth(0).MinHeight(0)
		div := ui.Box(c.Key("divider")).Role(ui.RoleSplitter).Label("Pane divider")
		if n.Vertical {
			div.Height(paneGap).Cursor(ui.CursorResizeRow)
		} else {
			div.Width(paneGap).Cursor(ui.CursorResizeColumn)
		}
		if dx, dy, ok := div.Dragged(); ok {
			total, movement := bounds.W-paneGap, dx
			if n.Vertical {
				total, movement = bounds.H-paneGap, dy
			}
			if total > 0 {
				n.Ratio = min(max(n.Ratio+movement/total, 0.1), 0.9)
				a.changed()
			}
		}
		if div.DoubleClicked() {
			n.Ratio = 0.5
			a.changed()
		}
		a.nodeView(c, k, second).Grow(1 - ratio).Basis(0).MinWidth(0).MinHeight(0)
	})
	return box
}

func (a *App) paneCard(c *ui.Context, k *palette, p *Pane) ui.Element {
	focused := p.Tab != nil && p.Tab.Focus == p && a.tab() == p.Tab
	card := ui.Column(c.Key(p.ID)).Radius(12).Clip().MinWidth(0).MinHeight(0)
	pk, bg, border := a.paneColors(c, k, focused)
	card.Background(bg).Border(1, border).Shadow(0, 5, 18, 0, k.shadow)
	hovered := card.Hovered()
	card.Children(func() {
		a.paneHeader(c, pk, p, focused || hovered)
		if p.FindOpen {
			a.findBar(c, pk, p)
		}
		ui.Box(c).Grow(1).MinHeight(0).Padding(0, 5, 6, 5).Children(func() {
			if p.Term == nil {
				return
			}
			view := terminal.View(c, p.Term).Fill()
			if a.focusReq == p && a.tab() == p.Tab {
				view.Focus()
				a.focusReq = nil
			}
			if view.Focused() && p.Tab.Focus != p {
				a.focus(p)
			}
			if view.Focused() {
				p.Attention = false
			}
		})
	})
	p.Bounds = card.Bounds()
	if card.Clicked() && p.Tab.Focus != p {
		a.focus(p)
	}
	return card
}

func (a *App) findBar(c *ui.Context, k *palette, p *Pane) {
	ui.Row(c).Height(40).Padding(0, 8).Gap(6).AlignItems(ui.Center).Background(k.track).Children(func() {
		in := ui.TextInputBase(c, &p.FindQuery).Grow(1).MinWidth(80).Height(28).
			FontSize(12.5).Placeholder("Find in terminal").Background(k.card).
			Border(1, k.border).Radius(7).Padding(0, 8).Label("Find in terminal")
		if p.FindFocus {
			in.Focus()
			p.FindFocus = false
		}
		if in.Changed() {
			p.Term.Search(p.FindQuery)
		}
		if in.Submitted() && p.FindQuery != "" {
			p.Term.SearchNext()
		}
		if in.Shortcut(0, ui.KeyEscape) {
			a.closeFind(p)
		}
		status := ""
		if p.FindQuery != "" {
			status = fmt.Sprintf("%d of %d", p.FindStatus.Selected, p.FindStatus.Total)
			if p.FindStatus.Searching {
				status += "…"
			}
		}
		ui.Text(c, status).Width(66).FontSize(11.5).TextColor(k.muted).SingleLine()
		if chromeButton(c, k, "chevron.up", "Previous match").Clicked() {
			p.Term.SearchPrevious()
		}
		if chromeButton(c, k, "chevron.down", "Next match").Clicked() {
			p.Term.SearchNext()
		}
		if chromeButton(c, k, "xmark", "Close find").Clicked() {
			a.closeFind(p)
		}
	})
	ui.Box(c).Height(1).FillWidth().Background(k.border)
}

func (a *App) paneHeader(c *ui.Context, k *palette, p *Pane, showButtons bool) {
	name, dir := p.label()
	header := ui.Row(c).Height(paneHeaderHeight).Padding(0, 8, 0, 12).Gap(7).AlignItems(ui.Center).MinWidth(0)
	header.Drag(p)
	if dropped, ok := ui.Drop[*Pane](header); ok {
		a.later(c, func() { a.swapPanes(dropped, p) })
	}
	if header.Clicked() {
		a.focus(p)
	}
	if header.DoubleClicked() {
		a.focus(p)
		a.toggleZoom()
	}
	header.Children(func() {
		sfsymbols.Symbol(c, programSymbol(p.Info.Program)).Size(15).Color(k.text)
		ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(k.text).SingleLine().Ellipsis("…").Shrink(0)
		ui.Text(c, dir).FontSize(12).TextColor(k.muted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if p.Attention {
			ui.Box(c).Size(7, 7).Radius(4).Background(k.attention)
		} else if !p.Info.Idle && time.Since(p.Info.LastOutput) < 1500*time.Millisecond {
			ui.Box(c).Size(7, 7).Radius(4).Background(k.active)
		}
		if showButtons {
			if chromeButton(c, k, "rectangle.split.2x1", "Split right").Clicked() {
				a.later(c, func() { a.focus(p); a.split(false) })
			}
			if chromeButton(c, k, "rectangle.split.1x2", "Split down").Clicked() {
				a.later(c, func() { a.focus(p); a.split(true) })
			}
			symbol := "arrow.up.left.and.arrow.down.right"
			if p.Tab.Zoom == p {
				symbol = "arrow.down.right.and.arrow.up.left"
			}
			if chromeButton(c, k, symbol, "Zoom pane").Clicked() {
				a.later(c, func() { a.focus(p); a.toggleZoom() })
			}
			if chromeButton(c, k, "xmark", "Close pane").Clicked() {
				a.later(c, func() { a.closePane(p) })
			}
		}
	})
}

func programSymbol(program string) string {
	switch strings.ToLower(program) {
	case "node", "bun", "deno":
		return "hexagon"
	case "git", "lazygit":
		return "arrow.triangle.branch"
	case "ssh":
		return "network"
	case "codex", "claude":
		return "sparkles"
	case "vim", "nvim", "nano":
		return "chevron.left.forwardslash.chevron.right"
	default:
		return "terminal"
	}
}

func (a *App) hostChip(c *ui.Context, k *palette) {
	name, _ := os.Hostname()
	name = strings.TrimSuffix(name, ".local")
	chip := ui.Row(c).Gap(7).Padding(3, 7).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Role(ui.RoleButton).Label("Host " + name).MaxWidth(190).Shrink(0)
	if chip.Hovered() || a.hostOpen {
		chip.Background(k.hover)
	}
	if chip.Clicked() {
		a.hostOpen = !a.hostOpen
	}
	chip.Children(func() {
		sfsymbols.Symbol(c, "laptopcomputer").Size(18).Color(k.muted)
		ui.Column(c).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(12.5).FontWeight(700).TextColor(k.text).SingleLine().Ellipsis("…")
			ui.Text(c, "This Mac").FontSize(10.5).TextColor(k.faint)
		})
	})
	ui.PopoverBase(c, chip, &a.hostOpen, func(panel ui.Element) {
		panel.Margin(8, 0, 0, 0).Width(290).Padding(14).Gap(9).Radius(13).Background(k.focused).Border(1, k.border).Shadow(0, 12, 28, 0, k.shadow)
		ui.Text(c, name).FontSize(15).Bold().TextColor(k.text)
		hostRow(c, k, "Server", fmt.Sprintf("pid %d", a.serverPID))
		hostRow(c, k, "Sessions", fmt.Sprintf("%d panes", a.paneCount()))
		ui.Text(c, "Shells keep running after the window closes.").FontSize(11.5).TextColor(k.muted)
	})
}

func (a *App) paneCount() int {
	n := 0
	for _, t := range a.tabs {
		n += len(t.panes())
	}
	return n
}
func hostRow(c *ui.Context, k *palette, label, value string) {
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, label).Width(68).FontSize(12).TextColor(k.faint)
		ui.Text(c, value).FontSize(12).TextColor(k.text)
	})
}

func (a *App) errorBanner(c *ui.Context, k *palette) {
	ui.Overlay(c, func() {
		ui.Row(c).Absolute().Bottom(16).Left(0).Right(0).Justify(ui.Center).PassThrough().Children(func() {
			ui.Row(c).Gap(8).Padding(9, 12).Radius(10).Background(k.focused).Border(1, k.border).Children(func() {
				ui.Text(c, a.err).FontSize(12).TextColor(k.text)
				if chromeButton(c, k, "xmark", "Dismiss error").Clicked() {
					a.err = ""
				}
			})
		})
	})
}

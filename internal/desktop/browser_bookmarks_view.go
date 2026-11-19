package desktop

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) bookmarksDialog(c *ui.Context, k *palette) {
	if !a.bookmarksOpen {
		return
	}
	if a.bookmarksPane == nil || a.bookmarksPane.Closed {
		a.bookmarksOpen = false
		return
	}
	var items []bookmark
	if a.bookmarks != nil {
		items = append(items, a.bookmarks.items...)
	}
	query := strings.ToLower(strings.TrimSpace(a.bookmarksQuery))
	ui.DialogBase(c, &a.bookmarksOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.32))
		panel.Width(500).MaxHeight(510).Radius(14).Background(k.focused).Border(1, k.border).
			Shadow(0, 18, 48, 0, k.shadow).Clip()
		ui.Row(c).Height(52).Padding(0, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Bookmarks").FontSize(16).Bold().TextColor(k.text).Grow(1)
			if ui.Button(c, "Done").Clicked() {
				a.bookmarksOpen = false
			}
		})
		ui.Column(c).Padding(0, 16, 8, 16).Gap(7).Children(func() {
			in := ui.TextInputBase(c, &a.bookmarksQuery).FillWidth().Height(32).FontSize(13).
				Placeholder("Search bookmarks…").AutoFocus().Label("Search bookmarks")
			if in.Shortcut(0, ui.KeyEscape) {
				a.bookmarksOpen = false
			}
			if a.bookmarkError != "" {
				ui.Text(c, a.bookmarkError).FontSize(11.5).TextColor(k.attention).Wrap()
			}
		})
		ui.Scroll(c).Height(300).Padding(4, 12, 12, 12).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				shown := 0
				for _, saved := range items {
					item := saved
					if query != "" && !strings.Contains(strings.ToLower(item.Title+" "+item.URL), query) {
						continue
					}
					shown++
					ui.Row(c.Key(item.URL)).Height(48).Padding(2, 8).Gap(8).AlignItems(ui.Center).
						Radius(8).Background(k.track).Children(func() {
						open := ui.Column(c).Grow(1).MinWidth(0).Gap(2).Cursor(ui.CursorPointer).
							Role(ui.RoleButton).Label("Open " + item.Title)
						open.Children(func() {
							ui.Text(c, item.Title).FontSize(12.5).FontWeight(600).TextColor(k.text).SingleLine().Ellipsis("…")
							ui.Text(c, item.URL).FontSize(11).TextColor(k.muted).SingleLine().Ellipsis("…")
						})
						if open.Clicked() {
							a.bookmarksOpen = false
							a.later(c, func() {
								if p := a.bookmarksPane; p != nil && !p.Closed && p.Browser != nil {
									p.Browser.Address = item.URL
									a.navigateBrowser(p)
								}
							})
						}
						if chromeButton(c, k, "trash", "Remove "+item.Title).Clicked() {
							if err := a.bookmarks.remove(item.URL); err != nil {
								a.bookmarkError = err.Error()
							} else {
								a.bookmarkError = ""
							}
						}
					})
				}
				if shown == 0 {
					message := "No bookmarks yet"
					if query != "" {
						message = "No matching bookmarks"
					}
					ui.Text(c, message).Padding(14).FontSize(12).TextColor(k.muted)
				}
			})
		})
		ui.Text(c, "Saved pages stay available when sessions are cleared.").Padding(0, 16, 14, 16).
			FontSize(11).TextColor(k.muted)
	})
}

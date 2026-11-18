package desktop

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func databaseSVG(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var (
	iconDatabase   = databaseSVG(`<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.7 4 3 9 3s9-1.3 9-3V5M3 12c0 1.7 4 3 9 3s9-1.3 9-3"/>`)
	iconPlus       = databaseSVG(`<path d="M12 5v14M5 12h14"/>`)
	iconBack       = databaseSVG(`<path d="m15 18-6-6 6-6"/>`)
	iconForward    = databaseSVG(`<path d="m9 18 6-6-6-6"/>`)
	iconReload     = databaseSVG(`<path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/>`)
	iconPanel      = databaseSVG(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>`)
	iconPanelRight = databaseSVG(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M15 3v18"/>`)
	iconCopy       = databaseSVG(`<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>`)
	iconExport     = databaseSVG(`<path d="M12 3v12m-4-4 4 4 4-4M4 17v3a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-3"/>`)
	iconRun        = databaseSVG(`<path d="m8 5 11 7-11 7V5z"/>`)
)

func keyText(key string) string {
	return strings.NewReplacer("Cmd+", "⌘", "Enter", "↩").Replace(key)
}

func (a *App) iconButton(c *ui.Context, theme *ui.Theme, key string, icon *ui.SVG, tip string, on bool, fn func()) ui.Element {
	b := ui.Box(c.Key(key)).Size(32, 32).Center().Focusable().Radius(6).Label(tip).Tooltip(tip).Children(func() {
		ui.Icon(c, icon).FontSize(15).TextColor(theme.Text.Alpha(0.75))
	})
	if on {
		b.Background(theme.Text.Alpha(0.1))
	} else if b.Hovered() {
		b.Background(theme.Text.Alpha(0.06))
	}
	if b.Clicked() {
		fn()
	}
	return b
}

package desktop

import (
	_ "embed"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/fonts/JetBrainsMono-Regular.ttf
var regularFont []byte

//go:embed assets/fonts/JetBrainsMono-Bold.ttf
var boldFont []byte

//go:embed assets/fonts/OFL.txt
var fontLicense []byte

func registerFonts() {
	ui.RegisterFont(regularFont, "JetBrains Mono")
	ui.RegisterFont(boldFont, "JetBrains Mono")
}

type palette struct {
	top, middle, bottom                  ui.Color
	card, focused, border, focusedBorder ui.Color
	text, muted, faint, hover            ui.Color
	track, selected, active, attention   ui.Color
	shadow                               ui.Color
}

var lightPalette = palette{
	top: ui.RGBA(243, 229, 241, 0.24), middle: ui.RGBA(236, 226, 229, 0.14), bottom: ui.RGBA(232, 223, 199, 0.20),
	card: ui.RGBA(255, 255, 255, 0.68), focused: ui.RGBA(255, 255, 255, 0.82),
	border: ui.RGBA(255, 255, 255, 0.67), focusedBorder: ui.RGBA(255, 255, 255, 0.90),
	text: ui.Hex("#292630"), muted: ui.Hex("#68636b"), faint: ui.Hex("#928a93"),
	hover: ui.RGBA(70, 45, 75, 0.08), track: ui.RGBA(255, 255, 255, 0.32),
	selected: ui.RGBA(255, 255, 255, 0.96), active: ui.Hex("#49a96e"),
	attention: ui.Hex("#d48838"), shadow: ui.RGBA(72, 47, 61, 0.12),
}

var darkPalette = palette{
	top: ui.RGBA(52, 41, 55, 0.30), middle: ui.RGBA(40, 36, 45, 0.20), bottom: ui.RGBA(38, 37, 31, 0.24),
	card: ui.RGBA(24, 23, 29, 0.60), focused: ui.RGBA(36, 35, 42, 0.76),
	border: ui.RGBA(255, 255, 255, 0.08), focusedBorder: ui.RGBA(255, 255, 255, 0.17),
	text: ui.Hex("#f3edf4"), muted: ui.Hex("#b8adb9"), faint: ui.Hex("#938a96"),
	hover: ui.RGBA(255, 255, 255, 0.09), track: ui.RGBA(255, 255, 255, 0.06),
	selected: ui.RGBA(255, 255, 255, 0.15), active: ui.Hex("#64ca85"),
	attention: ui.Hex("#f3ad58"), shadow: ui.RGBA(0, 0, 0, 0.28),
}

func colors(c *ui.Context) *palette {
	if c.Theme().Dark {
		return &darkPalette
	}
	return &lightPalette
}

func gradient(p *ui.Painter, r ui.Rect, k *palette, solid bool) {
	top, middle, bottom := k.top, k.middle, k.bottom
	if solid {
		top.A, middle.A, bottom.A = 255, 255, 255
	}
	p.FillGradient(r, ui.LinearGradient{From: top, To: middle, Angle: 180}, 0)
	p.FillGradient(ui.Rect{X: r.X, Y: r.Y + r.H*0.45, W: r.W, H: r.H * 0.55},
		ui.LinearGradient{From: middle, To: bottom, Angle: 180}, 0)
}

func terminalFont(size float32) terminal.Font {
	return terminal.Font{Family: "JetBrains Mono, SF Mono, Menlo, monospace", Size: size, LineHeight: 1.08}
}

var lightTerminal = &terminal.Theme{
	Foreground: ui.Hex("#28282e"), Background: ui.Hex("#fcfafb"),
	Cursor: ui.Hex("#393540"), Selection: ui.RGBA(121, 108, 168, 0.26),
}
var darkTerminal = &terminal.Theme{
	Foreground: ui.Hex("#eae6ec"), Background: ui.Hex("#252329"),
	Cursor: ui.Hex("#eae6ec"), Selection: ui.RGBA(171, 151, 218, 0.32),
}

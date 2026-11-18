package desktop

import (
	_ "embed"
	"math"

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
	customBackground                     bool
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

func (a *App) windowColors(c *ui.Context) *palette {
	k := *colors(c)
	opacity := float32(a.prefs.BackgroundOpacity)
	if tint, err := parseBackgroundTint(a.prefs.BackgroundTint); err == nil && a.prefs.BackgroundTint != "" {
		k.top = tint.Alpha(opacity)
		k.customBackground = true
	} else {
		factor := opacity / defaultBackgroundOpacity
		k.top = scaleAlpha(k.top, factor)
		k.middle = scaleAlpha(k.middle, factor)
		k.bottom = scaleAlpha(k.bottom, factor)
	}
	if a.prefs.NativeBlur {
		k.card = paneAlpha(k.card, opacity)
		k.focused = paneAlpha(k.focused, opacity)
	}
	return &k
}

// paneColors keeps the pane chrome and transparent terminal on one surface.
func (a *App) paneColors(c *ui.Context, k *palette, focused bool) (*palette, ui.Color, ui.Color) {
	bg, border := k.card, k.border
	if focused {
		bg, border = k.focused, k.focusedBorder
	}
	if a.prefs.PaneBackground == "larik" {
		return k, bg, border
	}
	base := a.prefs.paneColor(c.Theme().Dark)
	bg = ui.RGBA(base.R, base.G, base.B, float32(bg.A)/255)
	if !a.prefs.NativeBlur {
		bg.A = 255
	}
	if bg.A == 0 {
		return k, bg, border
	}
	pk := *k
	lightText := contrastRatio(base, ui.Hex("#F7F5F9")) > contrastRatio(base, ui.Hex("#1B1920"))
	if lightText {
		pk.text = ui.Hex("#F7F5F9")
		pk.muted = ui.Hex("#C6C1CC")
		pk.faint = ui.Hex("#A9A3AF")
		pk.hover = ui.RGBA(255, 255, 255, 0.10)
		pk.track = ui.RGBA(255, 255, 255, 0.08)
		pk.selected = ui.RGBA(255, 255, 255, 0.16)
		border = ui.RGBA(255, 255, 255, 0.20)
	} else {
		pk.text = ui.Hex("#1B1920")
		pk.muted = ui.Hex("#58535E")
		pk.faint = ui.Hex("#746E79")
		pk.hover = ui.RGBA(0, 0, 0, 0.07)
		pk.track = ui.RGBA(0, 0, 0, 0.05)
		pk.selected = ui.RGBA(0, 0, 0, 0.11)
		border = ui.RGBA(0, 0, 0, 0.15)
	}
	return &pk, bg, border
}

func (p settings) paneColor(dark bool) ui.Color {
	if p.PaneBackground == "custom" {
		if color, err := parseBackgroundTint(p.PaneTint); err == nil && p.PaneTint != "" {
			return color
		}
	}
	light, darkTheme := p.terminalThemes()
	if dark {
		return darkTheme.Background
	}
	return light.Background
}

func (p settings) viewThemes() (light, dark *terminal.Theme) {
	light, dark = p.terminalThemes()
	if p.PaneBackground != "custom" {
		return light, dark
	}
	background := p.paneColor(false)
	adjust := func(theme *terminal.Theme) *terminal.Theme {
		copy := *theme
		if contrastRatio(background, copy.Foreground) < 4.5 {
			black, white := ui.Hex("#000000"), ui.Hex("#FFFFFF")
			copy.Foreground = black
			if contrastRatio(background, white) > contrastRatio(background, black) {
				copy.Foreground = white
			}
		}
		if copy.Cursor.A != 0 && contrastRatio(background, copy.Cursor) < 3 {
			copy.Cursor = copy.Foreground
		}
		return &copy
	}
	return adjust(light), adjust(dark)
}

func contrastRatio(a, b ui.Color) float64 {
	luminance := func(c ui.Color) float64 {
		linear := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
	}
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

func scaleAlpha(color ui.Color, factor float32) ui.Color {
	color.A = uint8(max(0, min(255, float32(color.A)*factor+0.5)))
	return color
}

func paneAlpha(color ui.Color, opacity float32) ui.Color {
	baseline := float32(color.A) / 255
	var alpha float32
	if opacity <= defaultBackgroundOpacity {
		alpha = baseline * opacity / defaultBackgroundOpacity
	} else {
		alpha = baseline + (1-baseline)*(opacity-defaultBackgroundOpacity)/(1-defaultBackgroundOpacity)
	}
	color.A = uint8(max(0, min(255, alpha*255+0.5)))
	return color
}

func gradient(p *ui.Painter, r ui.Rect, k *palette, solid bool) {
	if k.customBackground {
		color := k.top
		if solid {
			color.A = 255
		}
		p.Fill(r, color, 0)
		return
	}
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

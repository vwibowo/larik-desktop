//go:build darwin

package desktop

import (
	"bytes"
	"net"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"larik/internal/session"
)

func TestThemeChangeUpdatesOpenPane(t *testing.T) {
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	conn, other := net.Pipe()
	defer conn.Close()
	defer other.Close()
	term, err := terminal.New(terminal.Options{
		Conn: conn, Font: terminalFont(12),
		Theme: lightTerminal, DarkTheme: darkTerminal, Transparent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	p := &Pane{ID: 3, Term: term, Info: session.Info{Shell: "sh", Dir: "/tmp", Idle: true}}
	n := &Node{ID: 2, Pane: p}
	tab := &Tab{ID: 1, Root: n, Focus: p}
	p.Tab, p.Node = tab, n
	a := &App{tabs: []*Tab{tab}, prefs: defaultSettings()}
	term.Feed([]byte("theme color test\r\n"))
	tester := ui.NewTester(a.view, 800, 500)
	tester.Frame()
	before := bytes.Clone(tester.Image().Pix)
	a.updateSettings("light-theme", func(s *settings) { s.LightTheme = "Catppuccin Latte" })
	if a.settingsErr != "" {
		t.Fatal(a.settingsErr)
	}
	tester.Frame()
	if bytes.Equal(before, tester.Image().Pix) {
		t.Fatal("open pane did not redraw with the selected light theme")
	}
	tester.SetDark(true)
	tester.Frame()
	before = bytes.Clone(tester.Image().Pix)
	a.updateSettings("dark-theme", func(s *settings) { s.DarkTheme = "Catppuccin Mocha" })
	if a.settingsErr != "" {
		t.Fatal(a.settingsErr)
	}
	tester.Frame()
	if bytes.Equal(before, tester.Image().Pix) {
		t.Fatal("open pane did not redraw with the selected dark theme")
	}
}

//go:build darwin

package desktop

import (
	"net"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"larik/internal/session"
)

func TestFindInFocusedPane(t *testing.T) {
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	conn, other := net.Pipe()
	defer conn.Close()
	defer other.Close()
	p := &Pane{ID: 3, Info: session.Info{Shell: "sh", Dir: "/tmp", Idle: true}}
	term, err := terminal.New(terminal.Options{
		Conn: conn, Font: terminalFont(12),
		OnSearch: func(status terminal.SearchStatus) { p.FindStatus = status },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	p.Term = term
	n := &Node{ID: 2, Pane: p}
	tab := &Tab{ID: 1, Root: n, Focus: p}
	p.Tab, p.Node = tab, n
	a := &App{tabs: []*Tab{tab}, prefs: defaultSettings()}
	a.paletteQuery = "find"
	found := false
	for _, entry := range a.paletteEntries() {
		if entry.label == "Find in pane" {
			entry.run()
			found = true
			break
		}
	}
	if !found {
		t.Fatal("command palette has no Find action")
	}
	if !p.FindOpen || !p.FindFocus {
		t.Fatal("find did not open for the focused pane")
	}
	tester := ui.NewTester(a.view, 800, 500)
	if _, ok := tester.Find("Find in terminal"); !ok {
		t.Fatal("find input is missing")
	}
	term.Feed([]byte("first needle\r\nsecond needle\r\n"))
	tester.Frame()
	tester.Type("needle")
	for range 100 {
		tester.Frame()
		if status := term.SearchStatus(); !status.Searching && status.Total == 2 {
			break
		}
	}
	if status := term.SearchStatus(); status.Total != 2 || status.Selected != 1 || p.FindStatus.Total != 2 {
		t.Fatalf("find status = %+v, pane status = %+v", status, p.FindStatus)
	}
	a.findNext()
	if status := term.SearchStatus(); status.Selected != 2 {
		t.Fatalf("next match selected %d, want 2", status.Selected)
	}
	a.findPrevious()
	if status := term.SearchStatus(); status.Selected != 1 {
		t.Fatalf("previous match selected %d, want 1", status.Selected)
	}
	a.closeFind(p)
	if status := term.SearchStatus(); status.Text != "" || p.FindOpen || p.FindQuery != "" {
		t.Fatalf("closing find retained search: %+v, pane %+v", status, p)
	}
}

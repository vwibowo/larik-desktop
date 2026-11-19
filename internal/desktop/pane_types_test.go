package desktop

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"larik/internal/session"
)

func TestNewTabsAndSplitsStartWithChooser(t *testing.T) {
	a := &App{prefs: defaultSettings()}
	if err := a.newTab("/tmp/project"); err != nil {
		t.Fatal(err)
	}
	first := a.tab().Focus
	if first.Kind != paneEmpty || first.Dir != "/tmp/project" || first.SID != "" {
		t.Fatalf("new tab started work before selection: %+v", first)
	}
	a.split(false)
	second := a.tab().Focus
	if second.Kind != paneEmpty || second.Dir != first.Dir || second.SID != "" || len(a.tab().panes()) != 2 {
		t.Fatalf("split did not create a chooser: %+v", second)
	}
	view := ui.NewTester(a.view, 900, 620)
	for _, label := range []string{"Terminal — T", "Browser — B", "Database — D"} {
		if _, ok := view.Find(label); !ok {
			t.Errorf("chooser missing %q", label)
		}
	}
}

func TestChooserShortcutTargetsFocusedEmptyPane(t *testing.T) {
	a := &App{prefs: defaultSettings(), newWebView: func(string) (*mygo.WebView, error) {
		return nil, errors.New("WKWebView unavailable")
	}}
	if err := a.newTab("/tmp"); err != nil {
		t.Fatal(err)
	}
	first := a.tab().Focus
	a.split(false)
	second := a.tab().Focus
	view := ui.NewTester(a.view, 900, 620)
	view.Key(0, ui.KeyB)
	view.Frame()
	if first.ChoiceError != "" || !strings.Contains(second.ChoiceError, "WKWebView unavailable") {
		t.Fatalf("B chose the wrong pane: first=%q second=%q", first.ChoiceError, second.ChoiceError)
	}
}

func TestChooserShortcutDoesNotRunBehindPalette(t *testing.T) {
	a := &App{prefs: defaultSettings(), newWebView: func(string) (*mygo.WebView, error) {
		return nil, errors.New("browser choice was invoked")
	}}
	if err := a.newTab("/tmp"); err != nil {
		t.Fatal(err)
	}
	p := a.tab().Focus
	a.paletteOpen = true
	view := ui.NewTester(a.view, 900, 620)
	view.Key(0, ui.KeyB)
	view.Frame()
	if p.Kind != paneEmpty || p.ChoiceError != "" {
		t.Fatalf("chooser shortcut ran behind palette: %+v", p)
	}
}

func TestBrowserCreationFailureKeepsChooser(t *testing.T) {
	a := &App{prefs: defaultSettings(), newWebView: func(string) (*mygo.WebView, error) { return nil, errors.New("WKWebView unavailable") }}
	_ = a.newTab("/tmp")
	p := a.tab().Focus
	a.choosePane(p, paneBrowser)
	if p.Kind != paneEmpty || p.Browser != nil || p.SID != "" || !strings.Contains(p.ChoiceError, "WKWebView unavailable") {
		t.Fatalf("browser failure changed the chooser: %+v", p)
	}
}

func TestMixedPaneSnapshotAndLegacyLayout(t *testing.T) {
	terminal := &Pane{ID: 10, Kind: paneTerminal, SID: "shell-1", Dir: "/tmp", Info: session.Info{Dir: "/tmp"}}
	browser := &Pane{ID: 11, Kind: paneBrowser, Dir: "/tmp", Browser: &browserState{URL: "https://example.com"}}
	empty := &Pane{ID: 12, Kind: paneEmpty, Dir: "/tmp"}
	root := &Node{Ratio: 0.5, A: &Node{Pane: terminal}, B: &Node{Ratio: 0.5, A: &Node{Pane: browser}, B: &Node{Pane: empty}}}
	tab := &Tab{Root: root, Focus: browser, Zoom: browser}
	a := &App{tabs: []*Tab{tab}}
	data, err := json.Marshal(a.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var saved savedLayout
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	st := saved.Tabs[0]
	if st.FocusID != browser.ID || st.ZoomID != browser.ID || st.Root.A.Kind != paneTerminal || st.Root.B.A.URL != "https://example.com" || st.Root.B.B.Kind != paneEmpty {
		t.Fatalf("mixed layout lost pane state: %+v", st)
	}
	legacy := `{"active":0,"tabs":[{"focus":"shell-1","root":{"sid":"shell-1","dir":"/tmp"}}]}`
	if err := json.Unmarshal([]byte(legacy), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Tabs[0].Root.Kind != "" || saved.Tabs[0].Root.SID != "shell-1" || saved.Tabs[0].Focus != "shell-1" {
		t.Fatalf("legacy terminal layout changed: %+v", saved.Tabs[0])
	}
}

func TestBrowserAddresses(t *testing.T) {
	for input, want := range map[string]string{
		"localhost:3000":      "http://localhost:3000",
		"127.0.0.1:5173/app":  "http://127.0.0.1:5173/app",
		"192.168.1.20:8080":   "http://192.168.1.20:8080",
		"github.com/egoist":   "https://github.com/egoist",
		"https://example.com": "https://example.com",
		"hello world":         "https://www.google.com/search?q=hello+world",
		"":                    "https://www.google.com/",
	} {
		if got := browserDestination(input); got != want {
			t.Errorf("%q => %q, want %q", input, got, want)
		}
	}
}

func TestBrowserSessionPersistenceAcrossMacOSVersions(t *testing.T) {
	for _, version := range []string{"13.0", "13.7.6", "", "unknown"} {
		if got := browserSessionForMacOS(version); got != mygo.DefaultSession() {
			t.Errorf("macOS %q should use the persistent default store", version)
		}
	}
	for _, version := range []string{"14.0", "15.7", "27.0.1"} {
		if got := browserSessionForMacOS(version); got != mygo.NewSession("larik-desktop-browser") {
			t.Errorf("macOS %q should use the named browser store", version)
		}
	}
}

func TestBrowserCleanupRemovesListeners(t *testing.T) {
	called := false
	b := &browserState{off: []func(){func() { called = true }}}
	p := &Pane{Kind: paneBrowser, Browser: b}
	p.closeView()
	if !called || p.Browser != nil || !p.Closed {
		t.Fatalf("browser was not cleaned up: %+v", p)
	}
}

func TestRestoreEmptyAndFailedBrowser(t *testing.T) {
	a := &App{prefs: defaultSettings(), newWebView: func(string) (*mygo.WebView, error) { return nil, errors.New("WKWebView unavailable") }}
	tab := &Tab{}
	empty := a.restoreNode(&savedNode{ID: 41, Kind: paneEmpty, Dir: "/tmp"}, nil, tab, nil)
	if empty == nil || empty.Pane.Kind != paneEmpty || empty.Pane.ID != 41 || empty.Pane.SID != "" {
		t.Fatalf("empty pane did not restore: %+v", empty)
	}
	browser := a.restoreNode(&savedNode{ID: 42, Kind: paneBrowser, Dir: "/tmp", URL: "https://example.com"}, nil, tab, nil)
	if browser == nil || browser.Pane.Kind != paneEmpty || !strings.Contains(browser.Pane.ChoiceError, "WKWebView unavailable") {
		t.Fatalf("failed browser did not become a chooser: %+v", browser)
	}
}

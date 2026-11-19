//go:build darwin && larik_e2e

package desktop

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"larik/internal/control"
	"larik/internal/database"
	"larik/internal/session"
)

// Native WebViews need App.Run on the main thread. Keep this test behind the
// larik_e2e tag so the normal unit suite works without a desktop session.
type discardWindowStates struct{}

func (discardWindowStates) LoadWindowStates() (map[string]mygo.WindowState, error) { return nil, nil }
func (discardWindowStates) SaveWindowStates(map[string]mygo.WindowState) error     { return nil }

func TestMain(m *testing.M) {
	code := make(chan int, 1)
	mygo.App.SetWindowStateStore(discardWindowStates{})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(func() {
		go func() {
			code <- m.Run()
			mygo.App.Quit()
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(<-code)
}

func nativeValue[T any](fn func() T) T {
	var value T
	mygo.RunOnMain(func() { value = fn() })
	return value
}

func waitMixed(t *testing.T, description string, ready func() bool) {
	t.Helper()
	for deadline := time.Now().Add(8 * time.Second); ; {
		if ready() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestMixedPaneWindowRestore(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "larik-mixed-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	t.Setenv("SHELL", "/bin/sh")
	oldBrowserSession := browserSession
	browserSession = mygo.NewEphemeralSession()
	t.Cleanup(func() { browserSession = oldBrowserSession })
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	registerFonts()

	dbPath := filepath.Join(dir, "fixture.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sample_items (id INTEGER PRIMARY KEY, label TEXT NOT NULL); INSERT INTO sample_items VALUES (1, 'one'), (2, 'two')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/attachment" {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="fixture.txt"`)
			_, _ = w.Write([]byte("LARIK_DOWNLOAD_OK"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><title>Mixed pane fixture</title><body>Browser restored. Browser restored.<input placeholder="Agent input"><button onclick="document.title='Agent clicked'">Agent action</button></body></html>`))
	}))
	defer page.Close()
	browserURL := page.URL + "/fixture"

	done := make(chan error, 1)
	go func() { done <- session.Serve() }()
	var client *session.Client
	waitMixed(t, "isolated session server", func() bool {
		if _, err := os.Stat(filepath.Join(dir, "server.sock")); err != nil {
			return false
		}
		client, err = session.Connect()
		return err == nil
	})
	t.Cleanup(func() {
		_ = client.Shutdown()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("session server: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("session server did not stop")
		}
	})

	downloadPath := filepath.Join(dir, "fixture.txt")
	a := &App{prefs: defaultSettings(), chooseDownload: func(string) (string, error) { return downloadPath, nil }}
	t.Cleanup(func() {
		nativeValue(func() struct{} {
			if a.win != nil {
				a.prefs.KeepSessions = false
				a.win.Close()
			}
			return struct{}{}
		})
	})
	if err := nativeValue(func() error { return a.open() }); err != nil {
		t.Fatal(err)
	}
	type initial struct {
		terminal *Pane
		browser  *Pane
		database *Pane
		tab      *Tab
		err      error
	}
	created := nativeValue(func() initial {
		p := a.tab().Focus
		a.choosePane(p, paneTerminal)
		if p.Kind != paneTerminal || p.Term == nil {
			return initial{err: fmt.Errorf("terminal chooser failed: %s", p.ChoiceError)}
		}
		a.split(false)
		b := a.tab().Focus
		if err := a.startBrowser(b, browserURL); err != nil {
			return initial{err: err}
		}
		a.split(true)
		d := a.tab().Focus
		a.startDatabase(d)
		if d.Database == nil || a.databaseStore == nil {
			return initial{err: fmt.Errorf("database chooser failed")}
		}
		if err := a.databaseStore.save(map[string]database.ConnectionConfig{
			"fixture": {Driver: "sqlite", Path: dbPath, AgentAccess: true},
		}); err != nil {
			return initial{err: err}
		}
		d.Database.profile = "fixture"
		d.Database.browserWidth = 35
		d.Database.showDetails = true
		a.loadDatabaseSchema(d)
		tab := a.tab()
		tab.Root.Ratio = 0.37
		tab.Root.B.Ratio = 0.61
		a.newTab(dir)
		a.selectTab(0)
		a.focus(b)
		a.toggleZoom()
		return initial{terminal: p, browser: b, database: d, tab: tab}
	})
	if created.err != nil {
		t.Fatal(created.err)
	}
	waitMixed(t, "browser page title", func() bool {
		return nativeValue(func() bool {
			return created.browser.Browser != nil && created.browser.Browser.Title == "Mixed pane fixture"
		})
	})
	if err := nativeValue(func() error { return created.browser.Browser.Web.Page().LoadURL(page.URL + "/attachment") }); err != nil {
		t.Fatal(err)
	}
	waitMixed(t, "browser download", func() bool {
		data, err := os.ReadFile(downloadPath)
		return err == nil && string(data) == "LARIK_DOWNLOAD_OK" && nativeValue(func() bool {
			return created.browser.Browser.DownloadPath == downloadPath
		})
	})
	nativeValue(func() struct{} {
		a.chooseDownload = func(string) (string, error) { return "", nil }
		return struct{}{}
	})
	if err := nativeValue(func() error { return created.browser.Browser.Web.Page().LoadURL(page.URL + "/attachment?cancel") }); err != nil {
		t.Fatal(err)
	}
	waitMixed(t, "canceled browser download", func() bool {
		return nativeValue(func() bool { return created.browser.Browser.DownloadStatus == "Download canceled" })
	})
	nativeValue(func() struct{} {
		a.chooseDownload = func(string) (string, error) { return "", errors.New("save panel unavailable") }
		return struct{}{}
	})
	if err := nativeValue(func() error { return created.browser.Browser.Web.Page().LoadURL(page.URL + "/attachment?failure") }); err != nil {
		t.Fatal(err)
	}
	waitMixed(t, "browser download save error", func() bool {
		return nativeValue(func() bool {
			return created.browser.Browser.DownloadError && strings.Contains(created.browser.Browser.DownloadStatus, "save panel unavailable")
		})
	})
	if err := nativeValue(func() error {
		a.bookmarkCurrent(created.browser)
		if !a.bookmarks.has(browserURL) {
			return fmt.Errorf("current browser page was not bookmarked: %s", a.bookmarkError)
		}
		a.openBookmarks(created.browser)
		if !a.bookmarksOpen {
			return fmt.Errorf("bookmarks dialog did not open")
		}
		a.bookmarksOpen = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := nativeValue(func() error {
		created.browser.Browser.Web.Focus()
		if nativeUIHasKeyboard(a.win) {
			return fmt.Errorf("browser page did not take the keyboard")
		}
		a.openFind()
		if !nativeUIHasKeyboard(a.win) {
			return fmt.Errorf("Find did not return the keyboard to the native UI")
		}
		created.browser.FindQuery = "Browser restored"
		a.browserFind(created.browser, false, false)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	waitMixed(t, "browser find results", func() bool {
		return nativeValue(func() bool {
			b := created.browser.Browser
			return b != nil && !b.FindBusy && b.FindResult.Matches == 2 && b.FindResult.Active == 1
		})
	})
	nativeValue(func() struct{} { a.findNext(); return struct{}{} })
	waitMixed(t, "next browser match", func() bool {
		return nativeValue(func() bool { return created.browser.Browser.FindResult.Active == 2 })
	})
	nativeValue(func() struct{} { a.findPrevious(); return struct{}{} })
	waitMixed(t, "previous browser match", func() bool {
		return nativeValue(func() bool { return created.browser.Browser.FindResult.Active == 1 })
	})
	nativeValue(func() struct{} { a.closeFind(created.browser); return struct{}{} })
	if nativeValue(func() bool {
		return created.browser.FindOpen || created.browser.FindQuery != "" || created.browser.Browser.FindResult.Matches != 0
	}) {
		t.Fatal("closing browser find retained query or match status")
	}
	if err := nativeValue(func() error {
		web := created.browser.Browser.Web
		web.Focus()
		a.browserCommand("address")
		if !nativeUIHasKeyboard(a.win) {
			return fmt.Errorf("Browser address did not return the keyboard to the native UI")
		}
		created.browser.Browser.FocusAddress = false
		web.Focus()
		a.openPalette()
		if !a.paletteOpen || !nativeUIHasKeyboard(a.win) {
			return fmt.Errorf("command palette did not return the keyboard to the native UI")
		}
		a.paletteOpen = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	waitMixed(t, "SQLite schema", func() bool {
		return nativeValue(func() bool { return created.database.Database != nil && len(created.database.Database.tables) > 0 })
	})
	if _, err := created.terminal.Stream.Write([]byte("printf 'MIXED_RESTORE_OK\\n'\n")); err != nil {
		t.Fatal(err)
	}
	waitMixed(t, "terminal output", func() bool { return strings.Contains(created.terminal.Term.Text(), "MIXED_RESTORE_OK") })
	originalSID := created.terminal.SID
	list, err := client.List()
	if err != nil || len(list) != 1 || list[0].ID != originalSID {
		t.Fatalf("one live shell expected: %+v, %v", list, err)
	}
	originalPID := list[0].PID

	nativeValue(func() struct{} { a.win.Close(); return struct{}{} })
	waitMixed(t, "first window close", func() bool { return nativeValue(func() bool { return a.win == nil }) })
	if !created.browser.Closed || !created.database.Closed || created.database.Database != nil {
		t.Fatal("closing the window left browser or database views alive")
	}
	list, err = client.List()
	if err != nil || len(list) != 1 || list[0].ID != originalSID || list[0].PID != originalPID {
		t.Fatalf("Keep Sessions changed the shell: %+v, %v", list, err)
	}
	nativeValue(func() struct{} { a.databaseStore = nil; return struct{}{} })
	if err := nativeValue(func() error { return a.open() }); err != nil {
		t.Fatal(err)
	}
	type restoredState struct {
		terminal, browser, database, chooser *Pane
		focus, zoom                          *Pane
		active, tabs                         int
		ratio, nestedRatio                   float32
	}
	restored := nativeValue(func() restoredState {
		if len(a.tabs) != 2 || a.tabs[0].Root == nil || a.tabs[0].Root.A == nil || a.tabs[0].Root.B == nil || a.tabs[0].Root.B.A == nil || a.tabs[0].Root.B.B == nil {
			return restoredState{tabs: len(a.tabs)}
		}
		tab := a.tabs[0]
		return restoredState{
			terminal: tab.Root.A.Pane, browser: tab.Root.B.A.Pane, database: tab.Root.B.B.Pane,
			chooser: a.tabs[1].Focus, focus: tab.Focus, zoom: tab.Zoom,
			active: a.active, tabs: len(a.tabs), ratio: tab.Root.Ratio, nestedRatio: tab.Root.B.Ratio,
		}
	})
	if restored.tabs != 2 || restored.terminal == nil || restored.browser == nil || restored.database == nil || restored.chooser == nil {
		t.Fatalf("mixed layout tree did not restore: %+v", restored)
	}
	if restored.active != 0 || restored.ratio != 0.37 || restored.nestedRatio != 0.61 ||
		restored.focus != restored.browser || restored.zoom != restored.browser ||
		restored.terminal.Kind != paneTerminal || restored.terminal.SID != originalSID || restored.terminal.Term == nil ||
		restored.browser.Kind != paneBrowser || restored.browser.Browser == nil || restored.browser.Browser.URL != browserURL || restored.browser.Browser.Web == nil ||
		restored.database.Kind != paneDatabase || restored.database.Database == nil || restored.database.Database.profile != "fixture" ||
		restored.database.Database.browserWidth != 35 || !restored.database.Database.showDetails || restored.chooser.Kind != paneEmpty {
		t.Fatalf("mixed layout did not restore: %+v", restored)
	}
	waitMixed(t, "restored browser page", func() bool {
		return nativeValue(func() bool {
			return restored.browser.Browser != nil && restored.browser.Browser.Title == "Mixed pane fixture"
		})
	})
	waitMixed(t, "restored database schema", func() bool {
		return nativeValue(func() bool { return restored.database.Database != nil && len(restored.database.Database.tables) > 0 })
	})
	profiles := a.controlDatabase(control.Request{Op: "database.profiles"})
	if profiles.Error != "" || len(profiles.Value.([]database.Profile)) != 1 {
		t.Fatalf("agent database profiles: %+v", profiles)
	}
	query := a.controlDatabase(control.Request{Op: "database.query", Profile: "fixture", SQL: "SELECT label FROM sample_items", Limit: 5})
	if query.Error != "" || len(query.Value.(database.Result).Rows) != 2 {
		t.Fatalf("agent database query: %+v", query)
	}
	browserID := restored.browser.ID
	snapshot := a.controlBrowser(control.Request{Op: "browser.snapshot", Pane: browserID})
	if snapshot.Error != "" {
		t.Fatalf("agent browser snapshot: %+v", snapshot)
	}
	outline, ok := snapshot.Value.(map[string]any)
	if !ok {
		t.Fatalf("browser snapshot type: %T", snapshot.Value)
	}
	refFor := func(label string) string {
		for _, line := range strings.Split(fmt.Sprint(outline["text"]), "\n") {
			if strings.Contains(line, label) {
				if match := regexp.MustCompile(`ref=(e[0-9]+)`).FindStringSubmatch(line); len(match) == 2 {
					return match[1]
				}
			}
		}
		return ""
	}
	inputRef, buttonRef := refFor("Agent input"), refFor("Agent action")
	if inputRef == "" || buttonRef == "" {
		t.Fatalf("interactive refs missing: %+v", outline)
	}
	if res := a.controlBrowser(control.Request{Op: "browser.type", Pane: browserID, Ref: inputRef, Text: "Ada"}); res.Error != "" {
		t.Fatalf("agent browser type: %+v", res)
	}
	if res := a.controlBrowser(control.Request{Op: "browser.eval", Pane: browserID, Text: "document.querySelector('input').value"}); res.Error != "" || res.Value != "Ada" {
		t.Fatalf("typed browser value: %+v", res)
	}
	if res := a.controlBrowser(control.Request{Op: "browser.click", Pane: browserID, Ref: buttonRef}); res.Error != "" {
		t.Fatalf("agent browser click: %+v", res)
	}
	if res := a.controlBrowser(control.Request{Op: "browser.eval", Pane: browserID, Text: "document.title"}); res.Error != "" || res.Value != "Agent clicked" {
		t.Fatalf("clicked browser title: %+v", res)
	}
	if res := a.controlBrowser(control.Request{Op: "browser.screenshot", Pane: browserID}); res.Error != "" || len(res.PNG) < 8 || string(res.PNG[:4]) != "\x89PNG" {
		t.Fatalf("agent browser screenshot: error=%q bytes=%d", res.Error, len(res.PNG))
	}
	if res := a.controlBrowser(control.Request{Op: "browser.navigate", Pane: browserID, URL: browserURL}); res.Error != "" {
		t.Fatalf("agent browser navigate: %+v", res)
	}
	if res := a.controlBrowser(control.Request{Op: "browser.click", Pane: browserID, Ref: buttonRef}); !strings.Contains(res.Error, "fresh snapshot") {
		t.Fatalf("stale browser ref: %+v", res)
	}
	waitMixed(t, "restored terminal screen", func() bool { return strings.Contains(restored.terminal.Term.Text(), "MIXED_RESTORE_OK") })
	list, err = client.List()
	if err != nil || len(list) != 1 || list[0].PID != originalPID {
		t.Fatalf("reopened window replaced the shell: %+v, %v", list, err)
	}
	nativeValue(func() struct{} { a.prefs.KeepSessions = false; a.win.Close(); return struct{}{} })
	waitMixed(t, "second window close", func() bool { return nativeValue(func() bool { return a.win == nil }) })
	list, err = client.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("Keep Sessions off left shells running: %+v, %v", list, err)
	}
	data, err := client.Layout()
	var saved savedLayout
	if err != nil || json.Unmarshal(data, &saved) != nil || len(saved.Tabs) != 0 {
		t.Fatalf("Keep Sessions off retained layout: %s, %v", data, err)
	}
	bookmarks, err := loadBookmarkStore()
	if err != nil || !bookmarks.has(browserURL) {
		t.Fatalf("clearing sessions removed bookmarks: %+v, %v", bookmarks, err)
	}
}

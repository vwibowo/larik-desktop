//go:build darwin && larik_e2e

package desktop

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><title>Mixed pane fixture</title><body>Browser restored. Browser restored.</body></html>"))
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

	a := &App{prefs: defaultSettings()}
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
			"fixture": {Driver: "sqlite", Path: dbPath},
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
}

package desktop

import (
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"larik/internal/control"
	"larik/internal/session"
)

// Run starts the native terminal workspace. The window may close while its
// shell sessions continue in the companion server.
func Run() error {
	if err := terminal.Load(); err != nil {
		return err
	}
	registerFonts()
	a := &App{prefs: readSettings()}
	var err error
	a.grants, err = control.LoadGrants()
	if err != nil {
		return err
	}
	a.prefs.applyAppearance()
	var startErr error
	mygo.App.WhenReady(func() {
		startErr = a.open()
		if startErr != nil {
			mygo.App.Quit()
			return
		}
		startErr = a.startControl()
		if startErr != nil {
			mygo.App.Quit()
		}
	})
	mygo.App.OnActivate(func(visible bool) {
		if !visible && a.win == nil {
			if err := a.open(); err != nil {
				startErr = err
				mygo.App.Quit()
			}
		}
	})
	// Keep the macOS app in the Dock so activation can recreate its window.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnDidResignActive(func() {
		if a.win != nil {
			a.win.SetActiveAppearance(false)
		}
	})
	mygo.App.OnDidBecomeActive(func() { a.keepTerminalMaterialActive() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.stopControl()
		if a.win != nil {
			a.persistWorkspace()
		}
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return startErr
}

func (a *App) open() error {
	if a.bookmarks == nil && a.bookmarkError == "" {
		var err error
		a.bookmarks, err = loadBookmarkStore()
		if err != nil {
			a.bookmarkError = err.Error()
		}
	}
	client, err := session.Connect()
	if err != nil {
		return err
	}
	client, ready, err := client.UpgradeIfIdle()
	if err != nil {
		return err
	}
	pid, version, caps, err := client.HelloCapabilities()
	if err != nil {
		return err
	}
	if version != session.ProtocolVersion {
		return fmt.Errorf("session server version %d is incompatible with this app (version %d); end old sessions before opening", version, session.ProtocolVersion)
	}
	a.client, a.serverPID, a.shellOverride = client, pid, ready
	a.terminalControl = slices.Contains(caps, session.TerminalControlCapability)
	a.tabs, a.active, a.closing, a.err = nil, 0, false, ""
	a.bookmarksOpen, a.bookmarksPane, a.bookmarksQuery = false, nil, ""
	a.pollDone = make(chan struct{})
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: "larik-desktop", Width: 1000, Height: 620, MinWidth: 560, MinHeight: 340,
		StateKey: "main", TitleBarStyle: mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 16, Y: 15},
		Transparent:          true,
		Vibrancy:             a.vibrancy(),
		Content:              ui.View(a.view),
	})
	a.win = win
	a.saveDatabaseCSV = databaseCSVDialog(win, a)
	a.keepTerminalMaterialActive()
	mygo.App.SetMenu(a.menu())
	if !a.restore() {
		home, _ := os.UserHomeDir()
		if err := a.newTab(home); err != nil {
			win.Close()
			return err
		}
	}
	a.changed()
	go a.poll(win, a.pollDone)
	win.OnClosed(func() {
		a.persistWorkspace()
		a.closing = true
		close(a.pollDone)
		for _, t := range a.tabs {
			for _, p := range t.panes() {
				p.closeView()
			}
		}
		a.win = nil
		a.saveDatabaseCSV = nil
		a.tabs = nil
	})
	return nil
}

func (a *App) openTerminal() {
	if a.win != nil {
		a.win.Focus()
		return
	}
	if err := a.open(); err != nil {
		a.settingsErr, a.settingsField = err.Error(), "window"
	}
}

func (a *App) persistWorkspace() {
	if a.client == nil {
		return
	}
	if a.prefs.KeepSessions {
		a.saveNow()
		return
	}
	for _, t := range a.tabs {
		for _, p := range t.panes() {
			if p.SID != "" {
				_ = a.client.Kill(p.SID)
			}
		}
	}
	if err := a.client.Save([]byte(`{"active":0,"tabs":[]}`)); err != nil {
		a.err = err.Error()
	}
}

func (a *App) poll(win *mygo.Window, done <-chan struct{}) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		infos, err := a.client.List()
		if err != nil {
			win.Update(func() {
				if a.win == win {
					a.err = "Session server disconnected: " + err.Error()
				}
			})
			return
		}
		win.Update(func() {
			if a.win == win {
				a.refresh(infos)
				if len(infos) == 0 && (!a.shellOverride || !a.terminalControl) {
					next, ready, err := a.client.UpgradeIfIdle()
					if err != nil {
						a.err = "Session server upgrade failed: " + err.Error()
					} else {
						a.client, a.shellOverride = next, ready
						pid, _, caps, _ := next.HelloCapabilities()
						a.serverPID = pid
						a.terminalControl = slices.Contains(caps, session.TerminalControlCapability)
						if ready && a.terminalControl {
							if a.settingsWin != nil {
								a.settingsWin.Invalidate()
							}
						}
					}
				}
			}
		})
	}
}

// EndSessions stops every shell in the background server.
func EndSessions() error {
	c, err := session.Connect()
	if err != nil {
		return err
	}
	return c.Shutdown()
}

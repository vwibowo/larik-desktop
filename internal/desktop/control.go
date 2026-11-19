package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"larik/internal/control"
	"larik/internal/database"
	"larik/internal/session"
)

func (a *App) startControl() error {
	ln, err := control.Listen()
	if err != nil {
		return err
	}
	a.controlListener = ln
	go control.Serve(ln, a.controlRequest)
	return nil
}

func (a *App) stopControl() {
	if a.controlListener != nil {
		_ = a.controlListener.Close()
		_ = os.Remove(control.SocketPath())
		a.controlListener = nil
	}
}

func (a *App) authorize(req control.Request) error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	agent := control.Agent{Name: req.Agent, Path: req.Path}
	if agent.Name == "" || agent.Path == "" {
		return errors.New("agent identity is missing")
	}
	if a.grants.Has(agent) {
		return nil
	}
	parent := a.win
	if parent == nil {
		parent = a.settingsWin
	}
	if parent == nil {
		return errors.New("open Larik Desktop to approve this agent")
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent: parent, Type: mygo.MessageQuestion, Title: "Allow agent control?",
		Message: fmt.Sprintf("Allow %s to control Larik Desktop?", agent.Name),
		Detail:  agent.Path + "\n\nThis grants pane, terminal, browser, and enabled database access. Revoke it in Settings.",
		Buttons: []string{"Deny", "Allow"}, DefaultButton: 0, CancelButton: 0,
	})
	if err != nil {
		return err
	}
	if res.Button != 1 {
		return errors.New("agent access denied")
	}
	return a.grants.Set(agent, true)
}

func (a *App) controlRequest(req control.Request) control.Response {
	if err := a.authorize(req); err != nil {
		return control.Response{Error: err.Error()}
	}
	if a.win == nil && (req.Op == "list" || req.Op == "terminal.read" || req.Op == "terminal.send") {
		res, err := ClosedControl(req)
		if err != nil {
			return control.Response{Error: err.Error()}
		}
		return res
	}
	if req.Op == "terminal.read" || req.Op == "terminal.send" {
		return a.controlTerminal(req)
	}
	if strings.HasPrefix(req.Op, "database.") && req.Op != "database.open" {
		return a.controlDatabase(req)
	}
	if strings.HasPrefix(req.Op, "browser.") {
		return a.controlBrowser(req)
	}
	return a.onControlMain(req)
}

func (a *App) onControlMain(req control.Request) control.Response {
	win := a.win
	if win == nil {
		return control.Response{Error: "Larik Desktop window is closed; run `larik-desktop control launch`"}
	}
	result := make(chan control.Response, 1)
	win.Update(func() { result <- a.controlUI(req) })
	select {
	case r := <-result:
		return r
	case <-time.After(10 * time.Second):
		return control.Response{Error: "Larik Desktop window did not respond"}
	}
}

func (a *App) paneByID(id int) *Pane {
	for _, t := range a.tabs {
		for _, p := range t.panes() {
			if p.ID == id {
				return p
			}
		}
	}
	return nil
}

func (a *App) tabByID(id int) *Tab {
	for _, t := range a.tabs {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (a *App) paneInfo(p *Pane) control.PaneInfo {
	title, _ := p.label()
	i := control.PaneInfo{ID: p.ID, Kind: p.Kind, Title: title, Dir: p.Dir, Session: p.SID, Focused: p.Tab == a.tab() && p.Tab.Focus == p}
	if p.Browser != nil {
		i.URL = p.Browser.URL
	}
	if p.Database != nil {
		i.Profile = p.Database.profile
	}
	return i
}

func (a *App) tabInfo(t *Tab) control.TabInfo {
	i := control.TabInfo{ID: t.ID, Name: t.Name, Focused: t == a.tab(), Panes: []control.PaneInfo{}}
	for _, p := range t.panes() {
		i.Panes = append(i.Panes, a.paneInfo(p))
	}
	return i
}

func (a *App) controlUI(req control.Request) control.Response {
	switch req.Op {
	case "list":
		out := control.Response{Tabs: []control.TabInfo{}}
		for _, t := range a.tabs {
			out.Tabs = append(out.Tabs, a.tabInfo(t))
		}
		return out
	case "open":
		var p *Pane
		if req.Split != "" {
			if req.Split != "right" && req.Split != "down" {
				return control.Response{Error: "split must be right or down"}
			}
			p = a.paneByID(req.Pane)
			if p == nil {
				return control.Response{Error: "target pane not found"}
			}
			for i, t := range a.tabs {
				if t == p.Tab {
					a.selectTab(i)
					break
				}
			}
			a.focus(p)
			a.split(req.Split == "down")
			p = a.tab().Focus
		} else {
			if req.Tab != 0 {
				return control.Response{Error: "open a split with --pane, or create a new tab without --tab"}
			}
			dir := req.Dir
			if dir == "" {
				dir = a.newTabDir()
			}
			if err := a.newTab(dir); err != nil {
				return control.Response{Error: err.Error()}
			}
			p = a.tab().Focus
		}
		if err := a.controlChoose(p, req); err != nil {
			return control.Response{Pane: ptr(a.paneInfo(p)), Error: err.Error()}
		}
		return control.Response{Tab: ptr(a.tabInfo(p.Tab)), Pane: ptr(a.paneInfo(p))}
	case "choose":
		p := a.paneByID(req.Pane)
		if p == nil {
			return control.Response{Error: "pane not found"}
		}
		if err := a.controlChoose(p, req); err != nil {
			return control.Response{Error: err.Error()}
		}
		return control.Response{Pane: ptr(a.paneInfo(p))}
	case "focus":
		p := a.paneByID(req.Pane)
		if p == nil {
			return control.Response{Error: "pane not found"}
		}
		for i, t := range a.tabs {
			if t == p.Tab {
				a.selectTab(i)
				break
			}
		}
		a.focus(p)
		return control.Response{Pane: ptr(a.paneInfo(p))}
	case "close":
		p := a.paneByID(req.Pane)
		if p == nil {
			return control.Response{Error: "pane not found"}
		}
		a.closePane(p)
		return control.Response{}
	}
	return control.Response{Error: "unknown control operation"}
}

func ptr[T any](v T) *T { return &v }

func (a *App) controlChoose(p *Pane, req control.Request) error {
	if p.Kind != paneEmpty {
		return errors.New("pane is not empty")
	}
	switch req.Kind {
	case "", paneEmpty:
		return nil
	case paneTerminal:
		_, err := a.startTerminal(p)
		if err != nil {
			return err
		}
	case paneBrowser:
		u := req.URL
		if u != "" {
			u = browserDestination(u)
		}
		if err := a.startBrowser(p, u); err != nil {
			return err
		}
	case paneDatabase:
		if req.Profile != "" {
			if a.databaseStore == nil {
				a.databaseStore = loadDatabaseStore()
			}
			if a.databaseStore == nil || a.databaseStore.manager == nil {
				return errors.New("database manager is unavailable")
			}
			if !profileAllowed(a.databaseStore.manager, req.Profile) {
				return errors.New("database profile not available to agents")
			}
		}
		a.startDatabase(p)
		if req.Profile != "" {
			p.Database.profile = req.Profile
			a.loadDatabaseSchema(p)
		}
	default:
		return errors.New("kind must be empty, terminal, browser, or database")
	}
	a.changed()
	return nil
}

func profileAllowed(m *database.Manager, name string) bool {
	for _, p := range m.List(true) {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (a *App) controlTerminal(req control.Request) control.Response {
	var sid string
	result := make(chan struct{}, 1)
	if win := a.win; win != nil {
		win.Update(func() {
			if p := a.paneByID(req.Pane); p != nil && p.Kind == paneTerminal {
				sid = p.SID
			}
			result <- struct{}{}
		})
		select {
		case <-result:
		case <-time.After(10 * time.Second):
			return control.Response{Error: "window did not respond"}
		}
	} else {
		// Window closed but process remains alive with Settings visible.
		for _, t := range a.tabs {
			for _, p := range t.panes() {
				if p.ID == req.Pane && p.Kind == paneTerminal {
					sid = p.SID
				}
			}
		}
	}
	if sid == "" {
		return control.Response{Error: "terminal pane not found"}
	}
	capable, err := a.client.TerminalControl()
	if err != nil {
		return control.Response{Error: err.Error()}
	}
	if !capable {
		return control.Response{Error: "older session server does not support terminal control; existing shells remain active"}
	}
	if req.Op == "terminal.read" {
		text, err := a.client.Read(sid, req.Lines)
		if err != nil {
			return control.Response{Error: err.Error()}
		}
		return control.Response{Text: text}
	}
	input := req.Text
	if req.Enter {
		input += "\r"
	}
	if err := a.client.Write(sid, input); err != nil {
		return control.Response{Error: err.Error()}
	}
	return control.Response{}
}

func (a *App) controlDatabase(req control.Request) control.Response {
	if a.win == nil {
		return control.Response{Error: "database commands need an open Larik Desktop window"}
	}
	var store *databaseStore
	ch := make(chan struct{}, 1)
	if win := a.win; win != nil {
		win.Update(func() {
			if a.databaseStore == nil {
				a.databaseStore = loadDatabaseStore()
			}
			store = a.databaseStore
			ch <- struct{}{}
		})
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			return control.Response{Error: "window did not respond"}
		}
	}
	if store == nil || store.manager == nil {
		return control.Response{Error: "database manager is unavailable"}
	}
	m := store.manager
	if req.Op == "database.profiles" {
		return control.Response{Value: m.List(true)}
	}
	if !profileAllowed(m, req.Profile) {
		return control.Response{Error: "database profile not available to agents"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	var value any
	var err error
	switch req.Op {
	case "database.schema":
		value, err = m.Schema(ctx, req.Profile, true)
	case "database.rows":
		value, err = m.Rows(ctx, req.Profile, req.Schema, req.Table, req.Offset, req.Limit, true)
	case "database.query":
		value, err = m.Query(ctx, req.Profile, req.SQL, req.Limit, true)
	default:
		return control.Response{Error: "unknown database operation"}
	}
	if err != nil {
		return control.Response{Error: err.Error()}
	}
	return control.Response{Value: value}
}

// ClosedControl serves saved layout listing and existing terminal I/O
// when the native window process has quit but its detached server remains.
func ClosedControl(req control.Request) (control.Response, error) {
	grants, err := control.LoadGrants()
	if err != nil {
		return control.Response{}, err
	}
	if !grants.Has(control.Agent{Name: req.Agent, Path: req.Path}) {
		return control.Response{}, errors.New("open Larik Desktop to approve this agent")
	}
	if req.Op != "list" && req.Op != "terminal.read" && req.Op != "terminal.send" {
		return control.Response{}, errors.New("this command needs an open Larik Desktop window")
	}
	client, err := session.Existing()
	if err != nil {
		return control.Response{}, err
	}
	layout, err := client.Layout()
	if err != nil {
		return control.Response{}, err
	}
	var saved savedLayout
	if err := json.Unmarshal(layout, &saved); err != nil {
		return control.Response{}, err
	}
	infos, err := client.List()
	if err != nil {
		return control.Response{}, err
	}
	known := make(map[string]session.Info, len(infos))
	for _, info := range infos {
		known[info.ID] = info
	}
	var tabs []control.TabInfo
	var target string
	for i, t := range saved.Tabs {
		item := control.TabInfo{ID: t.ID, Name: t.Name, Focused: i == saved.Active, Panes: []control.PaneInfo{}}
		var walk func(*savedNode)
		walk = func(n *savedNode) {
			if n == nil {
				return
			}
			if n.A != nil || n.B != nil {
				walk(n.A)
				walk(n.B)
				return
			}
			kind := n.Kind
			if kind == "" {
				kind = paneTerminal
			}
			p := control.PaneInfo{ID: n.ID, Kind: kind, Dir: n.Dir, Session: n.SID, URL: n.URL, Profile: n.DatabaseProfile, Focused: n.ID == t.FocusID}
			if info, ok := known[n.SID]; ok {
				p.Title, p.Dir = info.Program, info.Dir
			}
			if p.Title == "" {
				p.Title = kind
			}
			item.Panes = append(item.Panes, p)
			if n.ID == req.Pane && kind == paneTerminal {
				target = n.SID
			}
		}
		walk(t.Root)
		tabs = append(tabs, item)
	}
	if req.Op == "list" {
		return control.Response{Version: control.Version, Tabs: tabs}, nil
	}
	if target == "" {
		return control.Response{}, errors.New("terminal pane not found")
	}
	capable, err := client.TerminalControl()
	if err != nil {
		return control.Response{}, err
	}
	if !capable {
		return control.Response{}, errors.New("older session server does not support terminal control; reopen Larik Desktop after its sessions end")
	}
	if req.Op == "terminal.read" {
		text, err := client.Read(target, req.Lines)
		return control.Response{Version: control.Version, Text: text}, err
	}
	input := req.Text
	if req.Enter {
		input += "\r"
	}
	return control.Response{Version: control.Version}, client.Write(target, input)
}

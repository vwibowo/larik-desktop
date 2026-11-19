package desktop

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"larik/internal/control"
	"larik/internal/session"
)

type Pane struct {
	ID          int
	Kind        string
	Dir         string
	SID         string
	Node        *Node
	Tab         *Tab
	Term        *terminal.Terminal
	Browser     *browserState
	Database    *databaseState
	ChoiceError string
	Stream      *session.Stream
	Info        session.Info
	Title       string
	Attention   bool
	Closed      bool
	Bounds      ui.Rect
	FindOpen    bool
	FindFocus   bool
	FindQuery   string
	FindStatus  terminal.SearchStatus
}

const (
	paneEmpty    = "empty"
	paneTerminal = "terminal"
	paneBrowser  = "browser"
	paneDatabase = "database"
)

type Node struct {
	ID       int
	Parent   *Node
	Pane     *Pane
	Vertical bool
	Ratio    float32
	A, B     *Node
}

type Tab struct {
	ID    int
	Name  string
	Root  *Node
	Focus *Pane
	Zoom  *Pane
}

type App struct {
	win                *mygo.Window
	controlListener    net.Listener
	grants             *control.Grants
	authMu             sync.Mutex
	newWebView         func(string) (*mygo.WebView, error)
	chooseDownload     func(string) (string, error)
	chooseSkillsFolder func() (string, error)
	client             *session.Client
	databaseStore      *databaseStore
	bookmarks          *bookmarkStore
	bookmarkError      string
	bookmarksOpen      bool
	bookmarksPane      *Pane
	bookmarksQuery     string
	saveDatabaseCSV    func(string, []byte, func(string, error))
	copyText           func(string)
	tabs               []*Tab
	active             int
	nextID             int
	serverPID          int
	shellOverride      bool
	terminalControl    bool
	prefs              settings
	settingsWin        *mygo.Window
	settingsTab        int
	settingsErr        string
	settingsField      string
	themePickerOpen    bool
	themePickerDark    bool
	themeSearch        string
	themeNames         []string
	shellDraft         string
	folderDraft        string
	err                string
	closing            bool
	focusReq           *Pane
	pollDone           chan struct{}
	saveDue            bool
	saveAt             time.Time
	lastSnapshot       string
	lastTitle          string
	paletteOpen        bool
	paletteQuery       string
	paletteIndex       int
	hostOpen           bool
	renaming           *Tab
	renameText         string
	services           ui.Services
	pending            []func()
}

func (a *App) id() int { a.nextID++; return a.nextID }

func (a *App) tab() *Tab {
	if a.active < 0 || a.active >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.active]
}

func (n *Node) panes() []*Pane {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		return []*Pane{n.Pane}
	}
	return append(n.A.panes(), n.B.panes()...)
}
func (t *Tab) panes() []*Pane { return t.Root.panes() }

func (a *App) newPane(dir string) (*Pane, error) {
	return a.startTerminal(&Pane{ID: a.id(), Kind: paneEmpty, Dir: dir})
}

func (a *App) emptyPane(dir string) *Pane {
	return &Pane{ID: a.id(), Kind: paneEmpty, Dir: dir}
}

func (a *App) startTerminal(p *Pane) (*Pane, error) {
	if !a.shellOverride || !a.terminalControl {
		if next, ready, err := a.client.UpgradeIfIdle(); err == nil {
			a.client, a.shellOverride = next, ready
			pid, _, caps, _ := next.HelloCapabilities()
			a.serverPID = pid
			a.terminalControl = slices.Contains(caps, session.TerminalControlCapability)
		}
	}
	shell := ""
	if a.shellOverride {
		shell = a.prefs.Shell
	}
	info, err := a.client.CreateWithShell(p.Dir, shell)
	if err != nil {
		return nil, err
	}
	p, err = a.attachInto(info, p)
	if err != nil {
		_ = a.client.Kill(info.ID)
		return nil, err
	}
	p.ChoiceError = ""
	return p, nil
}

func (a *App) choosePane(p *Pane, kind string) {
	if p == nil || p.Kind != paneEmpty || p.Closed {
		return
	}
	var err error
	switch kind {
	case paneTerminal:
		_, err = a.startTerminal(p)
		if err == nil {
			a.focusReq = p
		}
	case paneBrowser:
		err = a.startBrowser(p, "")
	case paneDatabase:
		a.startDatabase(p)
	default:
		return
	}
	if err != nil {
		p.ChoiceError = err.Error()
	} else {
		p.ChoiceError = ""
	}
	a.changed()
}

func (a *App) attach(info session.Info) (*Pane, error) {
	return a.attachInto(info, nil)
}

func (a *App) attachInto(info session.Info, p *Pane) (*Pane, error) {
	stream, err := a.client.Attach(info.ID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		p = &Pane{ID: a.id()}
	}
	p.Kind, p.Dir, p.SID, p.Info, p.Stream = paneTerminal, info.Dir, info.ID, info, stream
	lightTheme, darkTheme := a.prefs.viewThemes()
	term, err := terminal.New(terminal.Options{
		Conn: stream, Font: terminalFont(a.prefs.FontSize),
		Theme: lightTheme, DarkTheme: darkTheme, Transparent: true,
		Cursor: a.cursorStyle(), OptionAsAlt: a.prefs.OptionAsAlt,
		CopyOnSelect: a.copyOnSelect(),
		OnSearch: func(status terminal.SearchStatus) {
			if p.FindStatus != status {
				p.FindStatus = status
				if p.FindOpen && a.win != nil {
					a.win.Invalidate()
				}
			}
		},
		OnTitle: func(title string) {
			if a.win != nil {
				a.win.Update(func() {
					if !p.Closed {
						p.Title = title
					}
				})
			}
		},
		OnBell: func() {
			if a.win != nil {
				a.win.Update(func() {
					if !p.Closed && (a.tab() != p.Tab || p.Tab.Focus != p || !a.win.IsFocused()) {
						p.Attention = true
						if a.prefs.NotifyBell {
							mygo.NewNotification(mygo.NotificationOptions{Title: "Terminal bell", Body: p.Info.Dir}).Show()
						}
					}
				})
			}
		},
		OnExit: func(int) {
			if a.win != nil {
				a.win.Update(func() {
					if !p.Closed && !a.closing {
						a.closePane(p)
					}
				})
			}
		},
	})
	if err != nil {
		stream.Close()
		p.Kind, p.SID, p.Stream = paneEmpty, "", nil
		return nil, err
	}
	p.Term = term
	return p, nil
}

func (a *App) newTab(dir string) error {
	p := a.emptyPane(dir)
	t := &Tab{ID: a.id(), Focus: p}
	n := &Node{ID: a.id(), Pane: p}
	p.Tab, p.Node, t.Root = t, n, n
	a.tabs = append(a.tabs, t)
	a.active = len(a.tabs) - 1
	a.focusReq = p
	a.changed()
	return nil
}

func (a *App) split(vertical bool) {
	t := a.tab()
	if t == nil || t.Focus == nil {
		return
	}
	old := t.Focus
	dir := old.Dir
	if dir == "" {
		dir = a.currentDir()
	}
	p := a.emptyPane(dir)
	p.Tab = t
	leaf := old.Node
	parent := &Node{ID: a.id(), Parent: leaf.Parent, Vertical: vertical, Ratio: 0.5}
	newLeaf := &Node{ID: a.id(), Parent: parent, Pane: p}
	p.Node = newLeaf
	parent.A, parent.B = leaf, newLeaf
	if leaf.Parent == nil {
		t.Root = parent
	} else if leaf.Parent.A == leaf {
		leaf.Parent.A = parent
	} else {
		leaf.Parent.B = parent
	}
	leaf.Parent = parent
	t.Focus = p
	a.focusReq = p
	a.changed()
}

func (a *App) closePane(p *Pane) {
	if p == nil || p.Closed {
		return
	}
	p.closeView()
	if p.SID != "" {
		_ = a.client.Kill(p.SID)
	}
	t := p.Tab
	if t.Zoom == p {
		t.Zoom = nil
	}
	n := p.Node
	if n.Parent == nil {
		a.removeTab(t)
		return
	}
	parent := n.Parent
	sibling := parent.A
	if sibling == n {
		sibling = parent.B
	}
	sibling.Parent = parent.Parent
	if parent.Parent == nil {
		t.Root = sibling
	} else if parent.Parent.A == parent {
		parent.Parent.A = sibling
	} else {
		parent.Parent.B = sibling
	}
	if t.Focus == p {
		t.Focus = sibling.panes()[0]
		a.focusReq = t.Focus
	}
	a.changed()
}

func (a *App) removeTab(t *Tab) {
	i := slices.Index(a.tabs, t)
	if i < 0 {
		return
	}
	a.tabs = slices.Delete(a.tabs, i, i+1)
	if len(a.tabs) == 0 {
		a.active = 0
		a.changed()
		if a.win != nil {
			a.win.Close()
		}
		return
	}
	if a.active >= len(a.tabs) {
		a.active = len(a.tabs) - 1
	} else if i < a.active {
		a.active--
	}
	a.focusReq = a.tab().Focus
	a.changed()
}

func (a *App) closeTab(t *Tab) {
	if t == nil {
		return
	}
	for _, p := range t.panes() {
		p.closeView()
		if p.SID != "" {
			_ = a.client.Kill(p.SID)
		}
	}
	a.removeTab(t)
}

func (p *Pane) closeView() {
	p.Closed = true
	if p.Term != nil {
		p.Term.Close()
		p.Term = nil
	}
	if p.Stream != nil {
		_ = p.Stream.Close()
		p.Stream = nil
	}
	if p.Browser != nil {
		p.Browser.close()
		p.Browser = nil
	}
	if p.Database != nil {
		p.Database.close()
		p.Database = nil
	}
}

func (a *App) selectTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	a.active = i
	a.focusReq = a.tabs[i].Focus
	if a.focusReq != nil {
		a.focusReq.Attention = false
	}
	a.changed()
}

func (a *App) moveTab(t *Tab, to int) {
	from := slices.Index(a.tabs, t)
	if from < 0 || to < 0 || to >= len(a.tabs) || to == from {
		return
	}
	active := a.tab()
	a.tabs = slices.Delete(a.tabs, from, from+1)
	a.tabs = slices.Insert(a.tabs, to, t)
	a.active = slices.Index(a.tabs, active)
	a.changed()
}

func (a *App) swapPanes(first, second *Pane) {
	if first == nil || second == nil || first == second || first.Tab != second.Tab || first.Node == nil || second.Node == nil {
		return
	}
	first.Node.Pane, second.Node.Pane = second, first
	first.Node, second.Node = second.Node, first.Node
	a.focus(first)
}

func (a *App) focus(p *Pane) {
	if p == nil || p.Tab == nil {
		return
	}
	p.Tab.Focus = p
	p.Attention = false
	a.focusReq = p
	a.changed()
}

func (a *App) toggleZoom() {
	if t := a.tab(); t != nil {
		if t.Zoom == t.Focus {
			t.Zoom = nil
		} else {
			t.Zoom = t.Focus
		}
		a.focusReq = t.Focus
		a.changed()
	}
}

func (a *App) equalize() {
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || n.Pane != nil {
			return
		}
		n.Ratio = 0.5
		walk(n.A)
		walk(n.B)
	}
	if t := a.tab(); t != nil {
		walk(t.Root)
		a.changed()
	}
}

func (a *App) moveFocus(dx, dy float32) {
	t := a.tab()
	if t == nil || t.Focus == nil {
		return
	}
	from := t.Focus.Bounds
	cx, cy := from.X+from.W/2, from.Y+from.H/2
	var best *Pane
	bestScore := float32(1e9)
	for _, p := range t.panes() {
		if p == t.Focus {
			continue
		}
		r := p.Bounds
		x, y := r.X+r.W/2-cx, r.Y+r.H/2-cy
		if dx*x+dy*y <= 0 {
			continue
		}
		score := x*x + y*y + 2*(dx*y-dy*x)*(dx*y-dy*x)
		if score < bestScore {
			best, bestScore = p, score
		}
	}
	if best != nil {
		a.focus(best)
	}
}

func (a *App) moveDivider(vertical bool, delta float32) {
	t := a.tab()
	if t == nil || t.Focus == nil {
		return
	}
	for n := t.Focus.Node.Parent; n != nil; n = n.Parent {
		if n.Vertical != vertical {
			continue
		}
		if n.B == t.Focus.Node || contains(n.B, t.Focus) {
			delta = -delta
		}
		n.Ratio = min(max(n.Ratio+delta, 0.1), 0.9)
		a.changed()
		return
	}
}

func contains(n *Node, p *Pane) bool {
	if n == nil {
		return false
	}
	if n.Pane != nil {
		return n.Pane == p
	}
	return contains(n.A, p) || contains(n.B, p)
}

func (a *App) currentDir() string {
	if t := a.tab(); t != nil && t.Focus != nil && t.Focus.Dir != "" {
		return t.Focus.Dir
	}
	if t := a.tab(); t != nil && t.Focus != nil && t.Focus.Info.Dir != "" {
		return t.Focus.Info.Dir
	}
	home, _ := os.UserHomeDir()
	return home
}

func (a *App) allPanes() []*Pane {
	var panes []*Pane
	for _, t := range a.tabs {
		panes = append(panes, t.panes()...)
	}
	return panes
}

func (a *App) update(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
	} else {
		fn()
	}
}

func (a *App) newTabDir() string {
	switch a.prefs.StartDirectory {
	case "home":
		home, _ := os.UserHomeDir()
		return home
	case "custom":
		return a.prefs.CustomDir
	default:
		return a.currentDir()
	}
}

func (p *Pane) label() (string, string) {
	switch p.Kind {
	case paneEmpty:
		return "New pane", "Choose a view"
	case paneBrowser:
		if p.Browser != nil {
			return p.Browser.label()
		}
		return "Browser", ""
	case paneDatabase:
		if p.Database != nil && p.Database.profile != "" {
			return p.Database.profile, "Database"
		}
		return "Database", "Connections"
	}
	name := p.Info.Program
	if name == "" || p.Info.Idle {
		name = p.Info.Shell
	}
	if name == "" {
		name = "Terminal"
	}
	if name == "zsh" || name == "bash" || name == "fish" { /* keep shell's familiar name */
	} else {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	dir := p.Info.Dir
	if dir == "" {
		dir = "~"
	}
	home, _ := os.UserHomeDir()
	if dir == home {
		dir = "~"
	} else if strings.HasPrefix(dir, home+string(filepath.Separator)) {
		dir = "~" + strings.TrimPrefix(dir, home)
	}
	return name, dir
}

func (t *Tab) label() (string, string) {
	if t.Name != "" {
		return t.Name, ""
	}
	if t.Focus != nil {
		return t.Focus.label()
	}
	return "New pane", ""
}

type savedLayout struct {
	Active int        `json:"active"`
	Tabs   []savedTab `json:"tabs"`
}
type savedTab struct {
	ID                int `json:"id,omitempty"`
	Name, Focus, Zoom string
	FocusID           int        `json:"focus_id,omitempty"`
	ZoomID            int        `json:"zoom_id,omitempty"`
	Root              *savedNode `json:"root"`
}
type savedNode struct {
	ID              int                `json:"id,omitempty"`
	Kind            string             `json:"kind,omitempty"`
	SID             string             `json:"sid,omitempty"`
	Dir             string             `json:"dir,omitempty"`
	URL             string             `json:"url,omitempty"`
	DatabaseProfile string             `json:"database_profile,omitempty"`
	DatabaseView    *savedDatabaseView `json:"database_view,omitempty"`
	Vertical        bool               `json:"vertical,omitempty"`
	Ratio           float32            `json:"ratio,omitempty"`
	A               *savedNode         `json:"a,omitempty"`
	B               *savedNode         `json:"b,omitempty"`
}

func saveNode(n *Node) *savedNode {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		p := n.Pane
		st := &savedNode{ID: p.ID, Kind: p.Kind, SID: p.SID, Dir: p.Dir}
		if p.Browser != nil {
			st.URL = p.Browser.URL
		}
		if p.Database != nil {
			st.DatabaseProfile = p.Database.profile
			view := saveDatabaseView(p.Database)
			st.DatabaseView = &view
		}
		return st
	}
	return &savedNode{Vertical: n.Vertical, Ratio: n.Ratio, A: saveNode(n.A), B: saveNode(n.B)}
}

func (a *App) snapshot() savedLayout {
	out := savedLayout{Active: a.active}
	for _, t := range a.tabs {
		st := savedTab{ID: t.ID, Name: t.Name, Root: saveNode(t.Root)}
		if t.Focus != nil {
			st.Focus = t.Focus.SID
			st.FocusID = t.Focus.ID
		}
		if t.Zoom != nil {
			st.Zoom = t.Zoom.SID
			st.ZoomID = t.Zoom.ID
		}
		out.Tabs = append(out.Tabs, st)
	}
	return out
}

func (a *App) changed() { a.saveDue = true; a.saveAt = time.Now() }

func (a *App) later(c *ui.Context, fn func()) {
	a.pending = append(a.pending, fn)
	c.Invalidate()
}

func (a *App) runPending() {
	work := a.pending
	a.pending = nil
	for _, fn := range work {
		fn()
	}
}

func (a *App) saveNow() {
	if a.client == nil {
		return
	}
	data, err := json.Marshal(a.snapshot())
	if err != nil {
		log.Printf("encode layout: %v", err)
		return
	}
	if err := a.client.Save(data); err != nil {
		log.Printf("save layout: %v", err)
		return
	}
	a.lastSnapshot = string(data)
	a.saveDue = false
}

func (a *App) restore() bool {
	data, err := a.client.Layout()
	if err != nil || len(data) == 0 {
		return false
	}
	var saved savedLayout
	if json.Unmarshal(data, &saved) != nil || len(saved.Tabs) == 0 {
		return false
	}
	var visitIDs func(*savedNode)
	visitIDs = func(n *savedNode) {
		if n == nil {
			return
		}
		a.nextID = max(a.nextID, n.ID)
		visitIDs(n.A)
		visitIDs(n.B)
	}
	for _, tab := range saved.Tabs {
		a.nextID = max(a.nextID, tab.ID)
		visitIDs(tab.Root)
	}
	list, err := a.client.List()
	if err != nil {
		return false
	}
	known := make(map[string]session.Info, len(list))
	for _, info := range list {
		known[info.ID] = info
	}
	for _, st := range saved.Tabs {
		id := st.ID
		if id == 0 {
			id = a.id()
		}
		t := &Tab{ID: id, Name: st.Name}
		t.Root = a.restoreNode(st.Root, nil, t, known)
		if t.Root == nil {
			continue
		}
		panes := t.panes()
		t.Focus = panes[0]
		for _, p := range panes {
			if st.FocusID != 0 && p.ID == st.FocusID || st.FocusID == 0 && p.SID != "" && p.SID == st.Focus {
				t.Focus = p
			}
			if st.ZoomID != 0 && p.ID == st.ZoomID || st.ZoomID == 0 && p.SID != "" && p.SID == st.Zoom {
				t.Zoom = p
			}
		}
		a.tabs = append(a.tabs, t)
	}
	if len(a.tabs) == 0 {
		return false
	}
	a.active = min(max(saved.Active, 0), len(a.tabs)-1)
	a.focusReq = a.tab().Focus
	return true
}

func (a *App) restoreNode(st *savedNode, parent *Node, tab *Tab, known map[string]session.Info) *Node {
	if st == nil {
		return nil
	}
	a.nextID = max(a.nextID, st.ID)
	n := &Node{ID: a.id(), Parent: parent, Vertical: st.Vertical, Ratio: st.Ratio}
	if st.A != nil && st.B != nil {
		if n.Ratio <= 0 || n.Ratio >= 1 {
			n.Ratio = 0.5
		}
		n.A = a.restoreNode(st.A, n, tab, known)
		n.B = a.restoreNode(st.B, n, tab, known)
		if n.A == nil {
			if n.B != nil {
				n.B.Parent = parent
			}
			return n.B
		}
		if n.B == nil {
			n.A.Parent = parent
			return n.A
		}
		return n
	}
	var p *Pane
	switch st.Kind {
	case paneEmpty:
		p = a.emptyPane(st.Dir)
	case paneBrowser:
		p = a.emptyPane(st.Dir)
		if err := a.startBrowser(p, st.URL); err != nil {
			p.ChoiceError = err.Error()
		}
	case paneDatabase:
		p = a.emptyPane(st.Dir)
		a.startDatabase(p)
		if st.DatabaseView != nil {
			restoreDatabaseView(p.Database, *st.DatabaseView)
		}
		if p.Database.profile == "" {
			p.Database.profile = st.DatabaseProfile
		}
		if p.Database.profile != "" {
			a.loadDatabaseSchema(p)
		}
	default: // Layouts saved before pane kinds contain terminal leaves.
		if info, ok := known[st.SID]; ok {
			p, _ = a.attach(info)
		}
		if p == nil {
			p, _ = a.newPane(st.Dir)
		}
	}
	if p == nil {
		return nil
	}
	if st.ID != 0 {
		p.ID = st.ID
	}
	n.Pane = p
	p.Node, p.Tab = n, tab
	return n
}

func (a *App) refresh(infos []session.Info) {
	byID := make(map[string]session.Info, len(infos))
	for _, info := range infos {
		byID[info.ID] = info
	}
	for _, t := range a.tabs {
		for _, p := range t.panes() {
			if p.Kind != paneTerminal {
				continue
			}
			info, ok := byID[p.SID]
			if !ok || p.Closed {
				continue
			}
			old := p.Info
			p.Info = info
			visible := a.tab() == t && t.Focus == p && a.win != nil && a.win.IsFocused()
			if !visible && (info.Bells > old.Bells || info.Exited && !old.Exited) {
				p.Attention = true
			}
			if a.prefs.NotifyFinished && !visible && !old.Idle && info.Idle && !old.LastInput.IsZero() && time.Since(old.LastInput) > 5*time.Second && a.win != nil && !a.win.IsFocused() {
				mygo.NewNotification(mygo.NotificationOptions{Title: "Command finished", Body: fmt.Sprintf("%s in %s", old.Program, old.Dir)}).Show()
			}
		}
	}
}

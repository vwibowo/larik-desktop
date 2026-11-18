package desktop

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/sfsymbols"
	"github.com/egoist/mygo/ui"
)

const blankPage = "about:blank"

var browserSession = browserDataSession()

// MyGo's named WKWebsiteDataStore persists from macOS 14 onward. On macOS
// 13, the app's default WKWebsiteDataStore is persistent and browser panes
// are its only users. An unknown version takes the persistent fallback.
func browserSessionForMacOS(version string) *mygo.Session {
	major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	if err != nil || major < 14 {
		return mygo.DefaultSession()
	}
	return mygo.NewSession("larik-desktop-browser")
}

type browserState struct {
	Web          *mygo.WebView
	URL          string
	Address      string
	Title        string
	Loading      bool
	Error        string
	FocusAddress bool
	off          []func()
}

func (b *browserState) close() {
	for _, off := range b.off {
		off()
	}
	b.off = nil
	if b.Web != nil {
		b.Web.Destroy()
	}
}

func (b *browserState) label() (string, string) {
	if b.Title != "" {
		return b.Title, browserHost(b.URL)
	}
	if b.URL == "" || b.URL == blankPage {
		return "Browser", "New page"
	}
	return browserHost(b.URL), b.URL
}

func browserHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

func (a *App) startBrowser(p *Pane, raw string) error {
	if p == nil || p.Kind != paneEmpty {
		return errors.New("choose an empty pane for the browser")
	}
	if raw == "" {
		raw = blankPage
	}
	create := a.newWebView
	if create == nil && a.win != nil {
		create = func(u string) (*mygo.WebView, error) {
			return a.win.NewWebView(mygo.WebViewOptions{URL: u, Page: mygo.PageOptions{
				Session: browserSession, Untrusted: true, SwipeNavigation: true, PinchZoom: true,
			}})
		}
	}
	if create == nil {
		return errors.New("browser window is unavailable")
	}
	web, err := create(raw)
	if err != nil {
		return err
	}
	if web == nil {
		return errors.New("browser window was not created")
	}
	b := &browserState{Web: web, URL: raw, Address: raw, FocusAddress: raw == blankPage}
	if raw == blankPage {
		b.Address = ""
	}
	p.Kind, p.Browser = paneBrowser, b
	page := web.Page()
	update := func(fn func()) {
		if a.win != nil {
			a.win.Update(func() {
				if !p.Closed && p.Browser == b {
					fn()
				}
			})
		}
	}
	syncNavigation := func() {
		u := page.URL()
		update(func() {
			if u != "" {
				b.URL = u
				if u == blankPage {
					b.Address = ""
				} else {
					b.Address = u
				}
				a.changed()
			}
			b.Loading = page.IsLoading()
		})
	}
	b.off = append(b.off,
		page.OnDidNavigate(func(string) { syncNavigation() }),
		page.OnPageTitleUpdated(func(e *mygo.TitleEvent) { update(func() { b.Title = e.Title }) }),
		page.OnDidFinishLoad(func() { syncNavigation(); update(func() { b.Loading = false }) }),
		page.OnDidFailLoad(func(e *mygo.LoadError) {
			update(func() { b.Loading, b.Error = false, e.Error() })
		}),
	)
	page.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if req.UserInitiated && req.URL != "" {
			update(func() {
				if p.Tab != nil {
					for i, t := range a.tabs {
						if t == p.Tab {
							a.selectTab(i)
							break
						}
					}
					a.focus(p)
					a.split(false)
					if next := a.tab().Focus; next != nil {
						if err := a.startBrowser(next, req.URL); err != nil {
							a.err = err.Error()
						}
					}
				}
			})
		}
		return nil
	})
	a.changed()
	return nil
}

// normalizeBrowserURL follows the address behavior of larik-hardness: local
// hosts use HTTP, other hosts use HTTPS, and plain words become a search.
func normalizeBrowserURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t\n") {
		return "", false
	}
	if u, err := url.Parse(s); err == nil {
		switch scheme := strings.ToLower(u.Scheme); {
		case scheme == "about" || scheme == "file":
			return s, true
		case (scheme == "http" || scheme == "https") && u.Host != "":
			return s, true
		}
	}
	host := s
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch {
	case host == "localhost" || strings.HasSuffix(host, ".localhost"):
		return "http://" + s, true
	case net.ParseIP(host) != nil:
		if ip := net.ParseIP(host); ip.IsLoopback() || ip.IsPrivate() {
			return "http://" + s, true
		}
		return "https://" + s, true
	case strings.Contains(host, ".") && !strings.HasPrefix(host, ".") && !strings.HasSuffix(host, "."):
		return "https://" + s, true
	}
	return "", false
}

func browserDestination(input string) string {
	if u, ok := normalizeBrowserURL(input); ok {
		return u
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "https://www.google.com/"
	}
	return "https://www.google.com/search?q=" + url.QueryEscape(input)
}

func (a *App) navigateBrowser(p *Pane) {
	b := p.Browser
	if b == nil || b.Web == nil {
		return
	}
	u := browserDestination(b.Address)
	if err := b.Web.Page().LoadURL(u); err != nil {
		b.Error = err.Error()
		return
	}
	b.URL, b.Address, b.Error, b.Loading = u, u, "", true
	b.Web.Focus()
	a.changed()
}

func (a *App) browserCommand(command string) {
	t := a.tab()
	if t == nil || t.Focus == nil || t.Focus.Browser == nil {
		return
	}
	b := t.Focus.Browser
	switch command {
	case "address":
		b.FocusAddress = true
		a.focusReq = nil
	case "reload":
		b.Web.Page().Reload()
	case "back":
		if b.Web.Page().CanGoBack() {
			b.Web.Page().GoBack()
		}
	case "forward":
		if b.Web.Page().CanGoForward() {
			b.Web.Page().GoForward()
		}
	}
}

func (a *App) browserPane(c *ui.Context, k *palette, p *Pane) {
	b := p.Browser
	if b == nil {
		return
	}
	ui.Row(c).Height(36).Padding(0, 7).Gap(5).AlignItems(ui.Center).Background(k.track).Children(func() {
		page := b.Web.Page()
		if chromeButton(c, k, "chevron.left", "Back").Clicked() && page.CanGoBack() {
			page.GoBack()
		}
		if chromeButton(c, k, "chevron.right", "Forward").Clicked() && page.CanGoForward() {
			page.GoForward()
		}
		if b.Loading {
			if chromeButton(c, k, "xmark", "Stop loading").Clicked() {
				page.Stop()
			}
		} else if chromeButton(c, k, "arrow.clockwise", "Reload page").Clicked() {
			page.Reload()
		}
		field := ui.TextInputBase(c.Key("address"), &b.Address).Grow(1).MinWidth(0).Height(27).
			FontSize(12).Placeholder("Search or enter address").Background(k.card).Border(1, k.border).Radius(7).Padding(0, 8).Label("Browser address")
		if b.FocusAddress {
			field.Focus()
			b.FocusAddress = false
		}
		if field.Submitted() {
			a.navigateBrowser(p)
		}
		if field.Shortcut(0, ui.KeyEscape) {
			b.Address = b.URL
			if b.URL == blankPage {
				b.Address = ""
			}
			b.Web.Focus()
		}
	})
	if b.Error != "" {
		ui.Row(c).Padding(5, 10).Background(k.track).Children(func() {
			sfsymbols.Symbol(c, "exclamationmark.triangle").Size(13).Color(k.attention)
			ui.Text(c, b.Error).FontSize(11).TextColor(k.text).Grow(1).SingleLine().Ellipsis("…")
		})
	}
	ui.WebView(c.Key("page"), b.Web).Grow(1).MinHeight(0)
}

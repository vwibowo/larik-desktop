package desktop

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"larik/internal/control"
)

//go:embed agent_snapshot.js
var agentSnapshotJS string

func (a *App) controlBrowser(req control.Request) control.Response {
	win := a.win
	if win == nil {
		return control.Response{Error: "browser commands need an open Larik Desktop window"}
	}
	ch := make(chan *browserState, 1)
	win.Update(func() {
		p := a.paneByID(req.Pane)
		if p == nil || p.Browser == nil || p.Closed {
			ch <- nil
		} else {
			ch <- p.Browser
		}
	})
	var b *browserState
	select {
	case b = <-ch:
	case <-time.After(10 * time.Second):
		return control.Response{Error: "window did not respond"}
	}
	if b == nil || b.Web == nil {
		return control.Response{Error: "browser pane not found"}
	}
	b.agentMu.Lock()
	defer b.agentMu.Unlock()
	page := b.Web.Page()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	switch req.Op {
	case "browser.navigate":
		if req.URL == "" {
			return control.Response{Error: "URL is required"}
		}
		u := browserDestination(req.URL)
		b.navSeq.Add(1)
		b.snapshotSeq, b.snapshotURL = 0, ""
		if err := page.LoadURL(u); err != nil {
			return control.Response{Error: err.Error()}
		}
		return control.Response{Value: map[string]string{"url": u}}
	case "browser.back", "browser.forward", "browser.reload":
		b.navSeq.Add(1)
		b.snapshotSeq, b.snapshotURL = 0, ""
		switch req.Op {
		case "browser.back":
			if !page.CanGoBack() {
				return control.Response{Error: "no previous page"}
			}
			page.GoBack()
		case "browser.forward":
			if !page.CanGoForward() {
				return control.Response{Error: "no next page"}
			}
			page.GoForward()
		case "browser.reload":
			page.Reload()
		}
		return control.Response{Value: map[string]string{"url": page.URL()}}
	case "browser.snapshot":
		seq := b.navSeq.Load()
		value, err := page.EvalContext(ctx, agentSnapshotJS+`("")`)
		if err != nil {
			return control.Response{Error: err.Error()}
		}
		if seq != b.navSeq.Load() {
			return control.Response{Error: "page navigated during snapshot; retry"}
		}
		b.snapshotSeq, b.snapshotURL = seq, page.URL()
		return control.Response{Value: value}
	case "browser.click", "browser.type":
		if !b.freshAgentSnapshot(page.URL()) {
			return control.Response{Error: "page changed; take a fresh snapshot"}
		}
		if !strings.HasPrefix(req.Ref, "e") || len(req.Ref) > 16 {
			return control.Response{Error: "invalid element ref"}
		}
		ref, _ := json.Marshal(req.Ref)
		base := fmt.Sprintf(`const el = window.__larikFind && window.__larikFind(%s); if (!el) return "element changed; take a fresh snapshot"; if (el.disabled) return "element is disabled"; el.scrollIntoView({block:"center",inline:"center"});`, ref)
		var script string
		if req.Op == "browser.click" {
			script = base + `el.click(); return "";`
		} else {
			value, _ := json.Marshal(req.Text)
			script = base + fmt.Sprintf(`if (!("value" in el) && !el.isContentEditable) return "element does not take text"; if (el.isContentEditable) { el.textContent = %s; } else { const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype; const setter = Object.getOwnPropertyDescriptor(proto,"value")?.set; if (setter) setter.call(el,%s); else el.value=%s; } el.dispatchEvent(new Event("input",{bubbles:true})); el.dispatchEvent(new Event("change",{bubbles:true})); %s return "";`, value, value, value, func() string {
				if req.Enter {
					return `el.form?.requestSubmit();`
				}
				return ""
			}())
		}
		value, err := page.EvalContext(ctx, script)
		if err != nil {
			return control.Response{Error: err.Error()}
		}
		if msg, ok := value.(string); ok && msg != "" {
			return control.Response{Error: msg}
		}
		return control.Response{Value: map[string]bool{"ok": true}}
	case "browser.eval":
		if strings.TrimSpace(req.Text) == "" {
			return control.Response{Error: "script is empty"}
		}
		value, err := page.EvalContext(ctx, req.Text)
		if err != nil {
			return control.Response{Error: err.Error()}
		}
		return control.Response{Value: value}
	case "browser.screenshot":
		png, err := b.Web.CapturePage()
		if err != nil {
			return control.Response{Error: err.Error()}
		}
		if len(png) > 8<<20 {
			return control.Response{Error: "page screenshot exceeds 8 MiB"}
		}
		return control.Response{PNG: png}
	}
	return control.Response{Error: errors.New("unknown browser operation").Error()}
}

func (b *browserState) freshAgentSnapshot(currentURL string) bool {
	return b.snapshotURL != "" && b.snapshotURL == currentURL && b.snapshotSeq == b.navSeq.Load()
}

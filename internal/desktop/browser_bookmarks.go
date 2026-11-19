package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"larik/internal/session"
)

type bookmark struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type bookmarkStore struct {
	path  string
	items []bookmark
}

func validBookmarkURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return false
	}
	return ((u.Scheme == "http" || u.Scheme == "https") && u.Host != "") ||
		(u.Scheme == "file" && u.Path != "")
}

func loadBookmarkStore() (*bookmarkStore, error) {
	s := &bookmarkStore{path: filepath.Join(session.Dir(), "browser-bookmarks.json")}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var saved struct {
		Bookmarks []bookmark `json:"bookmarks"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("read bookmarks: %w", err)
	}
	seen := map[string]bool{}
	for _, item := range saved.Bookmarks {
		if !validBookmarkURL(item.URL) || seen[item.URL] {
			return nil, errors.New("bookmarks file contains an invalid or duplicate URL")
		}
		seen[item.URL] = true
		item.Title = strings.TrimSpace(item.Title)
		if item.Title == "" {
			item.Title = browserHost(item.URL)
		}
		s.items = append(s.items, item)
	}
	return s, nil
}

func (s *bookmarkStore) has(raw string) bool {
	if s == nil {
		return false
	}
	for _, item := range s.items {
		if item.URL == raw {
			return true
		}
	}
	return false
}

func (s *bookmarkStore) replace(items []bookmark) error {
	if s == nil {
		return errors.New("bookmark storage is unavailable")
	}
	data, err := json.MarshalIndent(struct {
		Bookmarks []bookmark `json:"bookmarks"`
	}{items}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".browser-bookmarks-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.items = items
	return nil
}

func (s *bookmarkStore) put(item bookmark) error {
	if !validBookmarkURL(item.URL) {
		return errors.New("open a web page before bookmarking it")
	}
	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		item.Title = browserHost(item.URL)
	}
	items := []bookmark{item}
	for _, old := range s.items {
		if old.URL != item.URL {
			items = append(items, old)
		}
	}
	return s.replace(items)
}

func (s *bookmarkStore) remove(raw string) error {
	items := make([]bookmark, 0, len(s.items))
	for _, item := range s.items {
		if item.URL != raw {
			items = append(items, item)
		}
	}
	if len(items) == len(s.items) {
		return nil
	}
	return s.replace(items)
}

func (a *App) focusedBrowserPane() *Pane {
	t := a.tab()
	if t == nil || t.Focus == nil || t.Focus.Browser == nil {
		return nil
	}
	return t.Focus
}

func (a *App) bookmarkCurrent(p *Pane) {
	if p == nil || p.Browser == nil {
		return
	}
	b := p.Browser
	if a.bookmarks == nil {
		if a.bookmarkError == "" {
			a.bookmarkError = "Bookmark storage is unavailable"
		}
		return
	}
	var err error
	if a.bookmarks.has(b.URL) {
		err = a.bookmarks.remove(b.URL)
	} else {
		err = a.bookmarks.put(bookmark{Title: b.Title, URL: b.URL})
	}
	if err != nil {
		a.bookmarkError = err.Error()
	} else {
		a.bookmarkError = ""
	}
}

func (a *App) openBookmarks(p *Pane) {
	if p == nil || p.Closed || p.Browser == nil {
		return
	}
	a.bookmarksPane = p
	a.bookmarksOpen = true
	a.bookmarksQuery = ""
	focusNativeUI(a.win)
}

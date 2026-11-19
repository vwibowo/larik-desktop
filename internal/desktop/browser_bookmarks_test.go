package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBookmarkStorePersistsAndDeduplicates(t *testing.T) {
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	s, err := loadBookmarkStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.put(bookmark{Title: "First", URL: "https://example.com/one"}); err != nil {
		t.Fatal(err)
	}
	if err := s.put(bookmark{Title: "Second", URL: "https://example.com/two"}); err != nil {
		t.Fatal(err)
	}
	if err := s.put(bookmark{Title: "Updated", URL: "https://example.com/one"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadBookmarkStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.items) != 2 || loaded.items[0].Title != "Updated" || loaded.items[0].URL != "https://example.com/one" || !loaded.has("https://example.com/two") {
		t.Fatalf("unexpected saved bookmarks: %+v", loaded.items)
	}
	if err := loaded.remove("https://example.com/one"); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadBookmarkStore()
	if err != nil || len(loaded.items) != 1 || loaded.items[0].URL != "https://example.com/two" {
		t.Fatalf("remove did not persist: %+v, %v", loaded, err)
	}
}

func TestBookmarkStoreRejectsInvalidAndRetainsDataOnFailure(t *testing.T) {
	dir := t.TempDir()
	s := &bookmarkStore{path: filepath.Join(dir, "bookmarks.json")}
	for _, raw := range []string{"", "about:blank", "javascript:alert(1)", "https:///missing-host"} {
		if err := s.put(bookmark{URL: raw}); err == nil {
			t.Fatalf("accepted invalid bookmark %q", raw)
		}
	}
	if err := s.put(bookmark{URL: "https://example.com/first"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := s.path
	s.path = dir // rename over a directory fails, after the temp file is written
	if err := s.put(bookmark{URL: "https://example.com/second"}); err == nil {
		t.Fatal("expected save failure")
	}
	s.path = oldPath
	after, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || len(s.items) != 1 || s.has("https://example.com/second") {
		t.Fatal("failed save changed bookmarks")
	}
}

func TestBookmarkStoreDoesNotOverwriteInvalidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	path := filepath.Join(dir, "browser-bookmarks.json")
	if err := os.WriteFile(path, []byte(`{"bookmarks":[{"url":"javascript:bad"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The loader rejects a bad file instead of silently replacing its contents.
	if loaded, err := loadBookmarkStore(); err == nil || loaded != nil {
		t.Fatalf("invalid bookmark file loaded: %+v, %v", loaded, err)
	}
}

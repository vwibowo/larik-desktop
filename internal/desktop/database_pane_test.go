package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"larik/internal/database"
)

type testSecrets map[string][]byte

func (s testSecrets) Set(key string, value []byte) error {
	s[key] = append([]byte(nil), value...)
	return nil
}
func (s testSecrets) Get(key string) ([]byte, error) {
	if value, ok := s[key]; ok {
		return value, nil
	}
	return nil, mygo.ErrSecretNotFound
}
func (s testSecrets) Delete(key string) error { delete(s, key); return nil }

func TestDatabaseChooserAndProfileForm(t *testing.T) {
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	a := &App{prefs: defaultSettings()}
	if err := a.newTab("/tmp"); err != nil {
		t.Fatal(err)
	}
	p := a.tab().Focus
	a.choosePane(p, paneDatabase)
	if p.Kind != paneDatabase || p.Database == nil {
		t.Fatalf("database selection did not open a pane: %+v", p)
	}
	defer p.closeView()
	view := ui.NewTester(a.view, 1000, 620)
	if !view.HasText("Choose a database connection") || !view.HasText("Add connection") {
		t.Fatal("database pane has no connection chooser")
	}
	if err := view.Click("Add connection"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("Connection name") || !view.HasText("Absolute SQLite file path") {
		t.Fatal("database profile form is missing")
	}
}

func TestDatabaseProfilesUseSeparateStoreAndKeychain(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	secrets := testSecrets{}
	store := loadDatabaseStoreWithSecrets(secrets)
	if store.path != filepath.Join(dir, "database-profiles.json") {
		t.Fatalf("profile path = %q", store.path)
	}
	profile := database.ConnectionConfig{Driver: "postgres", Host: "db.example.com", Port: 5432, Database: "app", User: "reader", Password: "secret", SSLMode: "verify-full"}
	if err := store.save(map[string]database.ConnectionConfig{"analytics": profile}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || !strings.Contains(string(data), `"has_password": true`) {
		t.Fatalf("profile file contained a password or lost its marker: %s", data)
	}
	if string(secrets[databaseSecretKey("analytics")]) != "secret" {
		t.Fatal("password missing from separate secret store")
	}
	reloaded := loadDatabaseStoreWithSecrets(secrets)
	if got, ok := reloaded.get("analytics"); !ok || got.Password != "secret" || got.SSLMode != "verify-full" {
		t.Fatalf("profile did not reload: %+v", got)
	}
	if err := reloaded.save(map[string]database.ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := secrets[databaseSecretKey("analytics")]; ok {
		t.Fatal("deleting a profile left its password in the secret store")
	}
}

func TestDatabaseSaveFailurePreservesPassword(t *testing.T) {
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	secrets := testSecrets{}
	store := loadDatabaseStoreWithSecrets(secrets)
	p := database.ConnectionConfig{Driver: "postgres", Host: "127.0.0.1", Port: 5432, Database: "app", User: "reader", Password: "original"}
	if err := store.save(map[string]database.ConnectionConfig{"test": p}); err != nil {
		t.Fatal(err)
	}
	store.path = t.TempDir() // Rename to an existing directory must fail after Keychain update.
	p.Password = "replacement"
	if err := store.save(map[string]database.ConnectionConfig{"test": p}); err == nil {
		t.Fatal("expected profile save failure")
	}
	if got := string(secrets[databaseSecretKey("test")]); got != "original" {
		t.Fatalf("failed save changed Keychain password to %q", got)
	}
	if got, _ := store.get("test"); got.Password != "original" {
		t.Fatalf("failed save changed active profile: %+v", got)
	}
}

// Opt in on a Mac to verify the real login Keychain without touching a user's profiles.
func TestSystemKeychainProfileRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("LARIK_TEST_SYSTEM_KEYCHAIN") != "1" {
		t.Skip("set LARIK_TEST_SYSTEM_KEYCHAIN=1 on macOS to run the Keychain check")
	}
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	name := "larik-fixture-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	secret := "larik-test-only-keychain"
	key := databaseSecretKey(name)
	secrets := systemDatabaseSecrets{}
	t.Cleanup(func() { _ = secrets.Delete(key) })
	store := loadDatabaseStoreWithSecrets(secrets)
	p := database.ConnectionConfig{Driver: "postgres", Host: "127.0.0.1", Port: 18766, Database: "larik_fixture", User: "larik_reader", Password: secret, SSLMode: "disable"}
	if err := store.save(map[string]database.ConnectionConfig{name: p}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.path)
	if err != nil || strings.Contains(string(data), secret) {
		t.Fatalf("password exposed in profile file: %v", err)
	}
	reloaded := loadDatabaseStoreWithSecrets(secrets)
	if got, ok := reloaded.get(name); !ok || got.Password != secret {
		t.Fatalf("Keychain profile did not reload: %+v", got)
	}
	if err := reloaded.save(map[string]database.ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.Get(key); !errors.Is(err, mygo.ErrSecretNotFound) {
		t.Fatalf("test password remains in Keychain: %v", err)
	}
}

func TestDatabaseLayoutPersistsViewAndCancelsWork(t *testing.T) {
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	a := &App{prefs: defaultSettings()}
	_ = a.newTab("/tmp")
	p := a.tab().Focus
	a.choosePane(p, paneDatabase)
	d := p.Database
	d.profile, d.browserWidth, d.showDetails = "analytics", 35, true
	data, err := json.Marshal(a.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var saved savedLayout
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	leaf := saved.Tabs[0].Root
	if leaf.Kind != paneDatabase || leaf.DatabaseProfile != "analytics" || leaf.DatabaseView == nil || leaf.DatabaseView.BrowserWidth != 35 || !leaf.DatabaseView.ShowDetails {
		t.Fatalf("database view was not saved: %+v", leaf)
	}
	p.closeView()
	if d.ctx.Err() == nil {
		t.Fatal("closing database pane left work running")
	}
}

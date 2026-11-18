package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallBundle(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Applications", appName)
	bin := filepath.Join(root, "source")
	lib := filepath.Join(root, "libghostty-vt.dylib")
	if err := os.WriteFile(bin, []byte("first binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lib, []byte("ghostty library"), 0o755); err != nil {
		t.Fatal(err)
	}
	signed := false
	sign := func(stage string) error {
		signed = true
		if filepath.Ext(stage) != ".app" {
			t.Fatalf("stage %q is not an app bundle", stage)
		}
		return nil
	}
	if err := installBundle(app, bin, lib, sign); err != nil {
		t.Fatal(err)
	}
	if !signed {
		t.Fatal("bundle was not signed")
	}
	executable := filepath.Join(app, "Contents", "MacOS", "larik-desktop")
	info, err := os.Lstat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed executable has mode %v", info.Mode())
	}
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	assertContents(t, executable, "first binary")
	assertContents(t, filepath.Join(app, "Contents", "Resources", "libghostty-vt.dylib"), "ghostty library")
	icon, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "larik.icns"))
	if err != nil || len(icon) < 8 || string(icon[:4]) != "icns" {
		t.Fatalf("invalid icon: %v", err)
	}
	plist, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"larik-desktop", "dev.larik.terminal", "13.0"} {
		if !strings.Contains(string(plist), value) {
			t.Fatalf("Info.plist lacks %q", value)
		}
	}

	if err := os.WriteFile(bin, []byte("second binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installBundle(app, bin, lib, sign); err != nil {
		t.Fatal(err)
	}
	assertContents(t, executable, "second binary")

	if err := os.WriteFile(bin, []byte("broken update"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installBundle(app, bin, lib, func(string) error { return errors.New("sign failed") }); err == nil {
		t.Fatal("expected signing failure")
	}
	assertContents(t, executable, "second binary")
	if err := installBundle(app, bin, filepath.Join(root, "missing.dylib"), sign); err == nil {
		t.Fatal("expected missing library failure")
	}
	assertContents(t, executable, "second binary")
	stages, err := filepath.Glob(filepath.Join(root, "Applications", ".larik-desktop-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("leftover staging directories: %v, %v", stages, err)
	}
}

func assertContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s contains %q, want %q", path, got, want)
	}
}

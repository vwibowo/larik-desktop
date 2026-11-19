package main

import (
	"os"
	"path/filepath"
	"testing"

	"larik/internal/integration"
)

func TestSkillsCustomDirectoryCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	bin := filepath.Join(home, "Applications", "larik-desktop.app", "Contents", "MacOS", "larik-desktop")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := runSkills([]string{"install", "--dir", root}); err != nil {
		t.Fatal(err)
	}
	if err := runSkills([]string{"status", "--dir", root}); err != nil {
		t.Fatal(err)
	}
	if ok, err := integration.StatusAt(root); err != nil || !ok {
		t.Fatalf("custom skills not installed: %v, %v", ok, err)
	}
	if err := runSkills([]string{"remove", "--dir", root}); err != nil {
		t.Fatal(err)
	}
	if ok, err := integration.StatusAt(root); err != nil || ok {
		t.Fatalf("custom skills remain: %v, %v", ok, err)
	}
	for _, args := range [][]string{
		{"install", "--dir", "relative"},
		{"forget"},
		{"install", "--dir", root, "extra"},
	} {
		if err := runSkills(args); err == nil {
			t.Errorf("accepted invalid arguments %v", args)
		}
	}
}

//go:build darwin

package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"larik/internal/session"
)

func TestTerminalWindowClosePolicies(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "larik-close-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	t.Setenv("SHELL", "/bin/sh")
	done := make(chan error, 1)
	go func() { done <- session.Serve() }()
	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
		}
		c, err := session.Connect()
		if err == nil {
			_ = c.Shutdown()
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("session server did not stop")
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "server.sock")); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("session server stopped early: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("session server did not open its socket")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c, err := session.Connect()
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &Pane{SID: info.ID}
	tab := &Tab{Root: &Node{Pane: p}, Focus: p}
	a := &App{client: c, tabs: []*Tab{tab}, prefs: defaultSettings()}

	a.persistWorkspace()
	list, err := c.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("keep sessions ended a shell: %+v, %v", list, err)
	}
	var saved savedLayout
	data, err := c.Layout()
	if err != nil || json.Unmarshal(data, &saved) != nil || len(saved.Tabs) != 1 {
		t.Fatalf("keep sessions did not save layout: %s, %v", data, err)
	}

	a.prefs.KeepSessions = false
	a.persistWorkspace()
	list, err = c.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("close policy retained shells: %+v, %v", list, err)
	}
	data, err = c.Layout()
	if err != nil || json.Unmarshal(data, &saved) != nil || len(saved.Tabs) != 0 {
		t.Fatalf("close policy retained layout: %s, %v", data, err)
	}
}

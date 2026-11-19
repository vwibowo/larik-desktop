package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"larik/internal/control"
	"larik/internal/session"
)

func TestClosedControlUsesSavedPaneID(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "lcc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	t.Setenv("SHELL", "/bin/sh")
	done := make(chan error, 1)
	go func() { done <- session.Serve() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-done:
			t.Fatalf("session server exited before listen: %v", err)
		default:
		}
		if _, err := os.Stat(filepath.Join(dir, "server.sock")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session server did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	client, err := session.Existing()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Shutdown()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("session server: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("session server did not stop")
		}
	})
	info, err := client.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := json.Marshal(savedLayout{Tabs: []savedTab{{ID: 17, FocusID: 23, Root: &savedNode{ID: 23, Kind: paneTerminal, SID: info.ID}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Save(layout); err != nil {
		t.Fatal(err)
	}
	agent := control.Agent{Name: "test-agent", Path: "/test/agent"}
	grants, err := control.LoadGrants()
	if err != nil {
		t.Fatal(err)
	}
	if err := grants.Set(agent, true); err != nil {
		t.Fatal(err)
	}
	request := control.Request{Agent: agent.Name, Path: agent.Path}
	request.Op = "list"
	listed, err := ClosedControl(request)
	if err != nil || len(listed.Tabs) != 1 || listed.Tabs[0].ID != 17 || len(listed.Tabs[0].Panes) != 1 || listed.Tabs[0].Panes[0].ID != 23 {
		t.Fatalf("saved pane IDs: %+v, %v", listed, err)
	}
	request.Op, request.Pane, request.Text, request.Enter = "terminal.send", 23, "printf 'LARIK_CLOSED_CONTROL_OK\\n'", true
	if _, err := ClosedControl(request); err != nil {
		t.Fatal(err)
	}
	request.Op, request.Text, request.Enter = "terminal.read", "", false
	for time.Now().Before(deadline.Add(3 * time.Second)) {
		read, err := ClosedControl(request)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(read.Text, "LARIK_CLOSED_CONTROL_OK") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("detached shell output was not readable")
}

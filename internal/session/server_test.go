//go:build darwin

package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSessionSurvivesDetach(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "larik-session-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	t.Setenv("SHELL", "/bin/sh")
	done := make(chan error, 1)
	go func() { done <- Serve() }()
	running := false
	t.Cleanup(func() {
		if !running {
			return
		}
		c := &Client{path: socketPath()}
		_ = c.Shutdown()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("server did not stop")
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socketPath()); err == nil {
			running = true
			break
		}
		select {
		case err := <-done:
			t.Fatalf("server stopped before opening socket: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not create socket")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c := &Client{path: socketPath()}
	if _, version, err := c.Hello(); err != nil || version != ProtocolVersion {
		t.Fatalf("hello: version %d, %v", version, err)
	}
	if _, _, capabilities, err := c.HelloCapabilities(); err != nil || !slices.Contains(capabilities, ShellOverrideCapability) {
		t.Fatalf("shell capability missing: %v, %v", capabilities, err)
	}
	workDir := t.TempDir()
	info, err := c.Create(workDir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Kill(info.ID)
	stream, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The marker is encoded in the command, so an echoed input line cannot
	// satisfy the assertion without output from the running shell.
	if _, err := stream.Write([]byte("printf '\\123\\105\\123\\123\\111\\117\\116\\137\\101\\114\\111\\126\\105\\n'\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, stream, "SESSION_ALIVE")
	childDir := filepath.Join(workDir, "child")
	if err := os.Mkdir(childDir, 0o700); err != nil {
		t.Fatal(err)
	}
	canonicalDir, err := filepath.EvalSymlinks(childDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write([]byte("cd " + childDir + "\n")); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); ; {
		list, err := c.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(list) == 1 && list[0].Dir == canonicalDir {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("working directory did not update: %+v", list)
		}
		time.Sleep(30 * time.Millisecond)
	}
	stream.Close()
	if list, err := c.List(); err != nil || len(list) != 1 || list[0].Exited {
		t.Fatalf("session ended on detach: %+v, %v", list, err)
	}
	stream, err = c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, stream, "SESSION_ALIVE")
	if _, err := stream.Write([]byte("printf '\\033[?1049h\\101\\114\\124\\137\\123\\103\\122\\105\\105\\116\\n'; read x; printf '\\033[?1049l'\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, stream, "ALT_SCREEN")
	stream.Close()
	stream, err = c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, stream, "ALT_SCREEN")
	_, _ = stream.Write([]byte("\n"))
	stream.Close()
	layout := json.RawMessage(`{"tabs":[{"name":"restored"}]}`)
	if err := c.Save(layout); err != nil {
		t.Fatal(err)
	}
	got, err := c.Layout()
	if err != nil || string(got) != string(layout) {
		t.Fatalf("layout %s, %v", got, err)
	}
	custom, err := c.CreateWithShell(workDir, "/bin/zsh")
	if err != nil || custom.Shell != "zsh" {
		t.Fatalf("custom shell: %+v, %v", custom, err)
	}
	defer c.Kill(custom.ID)
	if _, err := c.CreateWithShell(workDir, "relative-shell"); err == nil {
		t.Fatal("server accepted a relative shell")
	}
}

func readUntil(t *testing.T, stream *Stream, want string) {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 8192)
		for {
			n, err := stream.Read(buf)
			out.Write(buf[:n])
			if strings.Contains(out.String(), want) || err != nil {
				done <- out.String()
				return
			}
		}
	}()
	select {
	case got := <-done:
		if !strings.Contains(got, want) {
			t.Fatalf("stream output %q lacks %q", got, want)
		}
	case <-time.After(5 * time.Second):
		stream.Close()
		t.Fatalf("timed out waiting for %q", want)
	}
}

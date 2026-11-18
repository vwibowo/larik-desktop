//go:build darwin

package session

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
)

// This helper runs the real session server in its own process. The integration
// test can then replace its app bundle while its PTY and headless VT stay alive.
func TestSessionServerHelper(t *testing.T) {
	if os.Getenv("LARIK_TEST_SESSION_SERVER_HELPER") != "1" {
		t.Skip("server helper")
	}
	if err := Serve(); err != nil {
		t.Fatal(err)
	}
}

func TestFullScreenSessionSurvivesBundleReplacement(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "larik-reopen-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("LARIK_DESKTOP_DIR", dir)
	t.Setenv("SHELL", "/bin/sh")

	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "larik-desktop.app")
	serverBinary := testBundleExecutable(t, testBinary, app)
	var serverLog bytes.Buffer
	cmd := exec.Command(serverBinary, "-test.run=^TestSessionServerHelper$")
	cmd.Env = append(os.Environ(), "LARIK_TEST_SESSION_SERVER_HELPER=1")
	cmd.Stdout, cmd.Stderr = &serverLog, &serverLog
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	c := &Client{path: socketPath()}
	t.Cleanup(func() {
		_ = c.Shutdown()
		select {
		case <-done:
			if waitErr != nil {
				t.Errorf("session server exited: %v\n%s", waitErr, serverLog.String())
			}
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Errorf("session server did not stop\n%s", serverLog.String())
		}
	})
	var serverPID int
	for deadline := time.Now().Add(5 * time.Second); ; {
		serverPID, _, err = c.Hello()
		if err == nil {
			break
		}
		select {
		case <-done:
			t.Fatalf("session server stopped early: %v\n%s", waitErr, serverLog.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("session server did not start: %v\n%s", err, serverLog.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	workDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	screenFile := filepath.Join(workDir, "screen.txt")
	if err := os.WriteFile(screenFile, []byte("SCREEN_RESTORED\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := c.CreateWithShell(workDir, "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Kill(info.ID) })
	stream, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write([]byte("/usr/bin/vi -u NONE -i NONE screen.txt\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, stream, "SCREEN_RESTORED")
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if got := waitForVi(t, c); got.PID != info.PID {
		t.Fatalf("shell changed after window detach: %+v, original PID %d", got, info.PID)
	}
	layout, err := json.Marshal(struct {
		Session string `json:"session"`
	}{Session: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Save(layout); err != nil {
		t.Fatal(err)
	}

	oldFile, err := os.Stat(serverBinary)
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(dir, "staged.app")
	testBundleExecutable(t, testBinary, stage)
	oldBundle := app + ".old"
	if err := os.Rename(app, oldBundle); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, app); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(oldBundle); err != nil {
		t.Fatal(err)
	}
	newFile, err := os.Stat(serverBinary)
	if err != nil || os.SameFile(oldFile, newFile) {
		t.Fatalf("server executable was not replaced: %v", err)
	}

	reopened := &Client{path: socketPath()}
	if got, _, err := reopened.Hello(); err != nil || got != serverPID {
		t.Fatalf("reopened app connected to server PID %d, want %d: %v", got, serverPID, err)
	}
	if got := waitForVi(t, reopened); got.PID != info.PID {
		t.Fatalf("shell changed after app update: %+v, original PID %d", got, info.PID)
	}
	if got, err := reopened.Layout(); err != nil || !bytes.Equal(got, layout) {
		t.Fatalf("saved layout changed after app update: %s, %v", got, err)
	}
	stream, err = reopened.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	view, err := terminal.New(terminal.Options{Conn: stream, Scrollback: 8 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	waitForTerminalText(t, view, "SCREEN_RESTORED")
	if _, err := stream.Write([]byte("\x1b:q!\r")); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		got := singleSession(t, reopened)
		if got.Idle {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("vi did not return to the shell: %+v", got)
		}
		time.Sleep(30 * time.Millisecond)
	}
	// Octal escapes keep the expected text out of the echoed input line.
	if _, err := stream.Write([]byte("printf '\\122\\105\\123\\125\\115\\105\\104\\137\\111\\116\\120\\125\\124\\137\\117\\113\\n'\n")); err != nil {
		t.Fatal(err)
	}
	waitForTerminalText(t, view, "RESUMED_INPUT_OK")
}

func waitForTerminalText(t *testing.T, view *terminal.Terminal, want string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		if text := view.Text(); strings.Contains(text, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("terminal screen did not contain %q: %q", want, view.Text())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func testBundleExecutable(t *testing.T, src, app string) string {
	t.Helper()
	dst := filepath.Join(app, "Contents", "MacOS", "larik-desktop")
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return dst
}

func singleSession(t *testing.T, c *Client) Info {
	t.Helper()
	list, err := c.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("want one live session, got %+v: %v", list, err)
	}
	return list[0]
}

func waitForVi(t *testing.T, c *Client) Info {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		info := singleSession(t, c)
		if !info.Exited && !info.Idle && strings.Contains(strings.ToLower(info.Program), "vi") {
			return info
		}
		if time.Now().After(deadline) {
			t.Fatalf("vi is no longer the foreground program: %+v", info)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

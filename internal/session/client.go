package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"
	"time"
)

type Client struct{ path string }

func Connect() (*Client, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("persistent sessions are supported on macOS only")
	}
	c := &Client{path: socketPath()}
	if conn, err := net.DialTimeout("unix", c.path, 250*time.Millisecond); err == nil {
		conn.Close()
		return c, nil
	}
	if err := spawn(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("unix", c.path, 200*time.Millisecond); err == nil {
			conn.Close()
			return c, nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return nil, fmt.Errorf("session server did not start; see %s", filepath.Join(Dir(), "server.log"))
}

func spawn() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(Dir(), "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	null, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer null.Close()
	cmd := exec.Command(exe, "server")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (c *Client) call(req Request) (Response, error) {
	conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var res Response
	if err := json.NewDecoder(conn).Decode(&res); err != nil {
		return Response{}, err
	}
	if res.Error != "" {
		return Response{}, errors.New(res.Error)
	}
	return res, nil
}

func (c *Client) Hello() (pid, version int, err error) {
	r, err := c.call(Request{Op: "hello"})
	return r.PID, r.Version, err
}
func (c *Client) HelloCapabilities() (pid, version int, capabilities []string, err error) {
	r, err := c.call(Request{Op: "hello"})
	return r.PID, r.Version, r.Capabilities, err
}

// UpgradeIfIdle leaves a legacy server and its shells intact. Once it has no
// sessions, it can be replaced by this executable's server without data loss.
func (c *Client) UpgradeIfIdle() (*Client, bool, error) {
	return c.upgradeIfIdle(Connect)
}

func (c *Client) upgradeIfIdle(connect func() (*Client, error)) (*Client, bool, error) {
	_, version, capabilities, err := c.HelloCapabilities()
	if err != nil {
		return c, false, err
	}
	if version != ProtocolVersion {
		return c, false, fmt.Errorf("incompatible session server version %d", version)
	}
	if slices.Contains(capabilities, ShellOverrideCapability) {
		return c, true, nil
	}
	list, err := c.List()
	if err != nil || len(list) > 0 {
		return c, false, err
	}
	if err := c.Shutdown(); err != nil {
		return c, false, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(c.path); errors.Is(err, os.ErrNotExist) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The socket is removed just before the old process releases its lock.
	time.Sleep(50 * time.Millisecond)
	fresh, err := connect()
	if err != nil {
		return c, false, err
	}
	_, _, capabilities, err = fresh.HelloCapabilities()
	if err != nil {
		return c, false, err
	}
	return fresh, slices.Contains(capabilities, ShellOverrideCapability), nil
}
func (c *Client) List() ([]Info, error) {
	r, err := c.call(Request{Op: "list"})
	return r.Sessions, err
}
func (c *Client) Create(dir string) (Info, error) {
	return c.CreateWithShell(dir, "")
}
func (c *Client) CreateWithShell(dir, shell string) (Info, error) {
	r, err := c.call(Request{Op: "create", Dir: dir, Shell: shell, Cols: 80, Rows: 24})
	return r.Session, err
}
func (c *Client) Kill(id string) error {
	_, err := c.call(Request{Op: "kill", ID: id})
	return err
}
func (c *Client) Layout() (json.RawMessage, error) {
	r, err := c.call(Request{Op: "layout"})
	return r.Layout, err
}
func (c *Client) Save(layout json.RawMessage) error {
	_, err := c.call(Request{Op: "save", Layout: layout})
	return err
}
func (c *Client) Shutdown() error {
	_, err := c.call(Request{Op: "shutdown"})
	return err
}

// Stream joins one pane to a server session. It is the terminal plugin's
// Conn, including resize reports from the native view.
type Stream struct {
	client *Client
	id     string
	conn   net.Conn
	reader *bufio.Reader
}

func (c *Client) Attach(id string) (*Stream, error) {
	conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(Request{Op: "attach", ID: id, Cols: 80, Rows: 24}); err != nil {
		conn.Close()
		return nil, err
	}
	reader := bufio.NewReaderSize(conn, 64<<10)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	var res Response
	if err := json.Unmarshal(line, &res); err != nil {
		conn.Close()
		return nil, err
	}
	if res.Error != "" {
		conn.Close()
		return nil, errors.New(res.Error)
	}
	return &Stream{client: c, id: id, conn: conn, reader: reader}, nil
}

func (s *Stream) Read(b []byte) (int, error)  { return s.reader.Read(b) }
func (s *Stream) Write(b []byte) (int, error) { return s.conn.Write(b) }
func (s *Stream) Close() error                { return s.conn.Close() }
func (s *Stream) Resize(cols, rows int) error {
	if cols < 1 || rows < 1 {
		return nil
	}
	_, err := s.client.call(Request{Op: "resize", ID: s.id, Cols: cols, Rows: rows})
	return err
}

var _ io.ReadWriteCloser = (*Stream)(nil)

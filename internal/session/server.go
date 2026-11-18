//go:build darwin

package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
)

type server struct {
	mu       sync.Mutex
	listener net.Listener
	sessions map[string]*running
	order    []string
	layout   json.RawMessage
	stop     chan struct{}
	once     sync.Once
}

type running struct {
	id, shell, dir string
	pty            *pty
	vt             *terminal.Terminal
	mu             sync.Mutex
	clients        map[*viewer]struct{}
	output         uint64
	bells          int64
	lastOutput     time.Time
	lastInput      time.Time
	exited         bool
	exitCode       int
}

type viewer struct {
	conn net.Conn
	out  chan []byte
	once sync.Once
}

type silentConn struct {
	done chan struct{}
	once sync.Once
}

func (c *silentConn) Read([]byte) (int, error)    { <-c.done; return 0, io.EOF }
func (c *silentConn) Write(b []byte) (int, error) { return len(b), nil }
func (c *silentConn) Close() error                { c.once.Do(func() { close(c.done) }); return nil }

// Serve owns PTYs after the window closes. The server is a detached process
// of the installed executable, reached only through a per-user Unix socket.
func Serve() error {
	if err := terminal.Load(); err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(Dir(), "server.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("session server is already running")
	}
	path := socketPath()
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer ln.Close()
	_ = os.Chmod(path, 0o600)
	defer os.Remove(path)
	s := &server{listener: ln, sessions: make(map[string]*running), stop: make(chan struct{})}
	if data, err := os.ReadFile(filepath.Join(Dir(), "layout.json")); err == nil && json.Valid(data) {
		s.layout = data
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stop:
				s.mu.Lock()
				for _, run := range s.sessions {
					run.pty.kill()
				}
				s.mu.Unlock()
				return nil
			default:
				return err
			}
		}
		go s.handle(conn)
	}
}

func (s *server) shutdown() {
	s.once.Do(func() { close(s.stop); _ = s.listener.Close() })
}

func (s *server) handle(conn net.Conn) {
	r := bufio.NewReaderSize(conn, 64<<10)
	line, err := r.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		conn.Close()
		return
	}
	if req.Op == "attach" {
		s.mu.Lock()
		run := s.sessions[req.ID]
		s.mu.Unlock()
		if run == nil {
			_ = json.NewEncoder(conn).Encode(Response{Error: "session no longer exists"})
			conn.Close()
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{})
		run.attach(conn, r, req.Cols, req.Rows)
		return
	}
	defer conn.Close()
	_ = json.NewEncoder(conn).Encode(s.request(req))
}

func (s *server) request(req Request) Response {
	switch req.Op {
	case "hello":
		return Response{PID: os.Getpid(), Version: ProtocolVersion, Capabilities: []string{ShellOverrideCapability}}
	case "list":
		s.mu.Lock()
		runs := make([]*running, 0, len(s.order))
		for _, id := range s.order {
			if run := s.sessions[id]; run != nil {
				runs = append(runs, run)
			}
		}
		s.mu.Unlock()
		out := make([]Info, 0, len(runs))
		for _, run := range runs {
			out = append(out, run.info())
		}
		return Response{Sessions: out}
	case "create":
		id, err := newID()
		if err != nil {
			return Response{Error: err.Error()}
		}
		run, err := start(id, req.Dir, req.Shell, req.Cols, req.Rows)
		if err != nil {
			return Response{Error: err.Error()}
		}
		s.mu.Lock()
		s.sessions[id] = run
		s.order = append(s.order, id)
		s.mu.Unlock()
		return Response{Session: run.info()}
	case "kill":
		s.mu.Lock()
		run := s.sessions[req.ID]
		delete(s.sessions, req.ID)
		for i, id := range s.order {
			if id == req.ID {
				s.order = append(s.order[:i], s.order[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		if run != nil {
			run.pty.kill()
		}
		return Response{}
	case "resize":
		s.mu.Lock()
		run := s.sessions[req.ID]
		s.mu.Unlock()
		if run == nil {
			return Response{Error: "session no longer exists"}
		}
		if err := run.resize(req.Cols, req.Rows); err != nil {
			return Response{Error: err.Error()}
		}
		return Response{}
	case "layout":
		s.mu.Lock()
		data := append(json.RawMessage(nil), s.layout...)
		s.mu.Unlock()
		return Response{Layout: data}
	case "save":
		if !json.Valid(req.Layout) {
			return Response{Error: "invalid layout JSON"}
		}
		data := append(json.RawMessage(nil), req.Layout...)
		path := filepath.Join(Dir(), "layout.json")
		if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
			return Response{Error: err.Error()}
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			return Response{Error: err.Error()}
		}
		s.mu.Lock()
		s.layout = data
		s.mu.Unlock()
		return Response{}
	case "shutdown":
		time.AfterFunc(50*time.Millisecond, s.shutdown)
		return Response{}
	}
	return Response{Error: fmt.Sprintf("unknown operation %q", req.Op)}
}

func newID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func start(id, dir, shell string, cols, rows int) (*running, error) {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	p, shell, err := startPTY(dir, shell, cols, rows)
	if err != nil {
		return nil, err
	}
	vt, err := terminal.New(terminal.Options{Conn: &silentConn{done: make(chan struct{})}, Scrollback: 8 << 20})
	if err != nil {
		p.kill()
		return nil, err
	}
	vt.Resize(cols, rows)
	run := &running{id: id, shell: shell, dir: dir, pty: p, vt: vt, clients: make(map[*viewer]struct{})}
	readDone := make(chan struct{})
	go func() { run.readOutput(); close(readDone) }()
	go func() {
		err := p.cmd.Wait()
		select {
		case <-readDone:
		case <-time.After(150 * time.Millisecond):
			_ = p.close()
			<-readDone
		}
		run.mu.Lock()
		run.exited = true
		if exit, ok := err.(*exec.ExitError); ok {
			run.exitCode = exit.ExitCode()
		}
		for v := range run.clients {
			v.finish()
		}
		clear(run.clients)
		run.mu.Unlock()
		vt.Close()
	}()
	return run, nil
}

func (run *running) readOutput() {
	buf := make([]byte, 64<<10)
	for {
		n, err := run.pty.master.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			run.mu.Lock()
			run.vt.Feed(data)
			run.output += uint64(n)
			run.lastOutput = time.Now()
			for _, b := range data {
				if b == 7 {
					run.bells++
				}
			}
			for v := range run.clients {
				if !v.send(data) {
					v.finish()
					delete(run.clients, v)
				}
			}
			run.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (run *running) info() Info {
	run.mu.Lock()
	info := Info{ID: run.id, PID: run.pty.cmd.Process.Pid, Shell: run.shell, Dir: run.dir,
		Output: run.output, Bells: run.bells, LastOutput: run.lastOutput, LastInput: run.lastInput,
		Exited: run.exited, ExitCode: run.exitCode}
	run.mu.Unlock()
	if !info.Exited {
		name, dir, idle := foreground(run.pty, run.shell)
		info.Program, info.Idle = name, idle
		if dir != "" {
			info.Dir = dir
		}
	} else {
		info.Program = run.shell
		info.Idle = true
	}
	return info
}

func (run *running) resize(cols, rows int) error {
	if cols < 1 || rows < 1 {
		return nil
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	run.vt.Resize(cols, rows)
	return run.pty.resize(cols, rows)
}

func (run *running) attach(conn net.Conn, reader *bufio.Reader, cols, rows int) {
	v := &viewer{conn: conn, out: make(chan []byte, 256)}
	run.mu.Lock()
	if cols > 0 && rows > 0 {
		run.vt.Resize(cols, rows)
		_ = run.pty.resize(cols, rows)
	}
	v.out <- run.vt.Snapshot()
	if run.exited {
		v.finish()
	} else {
		run.clients[v] = struct{}{}
	}
	run.mu.Unlock()
	go v.writeOutput()
	buf := make([]byte, 32<<10)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			run.mu.Lock()
			_, _ = run.pty.master.Write(buf[:n])
			run.lastInput = time.Now()
			run.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	run.mu.Lock()
	delete(run.clients, v)
	v.finish()
	run.mu.Unlock()
}

func (v *viewer) send(data []byte) bool {
	select {
	case v.out <- data:
		return true
	default:
		return false
	}
}
func (v *viewer) finish() { v.once.Do(func() { close(v.out) }) }
func (v *viewer) writeOutput() {
	for data := range v.out {
		for len(data) > 0 {
			n, err := v.conn.Write(data)
			if err != nil {
				v.conn.Close()
				return
			}
			data = data[n:]
		}
	}
	v.conn.Close()
}

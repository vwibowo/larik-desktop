// Package control carries local agent requests to the visible Desktop window.
package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"larik/internal/session"
)

const Version = 1
const maxRequest = 1 << 20

var ErrWindowClosed = errors.New("Larik Desktop window is not open; run `larik-desktop control launch`")

type Request struct {
	Version int    `json:"version"`
	Agent   string `json:"agent,omitempty"`
	Path    string `json:"path,omitempty"`
	Op      string `json:"op"`
	Tab     int    `json:"tab,omitempty"`
	Pane    int    `json:"pane,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Split   string `json:"split,omitempty"`
	Dir     string `json:"dir,omitempty"`
	URL     string `json:"url,omitempty"`
	Profile string `json:"profile,omitempty"`
	Schema  string `json:"schema,omitempty"`
	Table   string `json:"table,omitempty"`
	SQL     string `json:"sql,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Text    string `json:"text,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Offset  int    `json:"offset,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Enter   bool   `json:"enter,omitempty"`
}

type PaneInfo struct {
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Dir     string `json:"dir,omitempty"`
	Session string `json:"session,omitempty"`
	URL     string `json:"url,omitempty"`
	Profile string `json:"profile,omitempty"`
	Focused bool   `json:"focused,omitempty"`
}

type TabInfo struct {
	ID      int        `json:"id"`
	Name    string     `json:"name,omitempty"`
	Focused bool       `json:"focused,omitempty"`
	Panes   []PaneInfo `json:"panes"`
}

type Response struct {
	Version int       `json:"version"`
	Error   string    `json:"error,omitempty"`
	Tabs    []TabInfo `json:"tabs,omitempty"`
	Tab     *TabInfo  `json:"tab,omitempty"`
	Pane    *PaneInfo `json:"pane,omitempty"`
	Text    string    `json:"text,omitempty"`
	Value   any       `json:"value,omitempty"`
	PNG     []byte    `json:"png,omitempty"`
}

func SocketPath() string {
	sum := sha256.Sum256([]byte(session.Dir()))
	return filepath.Join("/tmp", "larik-desktop-"+hex.EncodeToString(sum[:8])+".sock")
}

func Call(req Request) (Response, error) {
	req.Version = Version
	conn, err := net.DialTimeout("unix", SocketPath(), 2*time.Second)
	if err != nil {
		return Response{}, ErrWindowClosed
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var res Response
	if err := json.NewDecoder(conn).Decode(&res); err != nil {
		return Response{}, err
	}
	if res.Version != Version {
		return Response{}, fmt.Errorf("incompatible Larik Desktop control version %d", res.Version)
	}
	if res.Error != "" {
		return res, errors.New(res.Error)
	}
	return res, nil
}

// Listen claims the window socket. The caller owns Close and removes the path.
func Listen() (net.Listener, error) {
	path := SocketPath()
	if conn, err := net.DialTimeout("unix", path, 100*time.Millisecond); err == nil {
		conn.Close()
		return nil, errors.New("Larik Desktop control socket is already active")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		os.Remove(path)
		return nil, err
	}
	return ln, nil
}

func Serve(ln net.Listener, handle func(Request) Response) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
			pid, err := CheckPeer(conn)
			if err != nil {
				_ = json.NewEncoder(conn).Encode(Response{Version: Version, Error: err.Error()})
				return
			}
			var req Request
			if err := json.NewDecoder(&boundedReader{r: conn, n: maxRequest}).Decode(&req); err != nil {
				_ = json.NewEncoder(conn).Encode(Response{Version: Version, Error: "invalid control request"})
				return
			}
			if req.Version != Version {
				_ = json.NewEncoder(conn).Encode(Response{Version: Version, Error: "unsupported control version"})
				return
			}
			agent := DetectAgentFromPeer(pid)
			req.Agent, req.Path = agent.Name, agent.Path
			res := handle(req)
			res.Version = Version
			_ = json.NewEncoder(conn).Encode(res)
		}()
	}
}

type boundedReader struct {
	r net.Conn
	n int
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, errors.New("control request too large")
	}
	if len(p) > r.n {
		p = p[:r.n]
	}
	n, err := r.r.Read(p)
	r.n -= n
	return n, err
}

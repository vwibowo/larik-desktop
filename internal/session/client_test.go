//go:build darwin

package session

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestLegacyServerKeepsActiveSessions(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "larik-legacy-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "server.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var shutdown atomic.Bool
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadBytes('\n')
				if err != nil {
					return
				}
				var req Request
				if json.Unmarshal(line, &req) != nil {
					return
				}
				var res Response
				switch req.Op {
				case "hello":
					res.Version = ProtocolVersion
					res.Capabilities = []string{ShellOverrideCapability}
				case "list":
					res.Sessions = []Info{{ID: "running-shell"}}
				case "shutdown":
					shutdown.Store(true)
				}
				_ = json.NewEncoder(conn).Encode(res)
			}()
		}
	}()
	c := &Client{path: path}
	next, ready, err := c.UpgradeIfIdle()
	if err != nil || !ready || next != c || shutdown.Load() {
		t.Fatalf("active legacy session was disturbed: client=%p ready=%v err=%v shutdown=%v", next, ready, err, shutdown.Load())
	}
}

func TestLegacyServerRestartsAfterLastSession(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "larik-upgrade-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	oldPath := filepath.Join(dir, "old.sock")
	newPath := filepath.Join(dir, "new.sock")
	old, err := net.Listen("unix", oldPath)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	fresh, err := net.Listen("unix", newPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	var shutdown atomic.Bool
	go func() {
		for {
			conn, err := old.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var req Request
				if json.NewDecoder(conn).Decode(&req) != nil {
					return
				}
				if req.Op == "shutdown" {
					shutdown.Store(true)
					_ = json.NewEncoder(conn).Encode(Response{})
					_ = old.Close()
					_ = os.Remove(oldPath)
					return
				}
				_ = json.NewEncoder(conn).Encode(Response{Version: ProtocolVersion})
			}()
		}
	}()
	go func() {
		for {
			conn, err := fresh.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = json.NewDecoder(conn).Decode(&Request{})
				_ = json.NewEncoder(conn).Encode(Response{Version: ProtocolVersion, Capabilities: []string{ShellOverrideCapability, TerminalControlCapability}})
			}()
		}
	}()
	c := &Client{path: oldPath}
	connected := false
	next, ready, err := c.upgradeIfIdle(func() (*Client, error) {
		connected = true
		return &Client{path: newPath}, nil
	})
	if err != nil || !ready || !shutdown.Load() || !connected || next.path != newPath {
		t.Fatalf("idle legacy server did not upgrade: ready=%v shutdown=%v connected=%v next=%v err=%v", ready, shutdown.Load(), connected, next, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old socket still exists: %v", err)
	}
}

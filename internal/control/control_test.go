//go:build darwin

package control

import (
	"os"
	"testing"
)

func TestLocalSocketOverridesClaimedIdentity(t *testing.T) {
	t.Setenv("LARIK_DESKTOP_DIR", t.TempDir())
	ln, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { ln.Close(); os.Remove(SocketPath()) }()
	go Serve(ln, func(req Request) Response {
		return Response{Value: Agent{Name: req.Agent, Path: req.Path}}
	})
	res, err := Call(Request{Op: "list", Agent: "pretend-agent", Path: "/pretend"})
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := res.Value.(map[string]any)
	if !ok || identity["name"] == "pretend-agent" || identity["path"] == "/pretend" {
		t.Fatalf("server trusted caller-supplied identity: %+v", res.Value)
	}
	if mode, err := os.Stat(SocketPath()); err != nil || mode.Mode().Perm() != 0o600 {
		t.Fatalf("socket permissions: %v, %v", mode, err)
	}
}

//go:build !darwin

package session

import (
	"errors"
	"os"
)

type pty struct{ master *os.File }

func startPTY(string, int, int) (*pty, string, error) {
	return nil, "", errors.New("persistent sessions are supported on macOS only")
}
func (*pty) resize(int, int) error { return errors.New("unsupported") }
func (*pty) close() error          { return nil }
func (*pty) kill()                 {}

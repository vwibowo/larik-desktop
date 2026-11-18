//go:build !darwin

package session

import "errors"

func Serve() error { return errors.New("persistent sessions are supported on macOS only") }

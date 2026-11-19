//go:build !darwin

package control

import (
	"errors"
	"net"
)

func CheckPeer(net.Conn) (int, error) {
	return 0, errors.New("Larik Desktop control is supported on macOS only")
}

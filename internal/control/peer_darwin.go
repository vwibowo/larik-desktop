//go:build darwin

package control

import (
	"errors"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

func CheckPeer(conn net.Conn) (int, error) {
	u, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, errors.New("control connection is not local")
	}
	raw, err := u.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var pid int
	var checkErr error
	err = raw.Control(func(fd uintptr) {
		var cred *unix.Xucred
		cred, checkErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if checkErr == nil {
			uid = cred.Uid
			pid, checkErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		}
	})
	if err != nil {
		return 0, err
	}
	if checkErr != nil {
		return 0, checkErr
	}
	if uid != uint32(os.Getuid()) {
		return 0, errors.New("control connection belongs to another user")
	}
	return pid, nil
}

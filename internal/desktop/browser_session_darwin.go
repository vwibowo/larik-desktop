//go:build darwin

package desktop

import (
	"github.com/egoist/mygo"
	"golang.org/x/sys/unix"
)

func browserDataSession() *mygo.Session {
	version, _ := unix.Sysctl("kern.osproductversion")
	return browserSessionForMacOS(version)
}

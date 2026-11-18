//go:build darwin

package session

import (
	"bytes"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

var processLookup struct {
	once sync.Once
	path func(int32, int32, uint64, unsafe.Pointer, int32) int32
}

// foreground reads the controlling PTY's active process group, then asks
// macOS for that process's name and working directory.
func foreground(p *pty, shell string) (name, dir string, idle bool) {
	pid := p.cmd.Process.Pid
	group, err := unix.IoctlGetInt(int(p.master.Fd()), unix.TIOCGPGRP)
	if err == nil && group > 0 {
		pid = group
	}
	name = processName(pid)
	if name == "" {
		name = shell
	}
	dir = processDir(pid)
	return name, dir, pid == p.cmd.Process.Pid
}

func processName(pid int) string {
	proc, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return ""
	}
	name := proc.Proc.P_comm[:]
	if n := bytes.IndexByte(name, 0); n >= 0 {
		name = name[:n]
	}
	return string(name)
}

func processDir(pid int) string {
	processLookup.once.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			purego.RegisterLibFunc(&processLookup.path, lib, "proc_pidinfo")
		}
	})
	if processLookup.path == nil {
		return ""
	}
	// PROC_PIDVNODEPATHINFO contains a 152-byte vnode_info before cwd's
	// MAXPATHLEN (1024) path. Its second half describes the executable.
	var info [2352]byte
	if processLookup.path(int32(pid), 9, 0, unsafe.Pointer(&info[0]), int32(len(info))) <= 0 {
		return ""
	}
	path := info[152:1176]
	if n := bytes.IndexByte(path, 0); n >= 0 {
		path = path[:n]
	}
	return string(path)
}

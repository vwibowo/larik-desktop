//go:build darwin

package session

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type pty struct {
	master *os.File
	cmd    *exec.Cmd
}

func startPTY(dir, shell string, cols, rows int) (*pty, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	var name [128]byte
	for _, ioctl := range []struct {
		request uintptr
		arg     unsafe.Pointer
	}{
		{0x20007454, nil},                      // TIOCPTYGRANT
		{0x20007452, nil},                      // TIOCPTYUNLK
		{0x40807453, unsafe.Pointer(&name[0])}, // TIOCPTYGNAME
	} {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), ioctl.request, uintptr(ioctl.arg)); e != 0 {
			master.Close()
			return nil, "", fmt.Errorf("open PTY: %w", e)
		}
	}
	end := bytes.IndexByte(name[:], 0)
	if end < 0 {
		end = len(name)
	}
	slave, err := os.OpenFile(string(name[:end]), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, "", err
	}
	defer slave.Close()
	if err := setSize(master, cols, rows); err != nil {
		master.Close()
		return nil, "", err
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir, _ = os.UserHomeDir()
	}
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
	}
	if !filepath.IsAbs(shell) {
		master.Close()
		return nil, "", fmt.Errorf("shell must be an absolute path")
	}
	if stat, err := os.Stat(shell); err != nil || stat.IsDir() || stat.Mode()&0o111 == 0 {
		master.Close()
		return nil, "", fmt.Errorf("shell must be an executable file: %s", shell)
	}
	cmd := &exec.Cmd{
		Path: shell, Args: []string{"-" + filepath.Base(shell)}, Dir: dir,
		Env:   append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=LarikDesktop"),
		Stdin: slave, Stdout: slave, Stderr: slave,
		SysProcAttr: &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0},
	}
	if err := cmd.Start(); err != nil {
		master.Close()
		return nil, "", err
	}
	return &pty{master: master, cmd: cmd}, filepath.Base(shell), nil
}

func setSize(f *os.File, cols, rows int) error {
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &unix.Winsize{
		Col: uint16(max(1, min(cols, 65535))), Row: uint16(max(1, min(rows, 65535))),
	})
}

func (p *pty) resize(cols, rows int) error { return setSize(p.master, cols, rows) }
func (p *pty) close() error                { return p.master.Close() }
func (p *pty) kill() {
	if p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGHUP)
		_ = p.master.Close()
	}
}

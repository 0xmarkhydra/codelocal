//go:build !windows

package process

import (
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type ptyHandle interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Signal(signal string) error
}

type unixPTY struct {
	file *os.File
	cmd  *exec.Cmd
}

func (p *unixPTY) Read(buf []byte) (int, error)  { return p.file.Read(buf) }
func (p *unixPTY) Write(buf []byte) (int, error) { return p.file.Write(buf) }
func (p *unixPTY) Close() error                  { return p.file.Close() }
func (p *unixPTY) Resize(cols, rows int) error {
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 36
	}
	return pty.Setsize(p.file, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (p *unixPTY) Signal(signal string) error {
	if p.cmd == nil || p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Signal(parseSignal(signal))
}

func startPTY(cmd *exec.Cmd, cols, rows int) (ptyHandle, error) {
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 36
	}
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &unixPTY{file: file, cmd: cmd}, nil
}

func parseSignal(value string) os.Signal {
	switch value {
	case "SIGINT":
		return syscall.SIGINT
	case "SIGKILL":
		return syscall.SIGKILL
	default:
		return syscall.SIGTERM
	}
}

func signalProcess(cmd *exec.Cmd, handle ptyHandle, signal string) error {
	if handle != nil {
		return handle.Signal(signal)
	}
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Signal(parseSignal(signal))
}

func terminateProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	return nil
}

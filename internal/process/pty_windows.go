//go:build windows

package process

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

type ptyHandle interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Signal(signal string) error
}

func startPTY(*exec.Cmd, int, int) (ptyHandle, error) {
	return nil, errors.New("native PTY is not available in this Windows build")
}

func signalProcess(cmd *exec.Cmd, _ ptyHandle, signal string) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	if signal == "SIGKILL" {
		return cmd.Process.Kill()
	}
	return cmd.Process.Signal(os.Interrupt)
}

func terminateProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

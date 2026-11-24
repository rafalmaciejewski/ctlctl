//go:build !windows

package loader

import (
	"os/exec"
	"syscall"
)

// isolate starts the tool in its own session, without a controlling
// terminal, so it can't read from or draw on ctlctl's screen, and lets a
// timeout kill everything it started.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

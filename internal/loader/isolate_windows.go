//go:build windows

package loader

import (
	"os/exec"
	"syscall"
)

// isolate keeps the tool from opening a console window of its own.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

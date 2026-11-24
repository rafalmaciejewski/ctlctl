//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

// Exec replaces ctlctl with the shell running line, so the command owns the
// terminal, receives signals directly and its exit status becomes ours.
func Exec(sh, flag, line string) error {
	path, err := exec.LookPath(sh)
	if err != nil {
		return err
	}
	return syscall.Exec(path, []string{sh, flag, line}, os.Environ())
}

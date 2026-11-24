//go:build windows

package runner

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// Exec runs line in the shell and exits with its status.
func Exec(sh, flag, line string) error {
	signal.Ignore(os.Interrupt)
	err := Command(sh, flag, line).Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

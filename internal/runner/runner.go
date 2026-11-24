// Package runner runs the command the user built.
package runner

import (
	"os"
	"os/exec"
	"runtime"
)

// Shell returns the shell that runs commands and the flag that passes it a
// command string.
func Shell(configured string) (string, string) {
	if runtime.GOOS == "windows" {
		if configured != "" {
			return configured, "/C"
		}
		if c := os.Getenv("COMSPEC"); c != "" {
			return c, "/C"
		}
		return "cmd.exe", "/C"
	}
	if configured != "" {
		return configured, "-c"
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s, "-c"
	}
	return "/bin/sh", "-c"
}

// Command returns a command that runs line in the shell with the terminal
// attached.
func Command(sh, flag, line string) *exec.Cmd {
	cmd := exec.Command(sh, flag, line)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// Pause returns a command that runs line, then waits for Enter so the output
// stays on screen before ctlctl comes back.
func Pause(sh, flag, line string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command(sh, flag, line+" & echo. & pause")
	}
	script := `"$0" "$1" "$2"; s=$?; printf '\n\033[2m[exit %s] press enter to return to ctlctl\033[0m' "$s"; read -r _`
	return exec.Command("/bin/sh", "-c", script, sh, flag, line)
}

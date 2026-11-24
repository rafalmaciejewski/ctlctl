// Package history keeps the commands ctlctl ran.
package history

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxEntries = 1000

// History is a list of commands, oldest first, backed by a file.
type History struct {
	path    string
	entries []string
}

// DefaultPath is $XDG_STATE_HOME/ctlctl/history (~/.local/state on Unix,
// %LOCALAPPDATA% on Windows).
func DefaultPath() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		if runtime.GOOS == "windows" {
			dir = os.Getenv("LOCALAPPDATA")
		} else if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".local", "state")
		}
	}
	return filepath.Join(dir, "ctlctl", "history")
}

// Open reads the history file. A missing or unreadable file gives an empty
// history. An empty path keeps the history in memory only.
func Open(path string) *History {
	h := &History{path: path}
	if path == "" {
		return h
	}
	f, err := os.Open(path)
	if err != nil {
		return h
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			h.entries = append(h.entries, l)
		}
	}
	if len(h.entries) > maxEntries {
		h.entries = h.entries[len(h.entries)-maxEntries:]
	}
	return h
}

// Entries returns the commands, oldest first.
func (h *History) Entries() []string { return h.entries }

// Add records a command and appends it to the file.
func (h *History) Add(cmd string) error {
	cmd = strings.TrimSpace(strings.ReplaceAll(cmd, "\n", " "))
	if cmd == "" {
		return nil
	}
	if n := len(h.entries); n > 0 && h.entries[n-1] == cmd {
		return nil
	}
	h.entries = append(h.entries, cmd)
	if h.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return err
	}
	if len(h.entries) > maxEntries {
		h.entries = h.entries[len(h.entries)-maxEntries:]
		return os.WriteFile(h.path, []byte(strings.Join(h.entries, "\n")+"\n"), 0o600)
	}
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(cmd + "\n")
	return err
}

// Matching returns the commands starting with prefix, newest first, without
// duplicates.
func (h *History) Matching(prefix string) []string {
	var out []string
	seen := map[string]bool{}
	for i := len(h.entries) - 1; i >= 0; i-- {
		e := h.entries[i]
		if strings.HasPrefix(e, prefix) && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

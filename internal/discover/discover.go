// Package discover finds the tools ctlctl lists.
package discover

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
)

// Tool is an executable found on PATH.
type Tool struct {
	Name string // command name without extension
	Path string // absolute path to the executable
}

// Options selects which tools to list.
type Options struct {
	Suffixes []string
	Include  []string
	Exclude  []string
	PathEnv  string // defaults to $PATH
	Self     string // ctlctl's own name, never listed
}

// Find scans PATH for executables the current user can run whose names end
// with one of the suffixes, and adds the included tools. It returns them
// sorted by name. When a name appears in several PATH directories, the first
// one wins, as it would in a shell.
func Find(o Options) []Tool {
	pathEnv := o.PathEnv
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	seen := map[string]bool{}
	var tools []Tool
	add := func(t Tool) {
		if seen[t.Name] || slices.Contains(o.Exclude, t.Name) || t.Name == o.Self {
			return
		}
		seen[t.Name] = true
		tools = append(tools, t)
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name, ok := commandName(e.Name())
			if !ok || !hasSuffix(name, o.Suffixes) || seen[name] {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if !isExecutable(p) {
				continue
			}
			add(Tool{Name: name, Path: p})
		}
	}
	for _, inc := range o.Include {
		if t, ok := Lookup(inc); ok {
			add(t)
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

// Lookup resolves a tool by name or path.
func Lookup(name string) (Tool, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return Tool{}, false
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	n, _ := commandName(filepath.Base(p))
	return Tool{Name: n, Path: p}, true
}

func hasSuffix(name string, suffixes []string) bool {
	for _, s := range suffixes {
		if s != "" && strings.HasSuffix(name, s) && len(name) > len(s) {
			return true
		}
	}
	return false
}

// commandName strips executable extensions on Windows.
func commandName(file string) (string, bool) {
	if runtime.GOOS != "windows" {
		return file, true
	}
	ext := strings.ToLower(filepath.Ext(file))
	pathext := os.Getenv("PATHEXT")
	if pathext == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}
	for _, e := range strings.Split(strings.ToLower(pathext), ";") {
		if e != "" && e == ext {
			return strings.TrimSuffix(file, filepath.Ext(file)), true
		}
	}
	return "", false
}

// isExecutable reports whether the current user can run the file at p.
func isExecutable(p string) bool {
	fi, err := os.Stat(p) // follows symlinks, so dangling links are skipped
	if err != nil || fi.IsDir() {
		return false
	}
	return canExecute(p, fi)
}

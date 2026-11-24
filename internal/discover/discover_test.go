package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFind(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix permissions")
	}
	a, b := t.TempDir(), t.TempDir()
	write := func(dir, name string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(a, "kubectl", 0o755)
	write(a, "notes.ctl", 0o644) // not executable
	write(a, "ctlctl", 0o755)    // ourselves
	write(a, "ctl", 0o755)       // the suffix alone
	write(b, "kubectl", 0o755)   // shadowed
	write(b, "systemctl", 0o755)
	write(b, "sysctl", 0o755)
	write(b, "docker", 0o755)
	if err := os.Symlink(filepath.Join(b, "missing"), filepath.Join(b, "brokenctl")); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", a+string(os.PathListSeparator)+b)
	tools := Find(Options{
		Suffixes: []string{"ctl"},
		Include:  []string{"docker"},
		Exclude:  []string{"sysctl"},
		Self:     "ctlctl",
	})
	var names []string
	for _, tl := range tools {
		names = append(names, tl.Name)
	}
	want := []string{"docker", "kubectl", "systemctl"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
	if tools[1].Path != filepath.Join(a, "kubectl") {
		t.Errorf("kubectl path = %s, want the first one on PATH", tools[1].Path)
	}
}

func TestFindSkipsToolsTheUserCannotRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix permissions")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can run anything with an execute bit")
	}
	dir := t.TempDir()
	for name, mode := range map[string]os.FileMode{
		"minectl":   0o700, // ours, executable by us
		"othersctl": 0o011, // executable by group and others, but not by us, the owner
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	tools := Find(Options{Suffixes: []string{"ctl"}, PathEnv: dir})
	if len(tools) != 1 || tools[0].Name != "minectl" {
		t.Fatalf("tools = %+v, want only minectl", tools)
	}
}

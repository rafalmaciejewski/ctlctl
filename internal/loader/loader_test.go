package loader

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rafalmaciejewski/ctlctl/internal/helpdoc"
)

// fakeTool writes a shell script that prints help like a systemd tool: the
// same text for every subcommand.
func fakeTool(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts")
	}
	p := filepath.Join(t.TempDir(), "fakectl")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

const rootHelp = `fakectl [OPTIONS...] COMMAND

Commands:
  start UNIT    Start a unit
  stop UNIT     Stop a unit

Options:
  -h --help     Show this help
`

func TestLoadRoot(t *testing.T) {
	bin := fakeTool(t, "cat <<'EOF'\n"+rootHelp+"EOF\n")
	r := Load(context.Background(), Request{Bin: bin, Name: "fakectl"})
	if r.Err != nil || r.Doc == nil {
		t.Fatalf("Load: %+v", r)
	}
	if r.Source != "fakectl --help" {
		t.Errorf("source = %q", r.Source)
	}
	if len(r.Doc.Commands) != 2 {
		t.Errorf("commands = %+v", r.Doc.Commands)
	}
}

func TestLoadInherited(t *testing.T) {
	bin := fakeTool(t, "cat <<'EOF'\n"+rootHelp+"EOF\n")
	root := helpdoc.Parse(rootHelp, []string{"fakectl"})
	r := Load(context.Background(), Request{
		Bin: bin, Name: "fakectl", Args: []string{"start"},
		Ancestors:    []*helpdoc.Doc{root},
		AncestorText: []string{helpdoc.Normalize(rootHelp)},
		DashH:        true,
	})
	if r.Inherited != 0 {
		t.Errorf("inherited = %d, want 0 (%+v)", r.Inherited, r)
	}
}

func TestSubcommandsAreNotRunWithoutAHelpMechanism(t *testing.T) {
	// A tool like cpuctl ignores --help and runs the subcommand. Unless its
	// help documents a way to ask for a subcommand's help, nothing runs.
	marker := filepath.Join(t.TempDir(), "ran")
	bin := fakeTool(t, "if [ $# -gt 1 ]; then touch "+marker+"; fi\necho 'usage: fakectl [ list ]'\n")
	root := helpdoc.Parse("usage: fakectl [ list ]", []string{"fakectl"})
	req := Request{Bin: bin, Name: "fakectl", Args: []string{"list"}, Ancestors: []*helpdoc.Doc{root}, AncestorText: []string{""}}
	if r := Load(context.Background(), req); r.Inherited != 0 {
		t.Errorf("inherited = %d, want 0", r.Inherited)
	}
	req.HelpFlag, req.NoProbe = true, true
	if r := Load(context.Background(), req); r.Inherited != 0 {
		t.Errorf("NoProbe: inherited = %d, want 0", r.Inherited)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the subcommand was run")
	}
}

func TestLoadPrefersHelpWithoutErrors(t *testing.T) {
	bin := fakeTool(t, `case "$1" in
--help) echo "fakectl: illegal option -- -"; echo "usage: fakectl [ list ]"; echo "       fakectl { offline | online } <cpu>"; exit 64 ;;
-h) echo "usage: fakectl [ list ]"; echo "       fakectl { offline | online } <cpu>"; exit 64 ;;
help) touch /dev/null; echo "should not run"; exit 1 ;;
esac
`)
	r := Load(context.Background(), Request{Bin: bin, Name: "fakectl"})
	if r.Source != "fakectl -h" {
		t.Errorf("source = %q, want fakectl -h", r.Source)
	}
	if r.Doc == nil || len(r.Doc.Commands) != 3 {
		t.Errorf("doc = %+v", r.Doc)
	}
}

func TestLoadTriesHelpCommandWhenOffered(t *testing.T) {
	bin := fakeTool(t, `case "$1" in
help) printf 'Usage: fakectl <subcommand>\n\nSubcommands:\n  list   List things\n  help   Prints the usage for a subcommand\n' ;;
*) echo "Unrecognized subcommand: $1"; echo "Usage: fakectl <subcommand> ... | help [subcommand]" ;;
esac
`)
	r := Load(context.Background(), Request{Bin: bin, Name: "fakectl"})
	if r.Source != "fakectl help" || r.Doc == nil || r.Doc.FindCommand("list") == nil {
		t.Errorf("Load = %q %+v", r.Source, r.Doc)
	}
}

func TestLoadNoTTYAndTimeout(t *testing.T) {
	// The tool must not be able to open the terminal.
	bin := fakeTool(t, "if (exec </dev/tty) 2>/dev/null; then echo has-tty; else echo no-tty; fi\necho line2\necho line3\n")
	r := Load(context.Background(), Request{Bin: bin, Name: "fakectl"})
	if r.Doc == nil || len(r.Doc.Lines) == 0 || r.Doc.Lines[0].Text != "no-tty" {
		t.Errorf("tool got a terminal: %+v", r.Doc)
	}

	bin = fakeTool(t, "sleep 10\n")
	start := time.Now()
	r = Load(context.Background(), Request{Bin: bin, Name: "fakectl", Args: []string{"x"}, HelpFlag: true, Timeout: 200 * time.Millisecond})
	if time.Since(start) > 3*time.Second {
		t.Errorf("timeout not enforced: took %s", time.Since(start))
	}
	if r.Err == nil {
		t.Errorf("expected a timeout error")
	}
}

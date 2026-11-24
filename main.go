// Command ctlctl is a fullscreen browser for *ctl command-line tools: pick a
// tool and its subcommands from a tree, read their help, click flags into
// the command line and run it.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/rafalmaciejewski/ctlctl/internal/config"
	"github.com/rafalmaciejewski/ctlctl/internal/discover"
	"github.com/rafalmaciejewski/ctlctl/internal/history"
	"github.com/rafalmaciejewski/ctlctl/internal/runner"
	"github.com/rafalmaciejewski/ctlctl/internal/tui"
)

// version is set at build time by GoReleaser.
var version = ""

func getVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "dev"
}

const usage = `ctlctl - browse and run your *ctl command-line tools

Usage:
  ctlctl [flags] [tool [subcommand...]]

Opens a fullscreen view of the executables on your PATH whose names end with
"ctl" (kubectl, systemctl, launchctl, ...). Pick a tool and its subcommands in
the tree, read their --help, click flags into the command line, and press
Enter to run the command.

Naming a tool opens it right away, even if its name doesn't end with "ctl":
  ctlctl kubectl config
  ctlctl docker

Flags:
  -p, --print     print the command instead of running it (for shell widgets)
  -l, --list      list the tools ctlctl finds and exit
  -v, --version   print the version and exit
  -h, --help      show this help

Config file: %s
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var printMode, list, showVersion bool
	var positional []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "-p", "--print":
			printMode = true
		case "-l", "--list":
			list = true
		case "-v", "--version":
			showVersion = true
		case "-h", "--help":
			fmt.Fprintf(stdout, usage, config.Path())
			return 0
		case "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "ctlctl: unknown flag %s\nRun 'ctlctl --help' for usage.\n", a)
				return 2
			}
			positional = append(positional, a)
		}
	}
	if showVersion {
		fmt.Fprintln(stdout, "ctlctl", getVersion())
		return 0
	}

	cfgPath := config.Path()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "ctlctl: ignoring %s: %v\n", cfgPath, err)
	}
	tools := discover.Find(discover.Options{
		Suffixes: cfg.Suffixes,
		Include:  cfg.Include,
		Exclude:  cfg.Exclude,
		Self:     "ctlctl",
	})
	if len(positional) > 0 {
		t, ok := discover.Lookup(positional[0])
		if !ok {
			fmt.Fprintf(stderr, "ctlctl: %s: command not found\n", positional[0])
			return 1
		}
		positional[0] = t.Name
		found := false
		for _, have := range tools {
			if have.Name == t.Name {
				found = true
			}
		}
		if !found {
			tools = append([]discover.Tool{t}, tools...)
		}
	}
	if list {
		for _, t := range tools {
			fmt.Fprintf(stdout, "%-24s %s\n", t.Name, t.Path)
		}
		return 0
	}

	// With --print, or when stdout is captured, draw on the terminal itself
	// so stdout carries only the command.
	var in io.Reader = os.Stdin
	var out io.Writer = os.Stdout
	if printMode || !isTerminal(os.Stdout) {
		if tty, err := openTTY(); err == nil {
			defer tty.Close()
			in, out = tty, tty
		} else if runtime.GOOS == "windows" || isTerminal(os.Stderr) {
			out = os.Stderr
		} else {
			fmt.Fprintln(stderr, "ctlctl: needs a terminal")
			return 1
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := tui.New(tui.Options{
		Tools:       tools,
		InitialPath: positional,
		History:     history.Open(history.DefaultPath()),
		Shell:       cfg.Shell,
		HelpTimeout: cfg.Timeout(),
		Version:     getVersion(),
		Output:      out,
		Context:     ctx,
	})
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithContext(ctx),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(stderr, "ctlctl:", err)
		return 1
	}
	cancel()

	res := m.Result()
	if res.Command == "" {
		if printMode {
			return 130
		}
		return 0
	}
	if printMode {
		fmt.Fprintln(stdout, res.Command)
		return 0
	}
	fmt.Fprintln(stderr, m.HighlightCommand(res.Command))
	sh, flag := runner.Shell(cfg.Shell)
	if err := runner.Exec(sh, flag, res.Command); err != nil {
		fmt.Fprintln(stderr, "ctlctl:", err)
		return 1
	}
	return 0
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

func openTTY() (*os.File, error) {
	if runtime.GOOS == "windows" {
		return nil, os.ErrNotExist
	}
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

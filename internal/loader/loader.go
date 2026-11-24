// Package loader runs tools to get their help text.
package loader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/rafalmaciejewski/ctlctl/internal/helpdoc"
	"github.com/rafalmaciejewski/ctlctl/internal/shell"
)

// Request asks for the help of one command.
type Request struct {
	Bin  string   // the tool's executable
	Name string   // the tool's name
	Args []string // subcommands below the tool

	// Ancestors holds the help of the parent commands, nearest first. A
	// subcommand whose help matches one of them has no help of its own.
	Ancestors []*helpdoc.Doc
	// AncestorText holds the normalized help text of the same ancestors.
	AncestorText []string
	// How the tool takes requests for a subcommand's help, from its
	// ancestors' help. A subcommand is only run with one of these, so a tool
	// that ignores help flags isn't made to run the subcommand for real.
	HelpCommand bool // the parent lists a "help" subcommand: `tool help sub`
	HelpFlag    bool // an ancestor documents --help: `tool sub --help`
	DashH       bool // an ancestor documents -h as help: `tool sub -h`
	// NoProbe is set for subcommands inferred from usage lines or man pages.
	// Nothing is run for them; they show their parent's help.
	NoProbe bool

	Width   int
	Timeout time.Duration
}

// Response is the help of one command.
type Response struct {
	Doc    *helpdoc.Doc
	Source string // the command line that printed it
	// Inherited is the index in Request.Ancestors of the ancestor whose help
	// this command printed, or -1.
	Inherited int
	// Global holds options that apply to every subcommand, for tools that
	// list them separately (`kubectl options`).
	Global       *helpdoc.Doc
	GlobalSource string
	Err          error
}

// Probes reports whether Load runs the tool for r. A tool always runs for
// its own help; a subcommand only when its ancestors document a way to ask
// for its help and it wasn't inferred from a usage line or man page.
// Otherwise the subcommand shows its parent's help.
func Probes(r Request) bool {
	return len(r.Args) == 0 || (!r.NoProbe && (r.HelpCommand || r.HelpFlag || r.DashH))
}

// Func loads help. Tests replace it.
type Func func(ctx context.Context, r Request) Response

type attempt struct {
	args []string
	out  string
	norm string
	code int
	weak bool // starts by complaining about the option it was given
}

var (
	reOptionError = regexp.MustCompile(`(?i)^(?:\S+:\s*)?(?:illegal|unrecognized|unknown|invalid|unexpected)\s+(?:option|flag|argument|subcommand|command)|^error: unexpected argument`)
	reOffersHelp  = regexp.MustCompile(`(?i)\bhelp\s+[\[<]?(?:sub)?command`)
)

// optionError reports whether output starts with a complaint about the help
// flag ("cpuctl: illegal option -- -"): the tool printed its usage by
// accident, and another flag may give cleaner help.
func optionError(out string) bool {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return reOptionError.MatchString(l)
		}
	}
	return false
}

// better prefers output that doesn't start with an error, then longer output.
func better(a, b *attempt) bool {
	if a.weak != b.weak {
		return !a.weak
	}
	return len(a.norm) > len(b.norm)
}

// Load runs the tool with the help flags it most likely understands, and
// returns the first output that looks like help.
func Load(ctx context.Context, r Request) Response {
	if r.Timeout <= 0 {
		r.Timeout = 5 * time.Second
	}
	path := append([]string{r.Name}, r.Args...)
	var tries [][]string
	if len(r.Args) == 0 {
		// `tool help` is only tried when the output so far offers it.
		tries = [][]string{{"--help"}, {"-h"}}
	} else {
		// A help subcommand never runs the subcommand itself, so it goes
		// first.
		if r.HelpCommand {
			tries = append(tries, append([]string{"help"}, r.Args...))
		}
		if r.HelpFlag {
			tries = append(tries, append(slices.Clone(r.Args), "--help"))
		}
		if r.DashH {
			tries = append(tries, append(slices.Clone(r.Args), "-h"))
		}
		if !Probes(r) {
			return Response{Inherited: 0}
		}
	}

	var best, inherited *attempt
	inheritedIdx := -1
	var lastErr error
	triedHelp := false
	for i := 0; i < len(tries); i++ {
		args := tries[i]
		out, code, err := run(ctx, r, args)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
		}
		a := &attempt{args: args, out: out, norm: helpdoc.Normalize(out), code: code, weak: optionError(out)}
		if a.norm == "" {
			continue
		}
		if len(r.Args) == 0 && !triedHelp && reOffersHelp.MatchString(out) {
			triedHelp = true
			tries = append(tries, []string{"help"})
		}
		if i := matchAncestor(a.norm, r.AncestorText); i >= 0 {
			if inherited == nil {
				inherited, inheritedIdx = a, i
			}
			continue
		}
		doc := helpdoc.Parse(out, path)
		if i := sameCommands(doc, r.Ancestors); i >= 0 {
			if inherited == nil {
				inherited, inheritedIdx = a, i
			}
			continue
		}
		if good(a) && !a.weak {
			return finish(ctx, r, a, doc)
		}
		if best == nil || better(a, best) {
			best = a
		}
	}
	if best != nil {
		return finish(ctx, r, best, helpdoc.Parse(best.out, path))
	}
	if inherited != nil {
		return Response{Source: source(r.Name, inherited.args), Inherited: inheritedIdx}
	}
	if len(r.Args) == 0 {
		if out, ok := man(ctx, r); ok {
			return Response{Doc: helpdoc.Parse(out, path), Source: "man " + r.Name, Inherited: -1}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s printed no help", source(r.Name, tries[0]))
	}
	return Response{Inherited: -1, Err: lastErr, Source: source(r.Name, tries[0])}
}

func finish(ctx context.Context, r Request, a *attempt, doc *helpdoc.Doc) Response {
	path := append([]string{r.Name}, r.Args...)
	resp := Response{Doc: doc, Source: source(r.Name, a.args), Inherited: -1}
	if len(r.Args) > 0 {
		return resp
	}
	// Short usage messages get the man page appended, when there is one.
	if nonBlankLines(a.out) < 8 {
		if out, ok := man(ctx, r); ok {
			resp.Doc = helpdoc.Parse(a.out+"\n\n"+out, path)
			resp.Source += " · man " + r.Name
		}
	}
	// kubectl lists its global flags in `kubectl options`.
	if regexp.MustCompile(`"` + regexp.QuoteMeta(r.Name) + ` options"`).MatchString(a.out) {
		if out, _, err := run(ctx, r, []string{"options"}); err == nil && strings.TrimSpace(out) != "" {
			resp.Global = helpdoc.Parse(out, path)
			resp.GlobalSource = source(r.Name, []string{"options"})
		}
	}
	return resp
}

func good(a *attempt) bool {
	n := nonBlankLines(a.out)
	return n >= 3 || (a.code == 0 && n >= 1)
}

func nonBlankLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// matchAncestor finds an ancestor whose help is the same as norm, allowing a
// short error line before or after the repeated help.
func matchAncestor(norm string, ancestors []string) int {
	for i, a := range ancestors {
		if a == "" {
			continue
		}
		if a == norm {
			return i
		}
		long, short := a, norm
		if len(short) > len(long) {
			long, short = short, long
		}
		if strings.Contains(long, short) && float64(len(short)) >= 0.8*float64(len(long)) {
			return i
		}
	}
	return -1
}

// sameCommands finds an ancestor that lists the same subcommands as doc,
// which means the tool ignored the subcommand and printed its general help.
func sameCommands(doc *helpdoc.Doc, ancestors []*helpdoc.Doc) int {
	if len(doc.Commands) == 0 {
		return -1
	}
	names := func(d *helpdoc.Doc) []string {
		var out []string
		for _, c := range d.Commands {
			out = append(out, c.Name)
		}
		slices.Sort(out)
		return out
	}
	mine := names(doc)
	for i, a := range ancestors {
		if a != nil && slices.Equal(mine, names(a)) {
			return i
		}
	}
	return -1
}

func source(name string, args []string) string {
	parts := []string{name}
	for _, a := range args {
		parts = append(parts, shell.Quote(a))
	}
	return strings.Join(parts, " ")
}

func man(ctx context.Context, r Request) (string, bool) {
	if runtime.GOOS == "windows" {
		return "", false
	}
	manPath, err := exec.LookPath("man")
	if err != nil {
		return "", false
	}
	out, code, err := runBin(ctx, r, manPath, []string{r.Name})
	if err != nil || code != 0 || nonBlankLines(out) < 3 {
		return "", false
	}
	return out, true
}

func run(ctx context.Context, r Request, args []string) (string, int, error) {
	return runBin(ctx, r, r.Bin, args)
}

const maxOutput = 2 << 20

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := maxOutput - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

func runBin(ctx context.Context, r Request, bin string, args []string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Stdin = nil
	cmd.Env = helpEnv(r.Width)
	cmd.WaitDelay = time.Second
	isolate(cmd)
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
		err = nil
		if ctx.Err() != nil {
			err = fmt.Errorf("%s timed out after %s", source(r.Name, args), r.Timeout)
		}
	case err != nil:
		if ctx.Err() != nil {
			err = fmt.Errorf("%s timed out after %s", source(r.Name, args), r.Timeout)
		}
		code = -1
	}
	return out.String(), code, err
}

// helpEnv keeps tools from paging, coloring or wrapping their help oddly.
func helpEnv(width int) []string {
	// Leave a little room: man pages overshoot MANWIDTH by a column or two.
	width = max(80, width-2)
	drop := map[string]bool{
		"CLICOLOR_FORCE": true, "FORCE_COLOR": true, "COLUMNS": true, "LINES": true,
		"PAGER": true, "MANPAGER": true, "GIT_PAGER": true, "SYSTEMD_PAGER": true,
		"TERM": true, "MANWIDTH": true, "LESS": true,
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[strings.ToUpper(k)] {
			env = append(env, kv)
		}
	}
	w := fmt.Sprint(width)
	return append(env,
		"NO_COLOR=1", "CLICOLOR=0", "TERM=dumb", "COLUMNS="+w, "MANWIDTH="+w,
		"PAGER=cat", "MANPAGER=cat", "GIT_PAGER=cat", "SYSTEMD_PAGER=cat", "SYSTEMD_COLORS=0",
	)
}

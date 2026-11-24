package tui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/rafalmaciejewski/ctlctl/internal/discover"
	"github.com/rafalmaciejewski/ctlctl/internal/helpdoc"
	"github.com/rafalmaciejewski/ctlctl/internal/history"
	"github.com/rafalmaciejewski/ctlctl/internal/loader"
)

// fakeLoad serves help from the parser's fixtures instead of running tools.
func fakeLoad(_ context.Context, r loader.Request) loader.Response {
	path := append([]string{r.Name}, r.Args...)
	file := strings.Join(path, "_")
	if r.Name == "systemctl" && len(r.Args) > 0 {
		return loader.Response{Inherited: len(r.Args) - 1, Source: "systemctl " + strings.Join(r.Args, " ") + " --help"}
	}
	b, err := os.ReadFile(filepath.Join("..", "helpdoc", "testdata", file+".txt"))
	if err != nil {
		return loader.Response{Inherited: -1, Err: err, Source: strings.Join(path, " ") + " --help"}
	}
	return loader.Response{Doc: helpdoc.Parse(string(b), path), Inherited: -1, Source: strings.Join(path, " ") + " --help"}
}

type harness struct {
	t *testing.T
	m *Model
}

func newHarness(t *testing.T, initial ...string) *harness {
	t.Helper()
	tools := []discover.Tool{
		{Name: "kubectl", Path: "/bin/kubectl"},
		{Name: "launchctl", Path: "/bin/launchctl"},
		{Name: "systemctl", Path: "/bin/systemctl"},
	}
	return newHarnessWith(t, tools, fakeLoad, initial...)
}

func newHarnessWith(t *testing.T, tools []discover.Tool, load loader.Func, initial ...string) *harness {
	t.Helper()
	spinInterval, statusDuration, debounceDelay = time.Millisecond, time.Millisecond, time.Millisecond
	m := New(Options{
		Tools:       tools,
		InitialPath: initial,
		History:     history.Open(""),
		Version:     "test",
		Output:      &bytes.Buffer{},
		Load:        load,
	})
	h := &harness{t: t, m: m}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 44})
	h.run(m.Init())
	return h
}

// run executes a command and feeds its messages back, skipping animation
// ticks so the test doesn't loop.
func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case spinMsg, statusClearMsg, nil:
	default:
		h.send(msg)
	}
}

func (h *harness) send(msg tea.Msg) {
	_, cmd := h.m.Update(msg)
	h.run(cmd)
}

func (h *harness) key(keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		h.send(msg)
	}
}

func (h *harness) screen() []string {
	return strings.Split(ansi.Strip(h.m.View()), "\n")
}

// find returns the screen position of the first occurrence of s at or below
// row minY and right of column minX.
func (h *harness) find(s string, minX int) (int, int) {
	h.t.Helper()
	for y, line := range h.screen() {
		rs := []rune(line)
		if minX >= len(rs) {
			continue
		}
		if i := strings.Index(string(rs[minX:]), s); i >= 0 {
			return minX + utf8.RuneCountInString(string(rs[minX:])[:i]), y
		}
	}
	h.t.Fatalf("%q not on screen:\n%s", s, strings.Join(h.screen(), "\n"))
	return 0, 0
}

// findInInfo scrolls the help pane with the mouse wheel until s is visible.
func (h *harness) findInInfo(s string) (int, int) {
	h.t.Helper()
	l := h.m.layout()
	h.m.infoScroll = 0
	for i := 0; i < 200; i++ {
		for y, line := range h.screen() {
			rs := []rune(line)
			if y == 0 || y >= l.info.h-1 || l.info.x >= len(rs) {
				continue
			}
			part := string(rs[l.info.x:])
			if j := strings.Index(part, s); j >= 0 {
				return l.info.x + utf8.RuneCountInString(part[:j]), y
			}
		}
		h.send(tea.MouseMsg{X: l.info.x + 5, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	h.t.Fatalf("%q not found in the help pane", s)
	return 0, 0
}

func (h *harness) click(x, y int) {
	h.send(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	h.send(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
}

func (h *harness) prompt() string { return string(h.m.input) }

func (h *harness) wantPrompt(want string) {
	h.t.Helper()
	if got := h.prompt(); got != want {
		h.t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func (h *harness) wantScreen(s string) {
	h.t.Helper()
	if !strings.Contains(strings.Join(h.screen(), "\n"), s) {
		h.t.Fatalf("%q not on screen:\n%s", s, strings.Join(h.screen(), "\n"))
	}
}

func TestBrowseAndRun(t *testing.T) {
	h := newHarness(t)
	h.wantPrompt("kubectl ")
	h.wantScreen("Basic Commands (Beginner):")
	h.wantScreen("❯ kubectl")

	// Enter opens kubectl and selects its first subcommand.
	h.key("enter")
	if h.m.sel.key() != "kubectl create" {
		t.Fatalf("selected %q", h.m.sel.key())
	}
	h.wantPrompt("kubectl create ")

	// Move to get; its help loads after the debounce.
	h.key("down", "down", "down", "down", "down")
	h.wantPrompt("kubectl get ")
	h.wantScreen("Display one or many resources.")
	h.wantScreen("kubectl get --help")

	// get has no subcommands, so Enter moves to the command line.
	h.key("enter")
	if h.m.focus != focusPrompt {
		t.Fatalf("focus = %d, want prompt", h.m.focus)
	}
	h.key("p", "o", "d", "s")
	h.wantPrompt("kubectl get pods")

	// Clicking a flag in the help adds it.
	x, y := h.findInInfo("--all-namespaces=false")
	h.click(x+3, y)
	h.wantPrompt("kubectl get pods --all-namespaces ")

	// A value flag leaves the cursor where the value goes.
	x, y = h.findInInfo("--output=''")
	h.click(x, y)
	h.wantPrompt("kubectl get pods --all-namespaces --output ")
	h.key("w", "i", "d", "e", " ")

	// Tab completes flags.
	h.key("-", "-", "w", "a", "t")
	if s, ok := h.m.suggestion(); !ok || s != "ch" {
		t.Fatalf("suggestion = %q %v", s, ok)
	}
	h.key("tab")
	h.wantPrompt("kubectl get pods --all-namespaces --output wide --watch ")

	h.key("enter")
	if got := h.m.Result().Command; got != "kubectl get pods --all-namespaces --output wide --watch" {
		t.Fatalf("result = %q", got)
	}
}

func TestClickExampleAndSubcommand(t *testing.T) {
	h := newHarness(t, "kubectl", "get")
	if h.m.sel.key() != "kubectl get" {
		t.Fatalf("initial path not opened: %q", h.m.sel.key())
	}
	x, y := h.findInInfo("kubectl get pods -o wide")
	h.click(x+2, y)
	h.wantPrompt("kubectl get pods -o wide")
	if h.m.focus != focusPrompt {
		t.Fatalf("focus = %d", h.m.focus)
	}

	// Clicking a subcommand in kubectl's help opens it in the tree.
	h.key("esc", "left")
	if h.m.sel.key() != "kubectl" {
		t.Fatalf("left did not go to the parent: %q", h.m.sel.key())
	}
	x, y = h.findInInfo("config          Modify")
	h.click(x+1, y)
	if h.m.sel.key() != "kubectl config" {
		t.Fatalf("selected %q", h.m.sel.key())
	}
	h.wantScreen("Available Commands:")
	h.wantScreen("use-context")
}

func TestInheritedHelp(t *testing.T) {
	h := newHarness(t, "systemctl", "start")
	if h.m.sel.key() != "systemctl start" {
		t.Fatalf("selected %q", h.m.sel.key())
	}
	h.wantScreen("systemctl start has no help of its own")
	if h.m.info.mark < 0 || !strings.Contains(h.m.info.lines[h.m.info.mark].Text, "Start (activate)") {
		t.Fatalf("mark = %d", h.m.info.mark)
	}
	h.key("e")
	if s, ok := h.m.suggestion(); ok || s != "UNIT..." {
		t.Fatalf("hint = %q %v", s, ok)
	}
}

func TestDraftsFollowSelection(t *testing.T) {
	h := newHarness(t)
	h.key("e", "x", "y", "esc", "down")
	h.wantPrompt("launchctl ")
	h.key("up")
	h.wantPrompt("kubectl xy")
}

func TestFilter(t *testing.T) {
	h := newHarness(t)
	h.key("/", "s", "y", "s")
	if len(h.m.rows) != 1 || h.m.rows[0].name != "systemctl" {
		var names []string
		for _, r := range h.m.rows {
			names = append(names, r.name)
		}
		t.Fatalf("rows = %v", names)
	}
	if h.m.sel.name != "systemctl" || h.m.sel.state != stateLoaded {
		t.Fatalf("selection = %q, state %d", h.m.sel.name, h.m.sel.state)
	}
	// Opening a matching tool shows its subcommands.
	h = newHarness(t)
	h.key("/kube", "enter", "enter")
	if h.m.sel.key() != "kubectl create" {
		t.Fatalf("filtered tool did not open: %q", h.m.sel.key())
	}
	h.key("esc")
	if h.m.filter != "" || len(h.m.rows) != 3+len(h.m.roots[0].children) {
		t.Fatalf("filter not cleared: %q, %d rows", h.m.filter, len(h.m.rows))
	}
}

func TestHistory(t *testing.T) {
	h := newHarness(t)
	_ = h.m.opts.History.Add("kubectl version --client")
	_ = h.m.opts.History.Add("launchctl list")
	h.key("e", "up")
	h.wantPrompt("kubectl version --client")
	h.key("down")
	h.wantPrompt("kubectl ")
}

func TestSmallTerminal(t *testing.T) {
	h := newHarness(t)
	h.send(tea.WindowSizeMsg{Width: 30, Height: 8})
	h.wantScreen("bigger terminal")
	h.send(tea.WindowSizeMsg{Width: 60, Height: 16})
	for i, l := range h.screen() {
		if w := ansi.StringWidth(l); w != 60 {
			t.Fatalf("line %d is %d wide: %q", i, w, l)
		}
	}
	if n := len(h.screen()); n != 16 {
		t.Fatalf("%d lines, want 16", n)
	}
}

func TestFrameFitsTerminal(t *testing.T) {
	h := newHarness(t, "kubectl", "get")
	for _, size := range [][2]int{{140, 44}, {80, 24}, {200, 60}} {
		h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(h.m.View(), "\n")
		if len(lines) != size[1] {
			t.Errorf("%v: %d lines", size, len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != size[0] {
				t.Errorf("%v: line %d is %d wide: %q", size, i, w, ansi.Strip(l))
			}
		}
	}
}

func TestInferredSubcommandsSettleWithoutLoading(t *testing.T) {
	var calls []string
	load := func(ctx context.Context, r loader.Request) loader.Response {
		calls = append(calls, strings.Join(append([]string{r.Name}, r.Args...), " "))
		return fakeLoad(ctx, r)
	}
	tools := []discover.Tool{{Name: "cpuctl", Path: "/bin/cpuctl"}, {Name: "kubectl", Path: "/bin/kubectl"}}
	h := newHarnessWith(t, tools, load, "cpuctl")

	// cpuctl's subcommands come from its usage line, so they show cpuctl's
	// help as soon as cpuctl's help is parsed: no loading state at all.
	root := h.m.roots[0]
	if len(root.children) != 3 {
		t.Fatalf("children = %d", len(root.children))
	}
	for _, c := range root.children {
		if c.state != stateLoaded || c.inherited != root {
			t.Fatalf("%s: state %d, inherited %v", c.name, c.state, c.inherited)
		}
		if g := h.m.glyph(c); g != "·" {
			t.Errorf("%s glyph = %q, want a leaf", c.name, g)
		}
	}
	// Selecting one shows the help with its line marked, synchronously:
	// the command returned by the selection isn't even run.
	h.m.selectNode(root.children[1])
	h.m.buildInfo()
	if h.m.info.mark < 0 || !strings.Contains(h.m.info.lines[h.m.info.mark].Text, "offline") {
		t.Fatalf("mark = %d", h.m.info.mark)
	}
	if strings.Contains(strings.ToLower(strings.Join(h.screen(), "\n")), "loading") {
		t.Fatal("the help pane showed a loading screen")
	}
	if len(calls) != 1 || calls[0] != "cpuctl" {
		t.Fatalf("loader calls = %v, want only cpuctl", calls)
	}
}

func TestLoadingShowsTheParentHelp(t *testing.T) {
	h := newHarness(t, "kubectl") // opens kubectl's subcommands
	expose := h.m.roots[0].children[1]
	// Select without letting the debounced load run.
	h.m.selectNode(expose)
	if expose.state == stateLoaded {
		t.Fatal("kubectl expose should still be loading")
	}
	h.wantScreen("kubectl expose ─")
	h.wantScreen(" loading ─╮")
	h.wantScreen("Basic Commands (Beginner):")
	if !strings.Contains(h.m.info.lines[h.m.info.mark].Text, "expose") {
		t.Fatalf("mark = %q", h.m.info.lines[h.m.info.mark].Text)
	}
}

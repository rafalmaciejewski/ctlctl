// Package tui is ctlctl's fullscreen interface: a tree of tools and
// subcommands, their help, and the command being built.
package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rafalmaciejewski/ctlctl/internal/discover"
	"github.com/rafalmaciejewski/ctlctl/internal/helpdoc"
	"github.com/rafalmaciejewski/ctlctl/internal/history"
	"github.com/rafalmaciejewski/ctlctl/internal/loader"
	"github.com/rafalmaciejewski/ctlctl/internal/runner"
)

type focus uint8

const (
	focusTree focus = iota
	focusInfo
	focusPrompt
)

// Options configures the interface.
type Options struct {
	Tools       []discover.Tool
	InitialPath []string // tool and subcommands to open at start
	History     *history.History
	Shell       string // configured shell, may be empty
	HelpTimeout time.Duration
	Version     string
	Output      io.Writer       // the terminal, for color detection
	Load        loader.Func     // defaults to loader.Load
	Context     context.Context // cancels help commands still running at exit
}

// Result is what the user chose.
type Result struct {
	Command string // empty when the user quit without running anything
}

// Model is the Bubble Tea model.
type Model struct {
	opts   Options
	st     *styles
	ctx    context.Context
	roots  []*node
	rows   []*node
	sel    *node
	focus  focus
	w, h   int
	result Result

	treeScroll int

	filtering bool
	filter    string

	info       infoView
	infoScroll int
	infoNode   *node     // the node whose help the pane shows
	infoState  loadState // and its state then
	hover      targetRef
	infoSel    int // index into info.targets, or -1

	input        []rune
	cur          int
	promptScroll int
	drafts       map[string]string
	histList     []string
	histPos      int
	histOrig     string

	autoPath []string
	loading  int
	frame    int
	seq      int
	status   string
	statusID int
	showHelp bool

	lastClick    time.Time
	lastClickRow int
}

type loadedMsg struct {
	n    *node
	resp loader.Response
}

type debounceMsg struct{ seq int }

type spinMsg struct{}

type statusClearMsg struct{ id int }

type execDoneMsg struct{ err error }

// New builds the model.
func New(o Options) *Model {
	if o.Load == nil {
		o.Load = loader.Load
	}
	if o.History == nil {
		o.History = history.Open("")
	}
	if o.Context == nil {
		o.Context = context.Background()
	}
	if o.Output == nil {
		o.Output = os.Stdout
	}
	r := lipgloss.NewRenderer(o.Output)
	m := &Model{
		opts:    o,
		st:      newStyles(r),
		ctx:     o.Context,
		drafts:  map[string]string{},
		infoSel: -1,
		hover:   noTarget,
		histPos: -1,
	}
	for _, t := range o.Tools {
		m.roots = append(m.roots, &node{name: t.Name, bin: t.Path})
	}
	m.rebuildRows()
	if len(o.InitialPath) > 0 {
		for _, n := range m.roots {
			if n.name == o.InitialPath[0] {
				m.sel = n
				m.autoPath = o.InitialPath[1:]
				n.pending = actExpand
			}
		}
	}
	if m.sel == nil && len(m.rows) > 0 {
		m.sel = m.rows[0]
	}
	if m.sel != nil {
		m.setInput(m.sel.key() + " ")
	}
	return m
}

// Result returns what the user chose once the program has exited.
func (m *Model) Result() Result { return m.result }

// Init does nothing: the first help loads once the terminal size is known,
// so tools and man pages can format it for the pane.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles a message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		first := m.w == 0
		m.w, m.h = msg.Width, msg.Height
		m.clampScroll()
		m.ensureTreeVisible()
		if first && m.sel != nil {
			return m, m.load(m.sel)
		}
		return m, nil
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	case tea.MouseMsg:
		return m, m.handleMouse(msg)
	case loadedMsg:
		return m, m.loaded(msg)
	case debounceMsg:
		if msg.seq == m.seq && m.sel != nil {
			return m, m.load(m.sel)
		}
	case spinMsg:
		m.frame++
		if m.loading > 0 {
			return m, spin()
		}
	case statusClearMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
	case execDoneMsg:
		if msg.err != nil {
			return m, m.flash("command failed: " + msg.err.Error())
		}
	}
	return m, nil
}

// Timings, shortened by tests.
var (
	spinInterval   = 90 * time.Millisecond
	statusDuration = 4 * time.Second
	debounceDelay  = 120 * time.Millisecond
)

func spin() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinMsg{} })
}

func (m *Model) flash(s string) tea.Cmd {
	m.statusID++
	id := m.statusID
	m.status = s
	return tea.Tick(statusDuration, func(time.Time) tea.Msg { return statusClearMsg{id} })
}

// load starts fetching a node's help.
func (m *Model) load(n *node) tea.Cmd {
	if n.state != stateIdle {
		return nil
	}
	n.state = stateLoading
	m.loading++
	req := loader.Request{
		Bin:     n.root().bin,
		Name:    n.root().name,
		Args:    n.path()[1:],
		Width:   m.infoWidth(),
		Timeout: m.opts.HelpTimeout,
		NoProbe: n.inferred,
	}
	for _, a := range n.ancestors() {
		d := a.helpDoc()
		req.Ancestors = append(req.Ancestors, d)
		text := ""
		if d != nil {
			var b strings.Builder
			for _, l := range d.Lines {
				b.WriteString(l.Text)
				b.WriteByte('\n')
			}
			text = helpdoc.Normalize(b.String())
		}
		req.AncestorText = append(req.AncestorText, text)
	}
	req.HelpCommand, req.HelpFlag, req.DashH = helpMechanisms(n)
	ctx, fn := m.ctx, m.opts.Load
	cmds := []tea.Cmd{func() tea.Msg { return loadedMsg{n, fn(ctx, req)} }}
	if m.loading == 1 {
		cmds = append(cmds, spin())
	}
	return tea.Batch(cmds...)
}

// helpMechanisms reports how n's ancestors say a subcommand's help can be
// requested: `tool help sub`, `tool sub --help` or `tool sub -h`.
func helpMechanisms(n *node) (helpCommand, helpFlag, dashH bool) {
	for _, a := range n.ancestors() {
		d := a.helpDoc()
		helpCommand = helpCommand || (a == n.parent && d.HelpCommand())
		helpFlag = helpFlag || d.HelpFlag()
		dashH = dashH || d.HelpDashH()
	}
	return helpCommand, helpFlag, dashH
}

// resolveChildren settles, right away, the subcommands that ctlctl won't run
// a help command for: they show n's help, so there's nothing to wait for and
// the help pane doesn't flash a loading screen when they're selected.
func resolveChildren(n *node) {
	if len(n.children) == 0 {
		return
	}
	helpCommand, helpFlag, dashH := helpMechanisms(n.children[0])
	for _, c := range n.children {
		req := loader.Request{Args: c.path()[1:], NoProbe: c.inferred, HelpCommand: helpCommand, HelpFlag: helpFlag, DashH: dashH}
		if !loader.Probes(req) {
			c.state, c.inherited = stateLoaded, n
		}
	}
}

func (m *Model) loaded(msg loadedMsg) tea.Cmd {
	n, r := msg.n, msg.resp
	m.loading--
	n.state = stateLoaded
	n.source = r.Source
	n.err = r.Err
	if r.Inherited >= 0 {
		if anc := n.ancestors(); r.Inherited < len(anc) {
			n.inherited = anc[r.Inherited]
		}
	} else {
		n.doc = r.Doc
	}
	if n.parent == nil {
		n.global, n.globalSource = r.Global, r.GlobalSource
	}
	if n.inherited == nil && n.doc != nil {
		n.setChildren(n.doc.Commands)
		resolveChildren(n)
	}

	var cmds []tea.Cmd
	pending := n.pending
	n.pending = actNone
	switch pending {
	case actActivate:
		cmds = append(cmds, m.activate(n))
	case actExpand:
		if len(n.children) > 0 {
			n.expanded = true
		}
	}
	m.rebuildRows()

	// Opening ctlctl with a path ("ctlctl kubectl config view") walks down it.
	if len(m.autoPath) > 0 && n == m.sel {
		if c := n.child(m.autoPath[0]); c != nil {
			m.autoPath = m.autoPath[1:]
			n.expanded = true
			m.rebuildRows()
			if len(m.autoPath) > 0 {
				c.pending = actExpand
			}
			cmds = append(cmds, m.selectNode(c), m.load(c))
		} else {
			cmds = append(cmds, m.flash(fmt.Sprintf("%s has no subcommand %q", n.key(), m.autoPath[0])))
			m.autoPath = nil
		}
	}
	return tea.Batch(cmds...)
}

// activate is Enter on a tree node: open its subcommands, or start editing
// the command when it has none.
func (m *Model) activate(n *node) tea.Cmd {
	if n.state != stateLoaded {
		n.pending = actActivate
		return m.load(n)
	}
	if len(n.children) > 0 {
		n.expanded = true
		m.rebuildRows()
		return m.selectNode(n.children[0])
	}
	if n != m.sel {
		cmd := m.selectNode(n)
		m.setFocus(focusPrompt)
		return cmd
	}
	m.setFocus(focusPrompt)
	return nil
}

// toggle expands or collapses a node.
func (m *Model) toggle(n *node) tea.Cmd {
	if n.state != stateLoaded {
		n.pending = actExpand
		return m.load(n)
	}
	if len(n.children) == 0 {
		return nil
	}
	n.expanded = !n.expanded
	if !n.expanded && m.sel != nil && isDescendant(m.sel, n) {
		cmd := m.selectNode(n)
		m.rebuildRows()
		return cmd
	}
	m.rebuildRows()
	return nil
}

func isDescendant(n, of *node) bool {
	for c := n.parent; c != nil; c = c.parent {
		if c == of {
			return true
		}
	}
	return false
}

// selectNode moves the tree cursor. The command line follows the selection;
// whatever was typed for the previous node is kept as its draft.
func (m *Model) selectNode(n *node) tea.Cmd {
	if n == nil || n == m.sel {
		return nil
	}
	if m.sel != nil {
		m.saveDraft()
	}
	m.sel = n
	if d, ok := m.drafts[n.key()]; ok {
		m.setInput(d)
	} else {
		m.setInput(n.key() + " ")
	}
	m.histPos = -1
	m.infoSel = -1
	m.hover = noTarget
	m.ensureTreeVisible()
	if n.state == stateIdle {
		m.seq++
		seq := m.seq
		return tea.Tick(debounceDelay, func(time.Time) tea.Msg { return debounceMsg{seq} })
	}
	return nil
}

func (m *Model) saveDraft() {
	key := m.sel.key()
	text := string(m.input)
	if strings.TrimSpace(text) == key {
		delete(m.drafts, key)
	} else {
		m.drafts[key] = text
	}
}

func (m *Model) setFocus(f focus) {
	m.focus = f
	if f == focusPrompt {
		m.filtering = false
	}
}

// runCommand finishes with the current command, or runs it and comes back.
func (m *Model) runCommand(stay bool) tea.Cmd {
	line := strings.TrimSpace(string(m.input))
	if line == "" {
		return m.flash("nothing to run")
	}
	_ = m.opts.History.Add(line)
	m.histPos = -1
	if stay {
		sh, flag := runner.Shell(m.opts.Shell)
		return tea.ExecProcess(runner.Pause(sh, flag, line), func(err error) tea.Msg { return execDoneMsg{err} })
	}
	m.result = Result{Command: line}
	return tea.Quit
}

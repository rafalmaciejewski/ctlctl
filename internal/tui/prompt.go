package tui

import (
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/rafalmaciejewski/ctlctl/internal/shell"
)

const (
	maxPromptRows = 5
	promptPad     = 3 // " ❯ " before the command
)

func (m *Model) setInput(s string) {
	m.input = []rune(s)
	m.cur = len(m.input)
	m.promptScroll = 0
}

func (m *Model) insert(rs []rune) {
	m.input = slices.Insert(m.input, m.cur, rs...)
	m.cur += len(rs)
	m.histPos = -1
}

// insertFlag adds a flag at the cursor, keeping words apart, and moves focus
// to the command line so a value can be typed right away.
func (m *Model) insertFlag(flag string, takesValue bool) tea.Cmd {
	m.setFocus(focusPrompt)
	if !takesValue && slices.Contains(shell.Fields(string(m.input)), flag) {
		return m.flash(flag + " is already in the command")
	}
	ins := []rune(flag + " ")
	if m.cur > 0 && m.input[m.cur-1] != ' ' {
		ins = append([]rune{' '}, ins...)
	}
	m.insert(ins)
	if takesValue && m.cur < len(m.input) && m.input[m.cur] != ' ' {
		m.input = slices.Insert(m.input, m.cur, ' ')
	}
	return nil
}

func isWordRune(r rune) bool { return r != ' ' && r != '\t' }

func (m *Model) wordLeft() int {
	i := m.cur
	for i > 0 && !isWordRune(m.input[i-1]) {
		i--
	}
	for i > 0 && isWordRune(m.input[i-1]) {
		i--
	}
	return i
}

func (m *Model) wordRight() int {
	i := m.cur
	for i < len(m.input) && !isWordRune(m.input[i]) {
		i++
	}
	for i < len(m.input) && isWordRune(m.input[i]) {
		i++
	}
	return i
}

func (m *Model) deleteRange(from, to int) {
	if from >= to {
		return
	}
	m.input = slices.Delete(m.input, from, to)
	m.cur = from
	m.histPos = -1
}

// handlePromptKey edits the command line. It reports whether it used the key.
func (m *Model) handlePromptKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "enter":
		return m.runCommand(false), true
	case "alt+enter", "ctrl+o":
		return m.runCommand(true), true
	case "esc":
		m.setFocus(focusTree)
	case "left", "ctrl+b":
		m.cur = max(0, m.cur-1)
	case "right", "ctrl+f":
		if m.cur == len(m.input) {
			if s, ok := m.suggestion(); ok {
				m.insert([]rune(s))
				break
			}
		}
		m.cur = min(len(m.input), m.cur+1)
	case "alt+left", "ctrl+left", "alt+b":
		m.cur = m.wordLeft()
	case "alt+right", "ctrl+right", "alt+f":
		m.cur = m.wordRight()
	case "home", "ctrl+a":
		m.cur = 0
	case "end", "ctrl+e":
		m.cur = len(m.input)
	case "backspace", "ctrl+h":
		if m.cur > 0 {
			m.deleteRange(m.cur-1, m.cur)
		}
	case "delete", "ctrl+d":
		if m.cur < len(m.input) {
			m.deleteRange(m.cur, m.cur+1)
		}
	case "ctrl+w", "alt+backspace":
		m.deleteRange(m.wordLeft(), m.cur)
	case "alt+d":
		end := m.wordRight()
		m.deleteRange(m.cur, end)
	case "ctrl+u":
		m.deleteRange(0, m.cur)
	case "ctrl+k":
		m.deleteRange(m.cur, len(m.input))
	case "up", "ctrl+p":
		m.historyStep(1)
	case "down", "ctrl+n":
		m.historyStep(-1)
	case "tab":
		if s, ok := m.suggestion(); ok {
			m.insert([]rune(s))
			if !strings.HasSuffix(s, "=") {
				m.insert([]rune{' '})
			}
			return nil, true
		}
		return nil, false
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			rs := make([]rune, 0, len(msg.Runes))
			for _, r := range msg.Runes {
				switch {
				case r == '\n' || r == '\r' || r == '\t':
					rs = append(rs, ' ')
				case r >= ' ':
					rs = append(rs, r)
				}
			}
			if msg.Alt {
				return nil, false
			}
			m.insert(rs)
			return nil, true
		}
		return nil, false
	}
	return nil, true
}

// historyStep walks the commands run before for the selected node.
func (m *Model) historyStep(dir int) {
	if m.histPos < 0 {
		if dir < 0 {
			return
		}
		m.histOrig = string(m.input)
		prefix := ""
		if m.sel != nil {
			prefix = m.sel.key() + " "
		}
		m.histList = m.opts.History.Matching(prefix)
		if len(m.histList) == 0 && m.sel != nil {
			m.histList = m.opts.History.Matching(m.sel.root().name + " ")
		}
	}
	next := m.histPos + dir
	switch {
	case next < 0:
		m.histPos = -1
		m.setInput(m.histOrig)
	case next < len(m.histList):
		m.histPos = next
		m.setInput(m.histList[next])
	}
}

// resolver tells the highlighter about the commands in the tree.
type resolver struct{ m *Model }

func (r resolver) node(path []string) *node {
	if len(path) == 0 {
		return nil
	}
	n := r.m.findRoot(path[0])
	for _, w := range path[1:] {
		if n == nil {
			return nil
		}
		n = n.child(w)
	}
	return n
}

func (r resolver) Subcommand(path []string, word string) bool {
	n := r.node(path)
	return n != nil && n.child(word) != nil
}

func (r resolver) FlagTakesValue(path []string, flag string) bool {
	n := r.node(path)
	if n == nil {
		return false
	}
	f := n.findFlag(flag)
	return f != nil && f.TakesValue()
}

func (m *Model) findRoot(name string) *node {
	name = filepath.Base(name)
	for _, n := range m.roots {
		if n.name == name {
			return n
		}
	}
	return nil
}

// resolve follows the words of a command through the tree. It returns the
// deepest subcommand reached and how many positional arguments follow it.
func (m *Model) resolve(words []string) (*node, int) {
	if len(words) == 0 {
		return nil, 0
	}
	n := m.findRoot(words[0])
	if n == nil {
		return nil, 0
	}
	args := 0
	skip := false
	for _, w := range words[1:] {
		switch {
		case skip:
			skip = false
		case strings.HasPrefix(w, "-") && len(w) > 1:
			if !strings.Contains(w, "=") {
				if f := n.findFlag(w); f != nil && f.TakesValue() {
					skip = true
				}
			}
		case args == 0 && n.child(w) != nil:
			n = n.child(w)
		default:
			args++
		}
	}
	return n, args
}

// suggestion returns the text shown after the cursor: a completion that Tab
// accepts (ok is true), or a hint about what comes next.
func (m *Model) suggestion() (string, bool) {
	if m.focus != focusPrompt || m.cur != len(m.input) {
		return "", false
	}
	s := string(m.input)
	cut := strings.LastIndexAny(s, " \t") + 1
	word, before := s[cut:], s[:cut]
	if strings.ContainsAny(before, "|;&<>") || strings.Count(before, "'")%2 == 1 || strings.Count(before, `"`)%2 == 1 {
		return "", false
	}
	n, args := m.resolve(shell.Fields(before))
	if n == nil {
		return "", false
	}
	switch {
	case word == "":
		if strings.TrimSpace(s) == n.key() {
			return m.usageHint(n), false
		}
	case strings.HasPrefix(word, "-") && len(word) >= 2:
		if strings.Contains(word, "=") {
			return "", false
		}
		for _, f := range n.flags() {
			for _, name := range f.Names {
				if len(name) > len(word) && strings.HasPrefix(name, word) {
					return name[len(word):], true
				}
			}
		}
	case args == 0:
		for _, c := range n.children {
			if len(c.name) > len(word) && strings.HasPrefix(c.name, word) {
				return c.name[len(word):], true
			}
		}
	}
	return "", false
}

// usageHint describes the arguments a command takes, from its usage line or
// its entry in the parent's command list.
func (m *Model) usageHint(n *node) string {
	path := n.path()
	if d := n.helpDoc(); d != nil && n.inherited == nil {
		for _, u := range d.Usage {
			f := strings.Fields(u)
			if len(f) <= len(path) || filepath.Base(f[0]) != path[0] {
				continue
			}
			if !slices.Equal(f[1:len(path)], path[1:]) {
				continue
			}
			return strings.Join(f[len(path):], " ")
		}
	}
	if n.parent != nil {
		if c := n.parent.helpDoc().FindCommand(n.name); c != nil && c.Args != "" {
			return c.Args
		}
	}
	if len(n.children) > 0 {
		return "<command>"
	}
	return ""
}

type promptCell struct {
	r      rune
	class  shell.Class
	ghost  bool
	hint   bool
	cursor bool
}

// promptCells lays the command line, cursor and suggestion out as cells.
func (m *Model) promptCells() ([]promptCell, int) {
	classes := shell.Classes(string(m.input), resolver{m})
	cells := make([]promptCell, 0, len(m.input)+32)
	for i, r := range m.input {
		cells = append(cells, promptCell{r: r, class: classes[i]})
	}
	cursor := m.cur
	if m.focus != focusPrompt {
		return cells, -1
	}
	sugg, ok := m.suggestion()
	if m.cur < len(cells) {
		cells[m.cur].cursor = true
		return cells, cursor
	}
	gr := []rune(sugg)
	if len(gr) == 0 {
		cells = append(cells, promptCell{r: ' ', cursor: true})
		return cells, cursor
	}
	for i, r := range gr {
		cells = append(cells, promptCell{r: r, ghost: ok, hint: !ok, cursor: i == 0})
	}
	return cells, cursor
}

// promptRowCount is how many rows the command line needs at a width.
func (m *Model) promptRowCount(width int) int {
	if width <= 0 {
		return 1
	}
	w := strWidth(m.input) + 1
	return clamp((w+width-1)/width, 1, maxPromptRows)
}

func (m *Model) renderPrompt(r rect) []string {
	in := r.inner()
	pw := max(1, in.w-promptPad)
	cells, cursor := m.promptCells()

	var rows [][]promptCell
	var row []promptCell
	w := 0
	cursorRow := 0
	for i, c := range cells {
		cw := max(1, runewidth.RuneWidth(c.r))
		if w+cw > pw {
			rows = append(rows, row)
			row, w = nil, 0
		}
		if i == cursor {
			cursorRow = len(rows)
		}
		row = append(row, c)
		w += cw
	}
	rows = append(rows, row)
	// Hints and completions never push the text onto more rows.
	if len(rows) > in.h {
		if cursorRow < m.promptScroll {
			m.promptScroll = cursorRow
		}
		if cursorRow >= m.promptScroll+in.h {
			m.promptScroll = cursorRow - in.h + 1
		}
	} else {
		m.promptScroll = 0
	}

	focused := m.focus == focusPrompt
	body := make([]string, in.h)
	for i := range body {
		ri := m.promptScroll + i
		prefix := "   "
		if ri == 0 {
			prefix = " ❯ "
		}
		ps := m.st.muted
		if focused {
			ps = m.st.accent.Bold(true)
		}
		line := ps.Render(prefix)
		used := promptPad
		if ri < len(rows) {
			line += m.renderCells(rows[ri])
			for _, c := range rows[ri] {
				used += max(1, runewidth.RuneWidth(c.r))
			}
		}
		if used < in.w {
			line += strings.Repeat(" ", in.w-used)
		}
		body[i] = line
	}
	right := "⏎ run · alt+⏎ run & return"
	if !focused {
		right = "e or click to edit"
	}
	return m.box(r, "Command", right, focused, body, scrollbar{})
}

func (m *Model) renderCells(cells []promptCell) string {
	var b strings.Builder
	for i := 0; i < len(cells); {
		c := cells[i]
		j := i + 1
		for j < len(cells) && cells[j].class == c.class && cells[j].ghost == c.ghost && cells[j].hint == c.hint && cells[j].cursor == c.cursor {
			j++
		}
		st := m.st.class[c.class]
		switch {
		case c.ghost:
			st = m.st.ghost
		case c.hint:
			st = m.st.hint
		}
		if c.cursor {
			st = st.Reverse(true)
		}
		var s strings.Builder
		for _, x := range cells[i:j] {
			s.WriteRune(x.r)
		}
		b.WriteString(st.Render(s.String()))
		i = j
	}
	return b.String()
}

// promptIndex maps a click in the command line to a cursor position.
func (m *Model) promptIndex(x, y int) int {
	in := m.layout().prompt.inner()
	pw := max(1, in.w-promptPad)
	row := y - in.y + m.promptScroll
	col := max(0, x-in.x-promptPad)
	idx, w, r := 0, 0, 0
	for idx < len(m.input) {
		cw := max(1, runewidth.RuneWidth(m.input[idx]))
		if w+cw > pw {
			r++
			w = 0
		}
		if r == row && w+cw > col {
			return idx
		}
		if r > row {
			return idx
		}
		w += cw
		idx++
	}
	return len(m.input)
}

// HighlightCommand renders a command line with the same colors as the
// command pane, for echoing it before it runs.
func (m *Model) HighlightCommand(line string) string {
	classes := shell.Classes(line, resolver{m})
	cells := make([]promptCell, 0, len(classes))
	for i, r := range []rune(line) {
		cells = append(cells, promptCell{r: r, class: classes[i]})
	}
	return m.st.accent.Bold(true).Render("❯ ") + m.renderCells(cells)
}

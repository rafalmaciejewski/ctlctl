package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/rafalmaciejewski/ctlctl/internal/helpdoc"
)

type targetRef struct{ line, idx int }

var noTarget = targetRef{-1, -1}

type infoRow struct{ line, start, end, prefix int }

// infoView is the help pane's content, wrapped to the pane's width.
type infoView struct {
	key     string
	lines   []helpdoc.Line
	runes   [][]rune
	owners  []*node // the node whose help each line belongs to
	mark    int     // line highlighted for a subcommand without its own help
	rows    []infoRow
	kinds   [][]helpdoc.Kind
	tmap    [][]int16
	targets []targetRef
}

func (m *Model) infoWidth() int {
	l := m.layout()
	if !l.ok {
		return 100
	}
	return max(10, l.info.w-4)
}

func textLine(text string, kind ...helpdoc.Kind) helpdoc.Line {
	l := helpdoc.Line{Text: text}
	if len(kind) > 0 && kind[0] != helpdoc.KindText && text != "" {
		l.Styles = []helpdoc.Span{{Start: 0, End: len([]rune(text)), Kind: kind[0]}}
	}
	return l
}

// buildInfo recomputes the help pane's content when what it shows changed.
func (m *Model) buildInfo() {
	n := m.sel
	width := m.infoWidth()
	state := loadState(0)
	if n != nil {
		state = n.state
	}
	key := fmt.Sprintf("%p/%d/%d/%v", n, state, width, m.showHelp)
	if state == stateLoading {
		key += fmt.Sprint("/", m.frame)
	}
	if key == m.info.key {
		return
	}
	v := infoView{key: key, mark: -1}
	add := func(owner *node, ls ...helpdoc.Line) {
		for _, l := range ls {
			v.lines = append(v.lines, l)
			v.owners = append(v.owners, owner)
		}
	}

	switch {
	case m.showHelp:
		add(nil, keysHelp()...)
	case n == nil:
		add(nil,
			textLine("No tools found.", helpdoc.KindHeader),
			textLine(""),
			textLine("ctlctl lists the executables on your PATH whose names end with \"ctl\"."),
			textLine("Add other tools with `ctlctl <tool>` or in the config file:"),
			textLine(""),
		)
		for _, l := range strings.Split(configExample, "\n") {
			k := helpdoc.KindText
			if strings.HasPrefix(l, "#") {
				k = helpdoc.KindComment
			}
			add(nil, textLine("  "+l, k))
		}
	case n.state != stateLoaded && n.parent != nil && n.parent.helpDoc() != nil:
		// Until its own help arrives, show the parent's at the subcommand's
		// line, so the pane doesn't go blank while moving through the tree.
		// The title shows that it's loading.
		anc := n.parent.docNode()
		add(anc, anc.doc.Lines...)
		if c := anc.doc.FindCommand(n.name); c != nil {
			v.mark = c.Line
		}
	case n.state != stateLoaded:
		if n.desc != "" {
			add(nil, textLine(n.desc, helpdoc.KindText), textLine(""))
		}
		add(nil, textLine(spinnerFrames[m.frame%len(spinnerFrames)]+" Loading the help of "+n.key()+"…", helpdoc.KindComment))
	case n.inherited != nil:
		anc := n.inherited
		add(nil,
			textLine(fmt.Sprintf("%s has no help of its own. This is the help of %s.", n.key(), anc.key()), helpdoc.KindComment),
			textLine(""))
		off := len(v.lines)
		add(anc, anc.doc.Lines...)
		if c := anc.doc.FindCommand(n.name); c != nil {
			v.mark = off + c.Line
		}
	case n.doc == nil || len(n.doc.Lines) == 0:
		msg := n.source + " printed nothing."
		if n.err != nil {
			msg = n.err.Error()
		}
		add(nil, textLine("Couldn't get help", helpdoc.KindHeader), textLine(""), textLine(msg, helpdoc.KindComment))
		if len(n.children) == 0 {
			add(nil, textLine(""), textLine("You can still type arguments in the command line below.", helpdoc.KindComment))
		}
	default:
		add(n, n.doc.Lines...)
	}

	if !m.showHelp && n != nil && n.state == stateLoaded {
		if root := n.root(); root.global != nil {
			add(nil, textLine(""), textLine("Global options ("+root.globalSource+"):", helpdoc.KindHeader), textLine(""))
			add(root, root.global.Lines...)
		}
	}

	v.runes = make([][]rune, len(v.lines))
	v.kinds = make([][]helpdoc.Kind, len(v.lines))
	v.tmap = make([][]int16, len(v.lines))
	for i, l := range v.lines {
		rs := []rune(l.Text)
		v.runes[i] = rs
		kinds := make([]helpdoc.Kind, len(rs))
		for _, s := range l.Styles {
			for j := max(s.Start, 0); j < s.End && j < len(rs); j++ {
				kinds[j] = s.Kind
			}
		}
		v.kinds[i] = kinds
		tm := make([]int16, len(rs))
		for j := range tm {
			tm[j] = -1
		}
		for ti, t := range l.Targets {
			v.targets = append(v.targets, targetRef{i, ti})
			for j := max(t.Start, 0); j < t.End && j < len(rs); j++ {
				if tm[j] < 0 {
					tm[j] = int16(ti)
				}
			}
		}
		v.tmap[i] = tm
		for _, r := range wrapRunes(rs, width) {
			r.line = i
			v.rows = append(v.rows, r)
		}
	}
	m.info = v

	if m.infoNode != n || m.infoState != state {
		m.infoNode, m.infoState = n, state
		m.infoScroll = 0
		m.infoSel = -1
		m.hover = noTarget
		if v.mark >= 0 {
			for ri, r := range v.rows {
				if r.line == v.mark {
					m.infoScroll = max(0, ri-2)
					break
				}
			}
		}
	}
	m.clampScroll()
}

// wrapRunes breaks a line into rows no wider than width. Continuation rows
// are indented to the line's description column when it has one.
func wrapRunes(rs []rune, width int) []infoRow {
	if width <= 0 || strWidth(rs) <= width {
		return []infoRow{{start: 0, end: len(rs)}}
	}
	indent := 0
	for indent < len(rs) && rs[indent] == ' ' {
		indent++
	}
	hang := indent + 2
	for i := indent; i+1 < len(rs); i++ {
		if rs[i] == ' ' && rs[i+1] == ' ' {
			j := i
			for j < len(rs) && rs[j] == ' ' {
				j++
			}
			if j < width/2 {
				hang = j
			}
			break
		}
	}
	if hang > width/2 {
		hang = min(indent, width/2)
	}
	var rows []infoRow
	start, prefix := 0, 0
	for start < len(rs) {
		avail := width - prefix
		end, w := start, 0
		for end < len(rs) {
			rw := runewidth.RuneWidth(rs[end])
			if w+rw > avail {
				break
			}
			w += rw
			end++
		}
		if end < len(rs) {
			for k := end; k > start+avail/3; k-- {
				if rs[k] == ' ' {
					end = k
					break
				}
			}
		}
		if end == start {
			end = start + 1
		}
		rows = append(rows, infoRow{start: start, end: end, prefix: prefix})
		start = end
		for start < len(rs) && rs[start] == ' ' {
			start++
		}
		prefix = hang
	}
	return rows
}

func strWidth(rs []rune) int {
	w := 0
	for _, r := range rs {
		w += runewidth.RuneWidth(r)
	}
	return w
}

func (m *Model) clampScroll() {
	l := m.layout()
	if !l.ok {
		return
	}
	h := l.info.h - 2
	m.infoScroll = clamp(m.infoScroll, 0, max(0, len(m.info.rows)-h))
	th := l.tree.h - 2
	m.treeScroll = clamp(m.treeScroll, 0, max(0, len(m.rows)-th))
}

func (m *Model) renderInfo(r rect) []string {
	m.buildInfo()
	in := r.inner()
	width := in.w - 2
	body := make([]string, in.h)
	for i := range body {
		ri := m.infoScroll + i
		if ri >= len(m.info.rows) {
			body[i] = strings.Repeat(" ", in.w)
			continue
		}
		body[i] = " " + m.renderInfoRow(m.info.rows[ri], width) + " "
	}
	title, right := "Help", ""
	switch {
	case m.showHelp:
		title = "Keys"
	case m.sel != nil:
		title = m.sel.key()
		if m.sel.state == stateLoaded {
			right = m.sel.docNode().source
		} else {
			right = spinnerFrames[m.frame%len(spinnerFrames)] + " loading"
		}
	}
	return m.box(r, title, right, m.focus == focusInfo, body, scrollbar{total: len(m.info.rows), visible: in.h, offset: m.infoScroll})
}

func (m *Model) renderInfoRow(row infoRow, width int) string {
	v := &m.info
	rs := v.runes[row.line]
	kinds := v.kinds[row.line]
	tm := v.tmap[row.line]
	marked := row.line == v.mark
	var b strings.Builder
	pad := m.st.text
	if marked {
		pad = pad.Background(m.st.p.markBg)
	}
	used := row.prefix
	b.WriteString(pad.Render(strings.Repeat(" ", row.prefix)))
	selected := noTarget
	if m.infoSel >= 0 && m.infoSel < len(v.targets) {
		selected = v.targets[m.infoSel]
	}
	for i := row.start; i < row.end; {
		k, t := kinds[i], tm[i]
		j := i + 1
		for j < row.end && kinds[j] == k && tm[j] == t {
			j++
		}
		st := m.st.kind(k)
		if t >= 0 {
			ref := targetRef{row.line, int(t)}
			if ref == m.hover {
				st = st.Underline(true).Bold(true)
			}
			if ref == selected {
				st = st.Reverse(true)
			}
		}
		if marked {
			st = st.Background(m.st.p.markBg)
		}
		seg := rs[i:j]
		used += strWidth(seg)
		b.WriteString(st.Render(string(seg)))
		i = j
	}
	if used < width {
		b.WriteString(pad.Render(strings.Repeat(" ", width-used)))
	}
	return b.String()
}

// infoHit finds the click target under a screen position.
func (m *Model) infoHit(x, y int) (targetRef, bool) {
	l := m.layout()
	in := l.info.inner()
	if !in.contains(x, y) {
		return noTarget, false
	}
	ri := m.infoScroll + y - in.y
	if ri < 0 || ri >= len(m.info.rows) {
		return noTarget, false
	}
	row := m.info.rows[ri]
	col := x - in.x - 1 - row.prefix
	if col < 0 {
		return noTarget, false
	}
	rs := m.info.runes[row.line]
	idx, w := row.start, 0
	for idx < row.end {
		rw := runewidth.RuneWidth(rs[idx])
		if w+rw > col {
			break
		}
		w += rw
		idx++
	}
	if idx >= row.end {
		return noTarget, false
	}
	t := m.info.tmap[row.line][idx]
	if t < 0 {
		return noTarget, false
	}
	return targetRef{row.line, int(t)}, true
}

// useTarget acts on a clicked or selected part of the help.
func (m *Model) useTarget(ref targetRef) tea.Cmd {
	if ref.line < 0 || ref.line >= len(m.info.lines) {
		return nil
	}
	t := m.info.lines[ref.line].Targets[ref.idx].Target
	switch t.Kind {
	case helpdoc.TargetFlag:
		return m.insertFlag(t.Value, t.TakesValue)
	case helpdoc.TargetExample:
		m.setInput(t.Value)
		m.setFocus(focusPrompt)
		return m.flash("example copied to the command line")
	case helpdoc.TargetCommand:
		owner := m.info.owners[ref.line]
		if owner == nil {
			return nil
		}
		c := owner.child(t.Value)
		if c == nil {
			return m.flash(fmt.Sprintf("%s has no subcommand %q", owner.key(), t.Value))
		}
		for a := owner; a != nil; a = a.parent {
			a.expanded = true
		}
		m.rebuildRows()
		m.setFocus(focusTree)
		return tea.Batch(m.selectNode(c), m.load(c))
	}
	return nil
}

// moveTarget selects the next or previous clickable part of the help.
func (m *Model) moveTarget(delta int) {
	n := len(m.info.targets)
	if n == 0 {
		return
	}
	if m.infoSel < 0 {
		// Start from the first target on screen.
		first := m.infoScroll
		for i, t := range m.info.targets {
			if m.targetRow(t) >= first {
				m.infoSel = i
				if delta < 0 {
					m.infoSel = (i - 1 + n) % n
				}
				break
			}
		}
		if m.infoSel < 0 {
			m.infoSel = 0
		}
	} else {
		m.infoSel = (m.infoSel + delta + n) % n
	}
	row := m.targetRow(m.info.targets[m.infoSel])
	h := m.layout().info.h - 2
	if row < m.infoScroll {
		m.infoScroll = row
	} else if row >= m.infoScroll+h {
		m.infoScroll = row - h + 1
	}
	m.clampScroll()
}

func (m *Model) targetRow(t targetRef) int {
	start := m.info.lines[t.line].Targets[t.idx].Start
	for ri, r := range m.info.rows {
		if r.line == t.line && start >= r.start && (start < r.end || r.end == r.start) {
			return ri
		}
		if r.line > t.line {
			return ri
		}
	}
	return 0
}

func keysHelp() []helpdoc.Line {
	type kv struct{ k, v string }
	groups := []struct {
		title string
		keys  []kv
	}{
		{"Everywhere", []kv{
			{"tab / shift+tab", "move focus between panes"},
			{"mouse click", "focus a pane, select a command, add a flag, use an example"},
			{"mouse wheel", "scroll the pane under the pointer"},
			{"?", "show or hide this help"},
			{"ctrl+c", "quit without running anything"},
		}},
		{"Tools", []kv{
			{"↑ ↓  k j", "move"},
			{"enter", "open the subcommands, or edit the command when there are none"},
			{"→ l / ← h", "expand / collapse or go to the parent"},
			{"space", "expand or collapse"},
			{"e", "edit the command for the selected node"},
			{"/", "filter by name"},
			{"q", "quit"},
		}},
		{"Help", []kv{
			{"↑ ↓  pgup pgdn", "scroll"},
			{"n / N", "select the next / previous flag, command or example"},
			{"enter", "use the selected item"},
		}},
		{"Command", []kv{
			{"enter", "run the command and exit"},
			{"alt+enter", "run the command and come back"},
			{"tab", "accept the suggestion"},
			{"↑ ↓", "history for this command"},
			{"ctrl+a / ctrl+e", "start / end of line"},
			{"ctrl+w / ctrl+u / ctrl+k", "delete word / to start / to end"},
			{"esc", "back to the tools"},
		}},
	}
	var out []helpdoc.Line
	for gi, g := range groups {
		if gi > 0 {
			out = append(out, textLine("", helpdoc.KindText))
		}
		out = append(out, textLine(g.title, helpdoc.KindHeader))
		for _, k := range g.keys {
			text := fmt.Sprintf("  %-26s %s", k.k, k.v)
			out = append(out, helpdoc.Line{Text: text, Styles: []helpdoc.Span{{Start: 2, End: 2 + len([]rune(k.k)), Kind: helpdoc.KindFlag}}})
		}
	}
	return out
}

const configExample = `# ~/.config/ctlctl/config.toml
suffixes = ["ctl"]
include = ["docker", "helm"]
exclude = ["sysctl"]`

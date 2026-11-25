package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// rebuildRows flattens the visible part of the tree.
func (m *Model) rebuildRows() {
	m.rows = m.rows[:0]
	f := strings.ToLower(m.filter)
	// With a filter, a matching node shows its subtree as usual, and a
	// node with matching descendants opens to show them.
	var walk func(ns []*node, inMatch bool)
	walk = func(ns []*node, inMatch bool) {
		for _, n := range ns {
			matched := inMatch || f == "" || strings.Contains(strings.ToLower(n.name), f)
			below := !matched && descendantMatches(n, f)
			if !matched && !below {
				continue
			}
			m.rows = append(m.rows, n)
			if n.expanded || below {
				walk(n.children, matched)
			}
		}
	}
	walk(m.roots, false)
	if m.sel != nil && m.rowIndex(m.sel) < 0 && len(m.rows) > 0 && f != "" {
		m.selectNode(m.rows[0])
	}
	m.ensureTreeVisible()
}

func matchesTree(n *node, f string) bool {
	return strings.Contains(strings.ToLower(n.name), f) || descendantMatches(n, f)
}

func descendantMatches(n *node, f string) bool {
	for _, c := range n.children {
		if matchesTree(c, f) {
			return true
		}
	}
	return false
}

func (m *Model) rowIndex(n *node) int {
	for i, r := range m.rows {
		if r == n {
			return i
		}
	}
	return -1
}

func (m *Model) ensureTreeVisible() {
	h := m.layout().tree.h - 2
	if h <= 0 {
		return
	}
	i := m.rowIndex(m.sel)
	if i < 0 {
		return
	}
	if i < m.treeScroll {
		m.treeScroll = i
	}
	if i >= m.treeScroll+h {
		m.treeScroll = i - h + 1
	}
	m.treeScroll = clamp(m.treeScroll, 0, max(0, len(m.rows)-h))
}

func (m *Model) moveTree(delta int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	i := m.rowIndex(m.sel)
	if i < 0 {
		i = 0
	}
	return m.selectNode(m.rows[clamp(i+delta, 0, len(m.rows)-1)])
}

// glyph returns the expander shown before a node's name.
func (m *Model) glyph(n *node) string {
	switch {
	case n.state == stateLoaded && len(n.children) == 0:
		return "·"
	case n.expanded:
		return "▾"
	}
	return "▸"
}

// busy reports whether n's help is on its way: loading, or selected and
// about to load once the cursor rests on it.
func (m *Model) busy(n *node) bool {
	return n.state == stateLoading || (n == m.sel && n.state == stateIdle)
}

func (m *Model) spinner() string { return spinnerFrames[m.frame%len(spinnerFrames)] }

func (m *Model) renderTree(r rect) []string {
	in := r.inner()
	body := make([]string, in.h)
	focused := m.focus == focusTree
	f := strings.ToLower(m.filter)
	for i := range body {
		ri := m.treeScroll + i
		if ri >= len(m.rows) {
			body[i] = strings.Repeat(" ", in.w)
			continue
		}
		n := m.rows[ri]
		selected := n == m.sel
		base := m.st.text
		glyphStyle := m.st.muted
		if n.parent == nil {
			base = base.Bold(true)
		}
		if n.state == stateLoaded && n.err != nil && n.doc == nil {
			base = m.st.err
		}
		if selected {
			bg := m.st.p.selBgBlur
			if focused {
				bg = m.st.p.selBg
			}
			base = base.Background(bg)
			glyphStyle = glyphStyle.Background(bg)
			if focused {
				base = base.Bold(true).Foreground(m.st.p.accent)
				glyphStyle = glyphStyle.Foreground(m.st.p.accent)
			}
		}
		var b strings.Builder
		b.WriteString(base.Render(" " + strings.Repeat("  ", n.depth)))
		b.WriteString(glyphStyle.Render(m.glyph(n)))
		b.WriteString(base.Render(" "))
		b.WriteString(highlightMatch(n.name, f, base, base.Inherit(m.st.match)))
		line := b.String()
		// The loading spinner goes after the name and survives truncation.
		suffix := ""
		if m.busy(n) && in.w > 6 {
			suffix = " " + m.spinner()
		}
		avail := in.w - lipgloss.Width(suffix)
		if w := lipgloss.Width(line); w > avail {
			line = ansi.Truncate(line, avail-1, "") + base.Render("…")
		}
		line += glyphStyle.Render(suffix)
		if w := lipgloss.Width(line); w < in.w {
			line += base.Render(strings.Repeat(" ", in.w-w))
		}
		body[i] = line
	}
	if len(m.roots) == 0 && in.h > 2 {
		body[0] = padRight(m.st.muted.Render(" no tools found"), in.w)
	}
	title := "Tools"
	if m.filtering || m.filter != "" {
		cursor := ""
		if m.filtering {
			cursor = "▏"
		}
		title = "/" + m.filter + cursor
	}
	return m.box(r, title, "", focused, body, scrollbar{total: len(m.rows), visible: in.h, offset: m.treeScroll})
}

func highlightMatch(name, f string, base, hl lipgloss.Style) string {
	if f == "" {
		return base.Render(name)
	}
	i := strings.Index(strings.ToLower(name), f)
	if i < 0 {
		return base.Render(name)
	}
	return base.Render(name[:i]) + hl.Render(name[i:i+len(f)]) + base.Render(name[i+len(f):])
}

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	minWidth  = 40
	minHeight = 12
)

type rect struct{ x, y, w, h int }

func (r rect) inner() rect { return rect{r.x + 1, r.y + 1, r.w - 2, r.h - 2} }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

type layout struct {
	tree, info, prompt rect
	status             int
	ok                 bool
}

func (m *Model) layout() layout {
	if m.w < minWidth || m.h < minHeight {
		return layout{}
	}
	side := clamp(m.w*27/100, 20, 40)
	ph := m.promptRowCount(m.w-2-promptPad) + 2
	mainH := m.h - ph - 1
	return layout{
		tree:   rect{0, 0, side, mainH},
		info:   rect{side, 0, m.w - side, mainH},
		prompt: rect{0, mainH, m.w, ph},
		status: m.h - 1,
		ok:     true,
	}
}

type scrollbar struct{ total, visible, offset int }

// box draws a rounded border with a title around body, whose lines must be
// exactly the inner width. A scrollbar replaces the right border when the
// content doesn't fit.
func (m *Model) box(r rect, title, right string, focused bool, body []string, sb scrollbar) []string {
	bs, ts := m.st.border, m.st.title
	if focused {
		bs, ts = m.st.borderFocus, m.st.titleFocus
	}
	t := ""
	if title != "" {
		t = " " + title + " "
	}
	rt := ""
	if right != "" {
		rt = " " + right + " "
	}
	tw, rw := ansi.StringWidth(t), ansi.StringWidth(rt)
	if r.w-4-tw-rw < 0 {
		rt, rw = "", 0
	}
	if r.w-4-tw < 0 {
		t = ansi.Truncate(t, max(0, r.w-5), "…")
		tw = ansi.StringWidth(t)
	}
	fill := max(0, r.w-4-tw-rw)
	top := bs.Render("╭─") + ts.Render(t) + bs.Render(strings.Repeat("─", fill)) + m.st.muted.Render(rt) + bs.Render("─╮")

	thumbStart, thumbEnd := -1, -1
	if sb.visible > 0 && sb.total > sb.visible {
		size := max(1, sb.visible*sb.visible/sb.total)
		pos := 0
		if sb.total-sb.visible > 0 {
			pos = sb.offset * (sb.visible - size) / (sb.total - sb.visible)
		}
		thumbStart, thumbEnd = pos, pos+size
	}
	thumb := m.st.muted
	if focused {
		thumb = m.st.accent
	}

	out := make([]string, 0, r.h)
	out = append(out, top)
	for i := 0; i < r.h-2; i++ {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		edge := bs.Render("│")
		if i >= thumbStart && i < thumbEnd {
			edge = thumb.Render("┃")
		}
		out = append(out, bs.Render("│")+line+edge)
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", max(0, r.w-2))+"╯"))
	return out
}

func padRight(s string, w int) string {
	if sw := lipgloss.Width(s); sw < w {
		return s + strings.Repeat(" ", w-sw)
	}
	return ansi.Truncate(s, w, "")
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	return max(lo, min(hi, v))
}

type hint struct{ key, label string }

func (m *Model) hints() []hint {
	switch {
	case m.showHelp:
		return []hint{{"↑↓", "scroll"}, {"esc", "close"}}
	case m.focus == focusPrompt:
		return []hint{{"⏎", "run"}, {"alt+⏎", "run & return"}, {"tab", "complete"}, {"↑↓", "history"}, {"esc", "back"}}
	case m.focus == focusInfo:
		return []hint{{"↑↓", "scroll"}, {"n/N", "next item"}, {"⏎", "use item"}, {"click", "add flag"}, {"esc", "back"}, {"?", "keys"}}
	case m.filtering:
		return []hint{{"type", "filter"}, {"⏎", "done"}, {"esc", "clear"}}
	}
	return []hint{{"↑↓", "move"}, {"⏎", "open"}, {"e", "edit"}, {"/", "filter"}, {"tab", "focus"}, {"?", "keys"}, {"q", "quit"}}
}

func (m *Model) renderStatus() string {
	var parts []string
	for _, h := range m.hints() {
		parts = append(parts, m.st.key.Render(h.key)+" "+m.st.muted.Render(h.label))
	}
	left := " " + strings.Join(parts, "  ")
	right := m.st.muted.Render("ctlctl " + m.opts.Version + " ")
	if m.status != "" {
		right = m.st.accent.Render(m.status + " ")
	}
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > m.w {
		if m.status != "" {
			return padRight(" "+right, m.w)
		}
		return padRight(left, m.w)
	}
	return left + strings.Repeat(" ", m.w-lw-rw) + right
}

// View renders the screen.
func (m *Model) View() string {
	if m.w == 0 {
		return ""
	}
	l := m.layout()
	if !l.ok {
		msg := "ctlctl needs a bigger terminal"
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, m.st.muted.Render(msg))
	}
	tree := m.renderTree(l.tree)
	info := m.renderInfo(l.info)
	lines := make([]string, 0, m.h)
	for i := 0; i < l.tree.h; i++ {
		lines = append(lines, tree[i]+info[i])
	}
	lines = append(lines, m.renderPrompt(l.prompt)...)
	lines = append(lines, m.renderStatus())
	return strings.Join(lines, "\n")
}

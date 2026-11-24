package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/rafalmaciejewski/ctlctl/internal/helpdoc"
	"github.com/rafalmaciejewski/ctlctl/internal/shell"
)

type palette struct {
	accent, border, muted, text, header, command, flag, arg, str, program,
	operator, variable, url, errc, selBg, selBgBlur, markBg lipgloss.TerminalColor
}

func newPalette(r *lipgloss.Renderer) palette {
	c := func(light, dark string) lipgloss.TerminalColor {
		if r.HasDarkBackground() {
			return lipgloss.Color(dark)
		}
		return lipgloss.Color(light)
	}
	return palette{
		accent:    c("#5B4BDB", "#A594FF"),
		border:    c("#C4C4D0", "#454559"),
		muted:     c("#6E6E85", "#8C8CA6"),
		text:      c("#24242E", "#DCDCE6"),
		header:    c("#A85A00", "#F2B661"),
		command:   c("#127A4A", "#62D9A3"),
		flag:      c("#0A6BA8", "#67C6FF"),
		arg:       c("#8A5A00", "#E6C384"),
		str:       c("#2E7D32", "#A6DA95"),
		program:   c("#B02A7E", "#FF7AC6"),
		operator:  c("#C62828", "#FF7B72"),
		variable:  c("#7B1FA2", "#D7A3FF"),
		url:       c("#0A6BA8", "#82AAFF"),
		errc:      c("#C62828", "#FF7B72"),
		selBg:     c("#E3DEFF", "#3B3266"),
		selBgBlur: c("#ECECF2", "#2A2A38"),
		markBg:    c("#FFF4D6", "#3A3220"),
	}
}

type styles struct {
	r     *lipgloss.Renderer
	p     palette
	kinds map[helpdoc.Kind]lipgloss.Style
	class map[shell.Class]lipgloss.Style

	text, muted, accent, border, borderFocus, title, titleFocus lipgloss.Style
	key, err, ghost, hint, cursor, treeSel, treeSelBlur, match  lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) *styles {
	p := newPalette(r)
	s := &styles{r: r, p: p}
	ns := func(c lipgloss.TerminalColor) lipgloss.Style { return r.NewStyle().Foreground(c) }

	s.text = ns(p.text)
	s.muted = ns(p.muted)
	s.accent = ns(p.accent)
	s.border = ns(p.border)
	s.borderFocus = ns(p.accent)
	s.title = ns(p.muted).Bold(true)
	s.titleFocus = ns(p.accent).Bold(true)
	s.key = ns(p.accent).Bold(true)
	s.err = ns(p.errc)
	s.ghost = ns(p.muted).Faint(true)
	s.hint = ns(p.muted).Italic(true).Faint(true)
	s.cursor = r.NewStyle().Reverse(true)
	s.treeSel = ns(p.text).Background(p.selBg).Bold(true)
	s.treeSelBlur = ns(p.text).Background(p.selBgBlur)
	s.match = r.NewStyle().Underline(true).Bold(true)

	s.kinds = map[helpdoc.Kind]lipgloss.Style{
		helpdoc.KindText:        s.text,
		helpdoc.KindHeader:      ns(p.header).Bold(true),
		helpdoc.KindCommand:     ns(p.command).Bold(true),
		helpdoc.KindFlag:        ns(p.flag),
		helpdoc.KindArg:         ns(p.arg).Italic(true),
		helpdoc.KindPlaceholder: ns(p.arg),
		helpdoc.KindDefault:     ns(p.muted).Italic(true),
		helpdoc.KindComment:     ns(p.muted).Italic(true),
		helpdoc.KindURL:         ns(p.url).Underline(true),
		helpdoc.KindCode:        ns(p.str),
		helpdoc.KindPunct:       ns(p.muted),
		helpdoc.KindProgram:     ns(p.program).Bold(true),
		helpdoc.KindString:      ns(p.str),
		helpdoc.KindOperator:    ns(p.operator).Bold(true),
		helpdoc.KindVariable:    ns(p.variable),
	}
	s.class = map[shell.Class]lipgloss.Style{
		shell.ClassNone:       s.text,
		shell.ClassProgram:    ns(p.program).Bold(true),
		shell.ClassSubcommand: ns(p.command).Bold(true),
		shell.ClassFlag:       ns(p.flag),
		shell.ClassValue:      ns(p.arg),
		shell.ClassString:     ns(p.str),
		shell.ClassVariable:   ns(p.variable),
		shell.ClassAssign:     ns(p.variable),
		shell.ClassOperator:   ns(p.operator).Bold(true),
		shell.ClassComment:    ns(p.muted).Italic(true),
		shell.ClassArg:        s.text,
	}
	return s
}

func (s *styles) kind(k helpdoc.Kind) lipgloss.Style {
	if st, ok := s.kinds[k]; ok {
		return st
	}
	return s.text
}

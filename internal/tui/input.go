package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	// Keys typed quickly can arrive as one message. Outside text fields
	// each one is a separate command.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste && m.focus != focusPrompt && !m.filtering {
		var cmds []tea.Cmd
		for _, r := range msg.Runes {
			cmds = append(cmds, m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt}))
		}
		return tea.Batch(cmds...)
	}
	key := msg.String()
	if key == "ctrl+c" {
		m.result = Result{}
		return tea.Quit
	}
	if m.showHelp {
		switch key {
		case "esc", "?", "q", "enter":
			m.showHelp = false
			return nil
		}
		m.scrollKeys(key)
		return nil
	}

	if m.focus == focusPrompt {
		if cmd, ok := m.handlePromptKey(msg); ok {
			return cmd
		}
	}

	switch key {
	case "tab":
		m.setFocus((m.focus + 1) % 3)
		return nil
	case "shift+tab":
		m.setFocus((m.focus + 2) % 3)
		return nil
	}

	if m.focus == focusTree && m.filtering {
		return m.handleFilterKey(msg)
	}

	switch m.focus {
	case focusTree:
		return m.handleTreeKey(key)
	case focusInfo:
		return m.handleInfoKey(key)
	}
	return nil
}

func (m *Model) handleFilterKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filter = ""
	case "enter":
		m.filtering = false
		return nil
	case "up", "ctrl+p":
		return m.moveTree(-1)
	case "down", "ctrl+n":
		return m.moveTree(1)
	case "backspace", "ctrl+h":
		r := []rune(m.filter)
		if len(r) == 0 {
			return nil
		}
		m.filter = string(r[:len(r)-1])
	default:
		if msg.Type != tea.KeyRunes && msg.Type != tea.KeySpace {
			return nil
		}
		m.filter += string(msg.Runes)
	}
	m.rebuildRows()
	// The filter may have moved the selection to a node not loaded yet.
	if m.sel != nil {
		return m.load(m.sel)
	}
	return nil
}

func (m *Model) treePage() int { return max(1, m.layout().tree.h-3) }

func (m *Model) handleTreeKey(key string) tea.Cmd {
	n := m.sel
	switch key {
	case "q":
		m.result = Result{}
		return tea.Quit
	case "?":
		m.showHelp = true
	case "/":
		m.filtering = true
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.rebuildRows()
		}
		return nil
	case "up", "k", "ctrl+p":
		return m.moveTree(-1)
	case "down", "j", "ctrl+n":
		return m.moveTree(1)
	case "pgup", "ctrl+b":
		return m.moveTree(-m.treePage())
	case "pgdown", "ctrl+f":
		return m.moveTree(m.treePage())
	case "home", "g":
		return m.moveTree(-len(m.rows))
	case "end", "G":
		return m.moveTree(len(m.rows))
	case "e", "i", "a":
		if n != nil {
			m.setFocus(focusPrompt)
		}
	}
	if n == nil {
		return nil
	}
	switch key {
	case "enter":
		return m.activate(n)
	case " ":
		return m.toggle(n)
	case "right", "l":
		if n.expanded && len(n.children) > 0 {
			return m.selectNode(n.children[0])
		}
		if n.state == stateLoaded && len(n.children) == 0 {
			return nil
		}
		return m.toggle(n)
	case "left", "h":
		if n.expanded {
			return m.toggle(n)
		}
		if n.parent != nil {
			return m.selectNode(n.parent)
		}
	}
	return nil
}

func (m *Model) infoPage() int { return max(1, m.layout().info.h-3) }

// scrollKeys scrolls the help pane. It reports whether it used the key.
func (m *Model) scrollKeys(key string) bool {
	switch key {
	case "up", "k", "ctrl+p":
		m.infoScroll--
	case "down", "j", "ctrl+n":
		m.infoScroll++
	case "pgup", "ctrl+b", "ctrl+u":
		m.infoScroll -= m.infoPage()
	case "pgdown", "ctrl+f", "ctrl+d", " ":
		m.infoScroll += m.infoPage()
	case "home", "g":
		m.infoScroll = 0
	case "end", "G":
		m.infoScroll = len(m.info.rows)
	default:
		return false
	}
	m.clampScroll()
	return true
}

func (m *Model) handleInfoKey(key string) tea.Cmd {
	if m.scrollKeys(key) {
		return nil
	}
	switch key {
	case "q":
		m.result = Result{}
		return tea.Quit
	case "?":
		m.showHelp = true
	case "esc", "left", "h":
		m.infoSel = -1
		m.setFocus(focusTree)
	case "n", "right", "l":
		m.moveTarget(1)
	case "N", "p":
		m.moveTarget(-1)
	case "enter":
		if m.infoSel >= 0 && m.infoSel < len(m.info.targets) {
			return m.useTarget(m.info.targets[m.infoSel])
		}
		m.moveTarget(1)
	case "e", "i", "a":
		m.setFocus(focusPrompt)
	}
	return nil
}

const doubleClick = 400 * time.Millisecond

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	l := m.layout()
	if !l.ok {
		return nil
	}
	x, y := msg.X, msg.Y

	if msg.Action == tea.MouseActionMotion {
		if ref, ok := m.infoHit(x, y); ok {
			m.hover = ref
		} else {
			m.hover = noTarget
		}
		return nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		d := 3
		if msg.Button == tea.MouseButtonWheelUp {
			d = -3
		}
		switch {
		case l.tree.contains(x, y):
			m.treeScroll += d
		case l.info.contains(x, y):
			m.infoScroll += d
		}
		m.clampScroll()
		if ref, ok := m.infoHit(x, y); ok {
			m.hover = ref
		} else {
			m.hover = noTarget
		}
		return nil
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return nil
		}
	default:
		return nil
	}

	switch {
	case l.tree.contains(x, y):
		m.setFocus(focusTree)
		m.filtering = false
		in := l.tree.inner()
		if !in.contains(x, y) {
			return nil
		}
		ri := m.treeScroll + y - in.y
		if ri < 0 || ri >= len(m.rows) {
			return nil
		}
		n := m.rows[ri]
		glyphX := in.x + 1 + 2*n.depth
		now := time.Now()
		double := ri == m.lastClickRow && now.Sub(m.lastClick) < doubleClick
		m.lastClick, m.lastClickRow = now, ri
		switch {
		case x == glyphX:
			cmd := m.selectNode(n)
			return tea.Batch(cmd, m.toggle(n))
		case double:
			m.lastClick = time.Time{}
			return m.activate(n)
		}
		return m.selectNode(n)

	case l.info.contains(x, y):
		if ref, ok := m.infoHit(x, y); ok {
			m.infoSel = -1
			return m.useTarget(ref)
		}
		m.setFocus(focusInfo)

	case l.prompt.contains(x, y):
		m.setFocus(focusPrompt)
		if l.prompt.inner().contains(x, y) {
			m.cur = m.promptIndex(x, y)
		}
	}
	return nil
}

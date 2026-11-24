package tui

import (
	"strings"

	"github.com/rafalmaciejewski/ctlctl/internal/helpdoc"
)

type loadState uint8

const (
	stateIdle loadState = iota
	stateLoading
	stateLoaded
)

type action uint8

const (
	actNone     action = iota
	actActivate        // open the node once its help is loaded (Enter)
	actExpand          // expand the node once its help is loaded
)

// node is a tool or one of its subcommands in the tree.
type node struct {
	name     string
	desc     string
	bin      string // set on tools
	parent   *node
	children []*node
	depth    int
	expanded bool

	state   loadState
	pending action
	doc     *helpdoc.Doc
	source  string
	err     error
	// inherited is the ancestor whose help this node printed: the tool has
	// no help for the subcommand itself.
	inherited *node

	global       *helpdoc.Doc // options for every subcommand (tools only)
	globalSource string

	// inferred is set for subcommands read from a usage line or a man page
	// list. ctlctl doesn't run anything for them.
	inferred bool
}

func (n *node) path() []string {
	var p []string
	for c := n; c != nil; c = c.parent {
		p = append([]string{c.name}, p...)
	}
	return p
}

func (n *node) key() string { return strings.Join(n.path(), " ") }

func (n *node) root() *node {
	for n.parent != nil {
		n = n.parent
	}
	return n
}

// docNode is the node whose help n shows.
func (n *node) docNode() *node {
	if n.inherited != nil {
		return n.inherited
	}
	return n
}

func (n *node) helpDoc() *helpdoc.Doc { return n.docNode().doc }

// child finds a subcommand by name or alias.
func (n *node) child(name string) *node {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	if d := n.doc; d != nil {
		if c := d.FindCommand(name); c != nil && c.Name != name {
			return n.child(c.Name)
		}
	}
	return nil
}

// findFlag looks a flag up in the help of n, its ancestors and the tool's
// global options.
func (n *node) findFlag(name string) *helpdoc.Flag {
	for c := n; c != nil; c = c.parent {
		if f := c.helpDoc().FindFlag(name); f != nil {
			return f
		}
	}
	return n.root().global.FindFlag(name)
}

// flags lists every flag that applies to n, nearest first.
func (n *node) flags() []*helpdoc.Flag {
	var out []*helpdoc.Flag
	seen := map[*helpdoc.Doc]bool{}
	add := func(d *helpdoc.Doc) {
		if d == nil || seen[d] {
			return
		}
		seen[d] = true
		for i := range d.Flags {
			out = append(out, &d.Flags[i])
		}
	}
	for c := n; c != nil; c = c.parent {
		add(c.helpDoc())
	}
	add(n.root().global)
	return out
}

func (n *node) setChildren(cmds []helpdoc.Command) {
	n.children = n.children[:0]
	for _, c := range cmds {
		n.children = append(n.children, &node{name: c.Name, desc: c.Desc, parent: n, depth: n.depth + 1, inferred: c.Fallback})
	}
}

// ancestors returns n's parents, nearest first.
func (n *node) ancestors() []*node {
	var out []*node
	for c := n.parent; c != nil; c = c.parent {
		out = append(out, c)
	}
	return out
}

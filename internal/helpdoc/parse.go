// Package helpdoc parses the free-form --help output of command-line tools
// into commands, flags and highlighted, clickable lines.
//
// Help formats differ a lot between tools (Cobra, urfave/cli, clap, argparse,
// Swift ArgumentParser, systemd, BSD usage lines, man pages), so the parser
// relies on layout heuristics shared by most of them: section headers end
// with a colon or are written in capitals, entries are indented, and a name
// is separated from its description by at least two spaces.
package helpdoc

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rafalmaciejewski/ctlctl/internal/shell"
)

// Kind is the highlighting class of a span of help text.
type Kind uint8

const (
	KindText Kind = iota
	KindHeader
	KindCommand
	KindFlag
	KindArg
	KindPlaceholder
	KindDefault
	KindComment
	KindURL
	KindCode
	KindPunct
	KindProgram
	KindString
	KindOperator
	KindVariable
)

// TargetKind says what clicking a target does.
type TargetKind uint8

const (
	TargetFlag    TargetKind = iota + 1 // insert a flag into the command
	TargetCommand                       // open a subcommand
	TargetExample                       // replace the command with an example
)

// Target is a clickable part of a line.
type Target struct {
	Kind       TargetKind
	Value      string
	TakesValue bool
}

// Span is a half-open range of rune offsets within a line.
type Span struct {
	Start, End int
	Kind       Kind
}

// TargetSpan is a clickable half-open range of rune offsets within a line.
type TargetSpan struct {
	Start, End int
	Target     Target
}

// Line is one line of help text with its highlighting and click targets.
type Line struct {
	Text    string
	Styles  []Span // sorted, non-overlapping
	Targets []TargetSpan
}

// Command is a subcommand listed in the help.
type Command struct {
	Name    string
	Aliases []string
	Args    string
	Desc    string
	Line    int
	// Fallback is set when the command was inferred from a usage line or a
	// man page list rather than read from a commands section. Such tools
	// rarely accept help flags after a subcommand.
	Fallback bool
}

// Flag is an option listed in the help.
type Flag struct {
	Names   []string
	Arg     string // value placeholder; empty for boolean flags
	Default string
	Desc    string
	Line    int
}

// TakesValue reports whether the flag consumes a value.
func (f *Flag) TakesValue() bool { return f.Arg != "" }

// Preferred returns the name to insert for the flag: the first long name, or
// the first name when there is no long one.
func (f *Flag) Preferred() string {
	for _, n := range f.Names {
		if strings.HasPrefix(n, "--") {
			return n
		}
	}
	return f.Names[0]
}

// Doc is parsed help output.
type Doc struct {
	Lines    []Line
	Commands []Command
	Flags    []Flag
	Usage    []string

	helpHint bool // the text says `<command> --help` works
}

// HelpFlag reports whether the help documents --help, so subcommands can be
// asked for their help with `sub --help`.
func (d *Doc) HelpFlag() bool {
	return d != nil && (d.FindFlag("--help") != nil || d.helpHint)
}

// HelpDashH reports whether the help documents -h as a help flag.
func (d *Doc) HelpDashH() bool {
	f := d.FindFlag("-h")
	return f != nil && strings.Contains(strings.ToLower(f.Desc), "help")
}

// HelpCommand reports whether the help lists a help subcommand.
func (d *Doc) HelpCommand() bool {
	c := d.FindCommand("help")
	return c != nil && !c.Fallback
}

// FindFlag looks a flag up by any of its names. "--name=value" matches too.
func (d *Doc) FindFlag(name string) *Flag {
	if d == nil {
		return nil
	}
	if i := strings.IndexByte(name, '='); i > 0 {
		name = name[:i]
	}
	for i := range d.Flags {
		if slices.Contains(d.Flags[i].Names, name) {
			return &d.Flags[i]
		}
	}
	return nil
}

// FindCommand looks a subcommand up by name or alias.
func (d *Doc) FindCommand(name string) *Command {
	if d == nil {
		return nil
	}
	for i := range d.Commands {
		c := &d.Commands[i]
		if c.Name == name || slices.Contains(c.Aliases, name) {
			return c
		}
	}
	return nil
}

type section uint8

const (
	secText section = iota
	secUsage
	secCommands
	secOptions
	secExamples
	secArgs
)

func classify(name string) section {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "usage") || strings.Contains(n, "synopsis"):
		return secUsage
	case strings.Contains(n, "example"):
		return secExamples
	case strings.Contains(n, "help topic"):
		return secText
	case strings.Contains(n, "command") || strings.Contains(n, "verbs") || n == "actions":
		return secCommands
	case strings.Contains(n, "option") || strings.Contains(n, "flag") || strings.Contains(n, "switches"):
		return secOptions
	case strings.Contains(n, "argument") || strings.Contains(n, "parameter"):
		return secArgs
	}
	return secText
}

var (
	reInlineHeader = regexp.MustCompile(`^([A-Za-z][A-Za-z ]{0,30}?):\s+(\S.*)$`)
	reAllCaps      = regexp.MustCompile(`^[A-Z][A-Z0-9 _/&()-]*[A-Z0-9)]$`)
	reFlagStart    = regexp.MustCompile(`^--?(?:\[no-\])?[A-Za-z0-9?]`)
	reFlagName     = regexp.MustCompile(`--?(?:\[no-\])?(?:[A-Za-z0-9][A-Za-z0-9_.+-]*|\?)`)
	reCmdName      = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.:+@-]*)\*?((?:\s*[,|]\s*[a-z][a-z0-9_.:+-]*)*)`)
	reURL          = regexp.MustCompile(`https?://[^\s)\]>"'` + "`" + `]+`)
	reDefault      = regexp.MustCompile(`\((?:[Dd]efaults?(?: to)?|[Dd]efault value)\b[^)]*\)|\[(?:[Dd]efault|env)[^\]]*\]`)
	reBacktick     = regexp.MustCompile("`[^`]+`")
	reLongMention  = regexp.MustCompile(`(?:^|[\s("'\[=,/])(--[A-Za-z0-9][A-Za-z0-9_-]*)`)
	reShortMention = regexp.MustCompile(`(?:^|[\s("'\[,/])(-[A-Za-z0-9])(?:[\s,.)'"\]=:/]|$)`)
	reAngle        = regexp.MustCompile(`<[^<>\s][^<>]*>`)
	reUpper        = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])([A-Z][A-Z0-9_]+(?:\.\.\.)?)`)
	rePunct        = regexp.MustCompile(`\.\.\.|[\[\]{}()|]`)
	reUsageFlag    = regexp.MustCompile(`(?:^|[\s\[(|,{=])(--?(?:\[no-\])?[A-Za-z0-9?][A-Za-z0-9_-]*)`)
	reWordCmd      = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	reComment      = regexp.MustCompile(`(?:^|\s)(#\s.*)$`)
	reDefTerm      = regexp.MustCompile(`^([a-z][a-z0-9_-]*)((?:,\s*[a-z][a-z0-9_-]*)*)`)
	reCommandsWord = regexp.MustCompile(`(?i)\b(?:sub)?commands?\b`)
	reHelpHint     = regexp.MustCompile(`(?i)[<\[](?:sub)?command[>\]]\s+--help|\bCOMMAND\s+--help`)
)

type entryKind uint8

const (
	entryFlag entryKind = iota + 1
	entryCommand
	entryArg
)

type entry struct {
	kind   entryKind
	indent int
	index  int
}

type mention struct {
	line, start, end int
	name             string
	styled           bool
	usage            bool // found in a usage line
}

type usageCmd struct {
	line, start, end int
	name             string
}

type example struct {
	lines []int
	start []int // rune offset of the command on each line
	cmd   string
}

type parser struct {
	d        *Doc
	path     []string
	mentions []mention
	usage    []usageCmd
	examples []example
	descCol  int
	runes    [][]rune

	// The commands section being read.
	secStart int
	secDesc  bool
	weak     []weakCmd
}

// usageHelp is set when a usage line offers --help.
func (p *parser) usageHelp() bool {
	for _, m := range p.mentions {
		if m.name == "--help" && m.usage {
			return true
		}
	}
	return false
}

// Parse parses raw help output for the command at path (the tool's name
// followed by subcommands).
func Parse(raw string, path []string) *Doc {
	text := Clean(raw)
	var src []string
	if text != "" {
		src = strings.Split(text, "\n")
	}
	d := &Doc{Lines: make([]Line, len(src))}
	p := &parser{d: d, path: path, runes: make([][]rune, len(src))}
	for i, l := range src {
		d.Lines[i].Text = l
		p.runes[i] = []rune(l)
	}

	sec := secText
	var cur *entry
	cmdIndent := -1
	var pendingExample *example

	flushExample := func() {
		if pendingExample != nil {
			p.examples = append(p.examples, *pendingExample)
			pendingExample = nil
		}
	}

	for li, l := range src {
		trimmed := strings.TrimLeft(l, " ")
		indent := len(l) - len(trimmed)
		if trimmed == "" {
			cur = nil
			flushExample()
			if sec == secUsage {
				sec = secText
			}
			continue
		}

		// A backslash-continued example keeps going on the next line.
		if pendingExample != nil {
			start := runeOff(l, indent)
			pendingExample.lines = append(pendingExample.lines, li)
			pendingExample.start = append(pendingExample.start, start)
			pendingExample.cmd += " " + strings.TrimSuffix(trimmed, "\\")
			if !strings.HasSuffix(trimmed, "\\") {
				flushExample()
			}
			continue
		}

		if name, rest, ok := header(l); ok {
			p.flushSection(cmdIndent)
			sec = classify(name)
			cur, cmdIndent, p.descCol = nil, -1, 0
			if rest < 0 {
				p.style(li, 0, len(p.runes[li]), KindHeader)
				continue
			}
			labelEnd := runeOff(l, len(name)+1)
			p.style(li, 0, labelEnd, KindHeader)
			if sec == secUsage {
				d.Usage = append(d.Usage, l[rest:])
				p.styleUsage(li, runeOff(l, rest))
			} else {
				p.styleText(li, runeOff(l, rest))
			}
			continue
		}

		switch sec {
		case secExamples:
			if strings.HasPrefix(trimmed, "#") {
				p.style(li, runeOff(l, indent), len(p.runes[li]), KindComment)
				continue
			}
			if indent > 0 || strings.HasPrefix(trimmed, "$ ") || p.startsWithProgram(trimmed) {
				pendingExample = p.newExample(li, l, indent)
				if !strings.HasSuffix(trimmed, "\\") {
					flushExample()
				}
				continue
			}
		case secUsage:
			if reFlagStart.MatchString(trimmed) {
				// "Usage of tool:" followed by a flag list (Go's flag package).
				sec = secOptions
				break
			}
			if indent > 0 || p.startsWithProgram(trimmed) {
				d.Usage = append(d.Usage, trimmed)
				p.styleUsage(li, runeOff(l, indent))
				continue
			}
			sec = secText
		}

		if reFlagStart.MatchString(trimmed) && trimmed != "--" && (indent > 0 || sec == secOptions) {
			if i := p.parseFlag(li, l, indent); i >= 0 {
				cur = &entry{kind: entryFlag, indent: indent, index: i}
				continue
			}
		}

		if cur != nil && indent > cur.indent {
			p.continueEntry(cur, li, l, indent)
			continue
		}

		switch sec {
		case secCommands:
			if cmdIndent < 0 || indent == cmdIndent {
				if i := p.parseCommand(li, l, indent); i >= 0 {
					cmdIndent = indent
					cur = &entry{kind: entryCommand, indent: indent, index: i}
					continue
				}
			}
		case secArgs:
			if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
				sec, cmdIndent, cur = secCommands, -1, nil
				p.style(li, runeOff(l, indent), len(p.runes[li]), KindPunct)
				continue
			}
			if indent > 0 {
				name, _, _ := strings.Cut(trimmed, " ")
				start := runeOff(l, indent)
				p.style(li, start, start+utf8.RuneCountInString(name), KindPlaceholder)
				p.styleText(li, start+utf8.RuneCountInString(name))
				cur = &entry{kind: entryArg, indent: indent}
				continue
			}
		}

		if strings.HasPrefix(trimmed, "$ ") {
			p.examples = append(p.examples, *p.newExample(li, l, indent))
			continue
		}
		p.styleText(li, 0)
	}
	flushExample()
	p.flushSection(cmdIndent)
	p.finish()
	return d
}

// flushSection ends a section. A commands section whose entries have no
// descriptions is a bare list, so entries with free-form arguments count too.
func (p *parser) flushSection(cmdIndent int) {
	if !p.secDesc && len(p.weak) > 0 {
		for _, w := range p.weak {
			if cmdIndent >= 0 && w.indent != cmdIndent {
				continue
			}
			l := p.d.Lines[w.line].Text
			if p.d.FindCommand(l[w.indent:][w.m[2]:w.m[3]]) != nil {
				continue
			}
			p.addCommand(w.line, l, w.indent, w.m, strings.TrimSpace(l[w.indent:][w.m[1]:]), -1)
		}
		slices.SortStableFunc(p.d.Commands[p.secStart:], func(a, b Command) int { return a.Line - b.Line })
	}
	p.weak, p.secDesc, p.secStart = nil, false, len(p.d.Commands)
}

// header recognises a section header. rest is the byte offset of text that
// follows an inline header such as "Usage: tool [flags]", or -1.
func header(l string) (name string, rest int, ok bool) {
	if l != "" && l[0] == ' ' {
		// "   Available commands:" (xsanctl) is indented.
		t := strings.TrimLeft(l, " ")
		if len(l)-len(t) <= 4 && strings.HasSuffix(t, ":") && len(strings.Fields(t)) <= 4 && reCommandsWord.MatchString(t) {
			return strings.TrimSuffix(t, ":"), -1, true
		}
		return "", 0, false
	}
	if l == "" || l[0] == '-' {
		return "", 0, false
	}
	if strings.HasSuffix(l, ":") {
		name = strings.TrimSuffix(l, ":")
		if name == "" || len(name) > 80 || len(strings.Fields(name)) > 10 || strings.Contains(name, "://") {
			return "", 0, false
		}
		return name, -1, true
	}
	if len(l) >= 3 && len(l) <= 40 && reAllCaps.MatchString(l) && countLetters(l) >= 3 {
		return l, -1, true
	}
	if m := reInlineHeader.FindStringSubmatchIndex(l); m != nil {
		label := l[m[2]:m[3]]
		lower := strings.ToLower(label)
		if lower == "usage" || lower == "synopsis" || (label == strings.ToUpper(label) && countLetters(label) >= 3) {
			return label, m[4], true
		}
	}
	return "", 0, false
}

func countLetters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// runeOff converts a byte offset in s to a rune offset.
func runeOff(s string, b int) int {
	if b > len(s) {
		b = len(s)
	}
	return utf8.RuneCountInString(s[:b])
}

// style records a highlight. Earlier styles win where spans overlap.
func (p *parser) style(li, start, end int, k Kind) {
	if end > start {
		p.d.Lines[li].Styles = append(p.d.Lines[li].Styles, Span{start, end, k})
	}
}

func (p *parser) target(li, start, end int, t Target) {
	if end > start {
		p.d.Lines[li].Targets = append(p.d.Lines[li].Targets, TargetSpan{start, end, t})
	}
}

func (p *parser) program() string {
	if len(p.path) == 0 {
		return ""
	}
	return p.path[0]
}

func (p *parser) startsWithProgram(s string) bool {
	first, _, _ := strings.Cut(s, " ")
	return p.program() != "" && filepath.Base(first) == p.program()
}

// gap returns the byte offsets where a run of two or more spaces starts and
// ends in s, skipping quoted text and brackets, or -1, -1 if there is none.
func gap(s string) (int, int) {
	var quote byte
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"':
			quote = c
		case c == '[' || c == '<' || c == '{':
			depth++
		case (c == ']' || c == '>' || c == '}') && depth > 0:
			depth--
		case c == ' ' && depth == 0 && i+1 < len(s) && s[i+1] == ' ':
			j := i
			for j < len(s) && s[j] == ' ' {
				j++
			}
			return i, j
		}
	}
	return -1, -1
}

// splitEntry splits an entry into its name part and description, using the
// section's description column when the two are separated by one space only.
func (p *parser) splitEntry(l string, indent int) (spec string, descAt int) {
	t := l[indent:]
	gs, ge := gap(t)
	if gs < 0 {
		gs, ge = len(t), len(t)
	}
	if p.descCol > indent+1 && p.descCol < len(l) && indent+gs > p.descCol && l[p.descCol-1] == ' ' && l[p.descCol] != ' ' {
		gs, ge = p.descCol-1-indent, p.descCol-indent
	}
	if ge < len(t) && p.descCol == 0 {
		p.descCol = indent + ge
	}
	if ge >= len(t) {
		return strings.TrimRight(t[:gs], " "), -1
	}
	return t[:gs], indent + ge
}

func (p *parser) parseFlag(li int, l string, indent int) int {
	spec, descAt := p.splitEntry(l, indent)
	kubeStyle := strings.HasSuffix(spec, ":") && descAt < 0
	spec = strings.TrimSuffix(spec, ":")
	base := runeOff(l, indent)

	f := Flag{Line: li}
	type piece struct{ start, end int } // byte offsets within spec
	var names []piece
	for _, m := range reFlagName.FindAllStringIndex(spec, -1) {
		if m[0] > 0 && !strings.ContainsRune(" ,|[(/", rune(spec[m[0]-1])) {
			continue
		}
		// Words after a flag name are its value, not more names, unless they
		// follow a separator.
		if m[0] > 0 && spec[m[0]-1] == ' ' && len(names) > 0 {
			between := spec[names[len(names)-1].end:m[0]]
			if strings.TrimSpace(strings.Trim(between, ", |/")) != "" && !strings.ContainsAny(between, ",|/") {
				continue
			}
		}
		names = append(names, piece{m[0], m[1]})
		raw := spec[m[0]:m[1]]
		raw = strings.TrimRight(raw, ".")
		if strings.Contains(raw, "[no-]") {
			plain := strings.Replace(raw, "[no-]", "", 1)
			neg := strings.Replace(raw, "[no-]", "no-", 1)
			f.Names = appendUnique(f.Names, plain, neg)
		} else {
			f.Names = appendUnique(f.Names, raw)
		}
	}
	if len(f.Names) == 0 {
		return -1
	}

	// Whatever is left between and after the names describes the value.
	var args []string
	var argPieces []piece
	defaultValue := false
	for i, n := range names {
		end := len(spec)
		if i+1 < len(names) {
			end = names[i+1].start
		}
		s := n.end
		chunk := spec[s:end]
		trim := strings.TrimRight(chunk, " ,|/")
		lead := len(trim) - len(strings.TrimLeft(trim, " "))
		s += lead
		trim = strings.TrimLeft(trim, " ")
		if trim == "" {
			continue
		}
		val := trim
		if strings.HasPrefix(val, "[=") {
			val = strings.TrimSuffix(strings.TrimPrefix(val, "[="), "]")
		} else if strings.HasPrefix(val, "=") {
			val = strings.TrimPrefix(val, "=")
			if kubeStyle || !isPlaceholder(val) {
				defaultValue = true
				f.Default = val
			}
		}
		argPieces = append(argPieces, piece{s, s + len(trim)})
		if val != "" && !slices.Contains(args, val) {
			args = append(args, val)
		}
	}
	switch {
	case defaultValue && (f.Default == "true" || f.Default == "false"):
		f.Arg = ""
	case defaultValue:
		f.Arg = "value"
	default:
		f.Arg = strings.Join(args, " ")
		if f.Arg == "true" || f.Arg == "false" {
			f.Arg = ""
		}
	}

	idx := len(p.d.Flags)
	for _, n := range names {
		value := strings.Replace(strings.TrimRight(spec[n.start:n.end], "."), "[no-]", "", 1)
		s, e := base+runeOff(spec, n.start), base+runeOff(spec, n.end)
		p.style(li, s, e, KindFlag)
		p.target(li, s, e, Target{Kind: TargetFlag, Value: value, TakesValue: f.TakesValue()})
	}
	for _, a := range argPieces {
		s, e := base+runeOff(spec, a.start), base+runeOff(spec, a.end)
		k := KindArg
		if defaultValue {
			k = KindDefault
		}
		p.style(li, s, e, k)
		p.target(li, s, e, Target{Kind: TargetFlag, Value: (&f).Preferred(), TakesValue: f.TakesValue()})
	}
	if descAt >= 0 {
		f.Desc = strings.TrimSpace(l[descAt:])
		p.styleText(li, runeOff(l, descAt))
	}
	p.d.Flags = append(p.d.Flags, f)
	return idx
}

func appendUnique(list []string, items ...string) []string {
	for _, it := range items {
		if it != "" && !slices.Contains(list, it) {
			list = append(list, it)
		}
	}
	return list
}

var typeWords = map[string]bool{
	"string": true, "strings": true, "int": true, "uint": true, "int32": true, "int64": true,
	"uint32": true, "uint64": true, "float": true, "float32": true, "float64": true,
	"duration": true, "value": true, "file": true, "path": true, "dir": true, "num": true,
	"n": true, "name": true, "list": true, "bytes": true, "ip": true, "stringArray": true,
	"stringToString": true, "int32Slice": true, "url": true, "host": true, "port": true,
}

// isPlaceholder reports whether a flag value like TYPE, <file> or string names
// the kind of value rather than giving a default.
func isPlaceholder(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "<") || strings.HasPrefix(s, "{") || typeWords[s] {
		return true
	}
	letters := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			letters = true
		}
	}
	return letters
}

func (p *parser) parseCommand(li int, l string, indent int) int {
	t := l[indent:]
	m := reCmdName.FindStringSubmatchIndex(t)
	if m == nil {
		return -1
	}
	name := t[m[2]:m[3]]
	rest := t[m[1]:]
	if rest != "" && rest[0] != ' ' {
		return -1
	}
	if countLetters(name) == 0 || (name == strings.ToUpper(name) && countLetters(name) > 1) {
		return -1
	}
	spec, descAt := p.splitEntry(l, indent)
	args := strings.TrimSpace(spec[min(m[1], len(spec)):])
	if a, d, ok := strings.Cut(args, " - "); ok && descAt < 0 {
		args = strings.TrimSpace(a)
		descAt = strings.Index(l, d)
	} else if strings.HasPrefix(args, "- ") && descAt < 0 {
		descAt = indent + m[1] + strings.Index(t[m[1]:], "- ") + 2
		args = ""
	}
	if !placeholdersOnly(args) {
		// "mount volumeName [options]": kept in case the whole section turns
		// out to be a bare list of commands without descriptions.
		if descAt < 0 {
			p.weak = append(p.weak, weakCmd{line: li, indent: indent, m: m})
		}
		return -1
	}
	return p.addCommand(li, l, indent, m, args, descAt)
}

type weakCmd struct {
	line, indent int
	m            []int // reCmdName match
}

// addCommand records a command entry and highlights its line.
func (p *parser) addCommand(li int, l string, indent int, m []int, args string, descAt int) int {
	t := l[indent:]
	name := t[m[2]:m[3]]
	c := Command{Name: strings.TrimSuffix(name, "*"), Args: args, Line: li}
	if m[4] < m[5] {
		for _, a := range strings.FieldsFunc(t[m[4]:m[5]], func(r rune) bool { return r == ',' || r == '|' }) {
			if a = strings.TrimSpace(a); a != "" {
				c.Aliases = append(c.Aliases, a)
			}
		}
	}
	base := runeOff(l, indent)
	s, e := base+runeOff(t, m[2]), base+runeOff(t, m[1])
	p.style(li, s, e, KindCommand)
	p.target(li, s, e, Target{Kind: TargetCommand, Value: c.Name})
	if args != "" {
		as := strings.Index(l[indent+m[1]:], args)
		if as >= 0 {
			as += indent + m[1]
			p.styleUsageTokens(li, runeOff(l, as), runeOff(l, as+len(args)))
		}
	}
	if descAt >= 0 && descAt < len(l) {
		c.Desc = strings.TrimPrefix(strings.TrimSpace(l[descAt:]), ": ")
		p.styleText(li, runeOff(l, descAt))
		p.secDesc = true
	}
	if p.d.FindCommand(c.Name) != nil {
		return len(p.d.Commands) - 1
	}
	p.d.Commands = append(p.d.Commands, c)
	return len(p.d.Commands) - 1
}

// placeholdersOnly reports whether s only holds argument placeholders such as
// UNIT..., [PATTERN] or <file>, which may follow a command name.
func placeholdersOnly(s string) bool {
	depth := 0
	for _, tok := range strings.Fields(s) {
		if depth == 0 {
			ok := strings.ContainsAny(tok[:1], "[<{(") || tok == "..." || tok == "|" || isPlaceholder(strings.Trim(tok, ".|=:,"))
			if !ok {
				if i := strings.IndexByte(tok, '='); i > 0 && isPlaceholder(tok[:i]) {
					ok = true
				}
			}
			if !ok {
				return false
			}
		}
		depth += strings.Count(tok, "[") + strings.Count(tok, "<") + strings.Count(tok, "{") + strings.Count(tok, "(")
		depth -= strings.Count(tok, "]") + strings.Count(tok, ">") + strings.Count(tok, "}") + strings.Count(tok, ")")
		if depth < 0 {
			depth = 0
		}
	}
	return true
}

func (p *parser) continueEntry(cur *entry, li int, l string, indent int) {
	text := strings.TrimSpace(l)
	switch cur.kind {
	case entryFlag:
		f := &p.d.Flags[cur.index]
		f.Desc = strings.TrimSpace(f.Desc + " " + text)
	case entryCommand:
		c := &p.d.Commands[cur.index]
		c.Desc = strings.TrimSpace(c.Desc + " " + text)
	}
	p.styleText(li, runeOff(l, indent))
}

func (p *parser) newExample(li int, l string, indent int) *example {
	start := indent
	if strings.HasPrefix(l[indent:], "$ ") {
		start += 2
	}
	cmd := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l[start:]), "\\"))
	return &example{lines: []int{li}, start: []int{runeOff(l, start)}, cmd: cmd}
}

// styleText highlights URLs, defaults, code and flag mentions in prose.
func (p *parser) styleText(li, from int) {
	l := p.d.Lines[li].Text
	fromB := byteOff(l, from)
	text := l[fromB:]
	for _, m := range reURL.FindAllStringIndex(text, -1) {
		u := strings.TrimRight(text[m[0]:m[1]], ".,;:")
		p.style(li, from+runeOff(text, m[0]), from+runeOff(text, m[0]+len(u)), KindURL)
	}
	for _, m := range reDefault.FindAllStringIndex(text, -1) {
		p.style(li, from+runeOff(text, m[0]), from+runeOff(text, m[1]), KindDefault)
	}
	for _, m := range reBacktick.FindAllStringIndex(text, -1) {
		p.style(li, from+runeOff(text, m[0]), from+runeOff(text, m[1]), KindCode)
	}
	for _, m := range reLongMention.FindAllStringSubmatchIndex(text, -1) {
		s, e := from+runeOff(text, m[2]), from+runeOff(text, m[3])
		p.style(li, s, e, KindFlag)
		p.mentions = append(p.mentions, mention{li, s, e, text[m[2]:m[3]], true, false})
	}
	for _, m := range reShortMention.FindAllStringSubmatchIndex(text, -1) {
		s, e := from+runeOff(text, m[2]), from+runeOff(text, m[3])
		p.mentions = append(p.mentions, mention{li, s, e, text[m[2]:m[3]], false, false})
	}
}

func byteOff(s string, r int) int {
	if r <= 0 {
		return 0
	}
	n := 0
	for i := range s {
		if n == r {
			return i
		}
		n++
	}
	return len(s)
}

// styleUsage highlights a usage line such as "tool sub [flags] <arg>".
func (p *parser) styleUsage(li, from int) {
	l := p.d.Lines[li].Text
	text := l[byteOff(l, from):]
	fields := fieldsIndex(text)
	matched, restAt := 0, 0
	for i, f := range fields {
		word := text[f[0]:f[1]]
		s, e := from+runeOff(text, f[0]), from+runeOff(text, f[1])
		if i == 0 && filepath.Base(word) == p.program() {
			p.style(li, s, e, KindProgram)
			matched, restAt = 1, f[1]
			continue
		}
		if matched == i && matched > 0 && matched < len(p.path) && word == p.path[matched] {
			p.style(li, s, e, KindCommand)
			matched, restAt = matched+1, f[1]
			continue
		}
		break
	}
	// "tool [ list ]" and "tool { start | stop } <unit>" name the subcommands.
	if matched > 0 && matched == len(p.path) {
		for _, a := range usageAlternatives(text[restAt:]) {
			s, e := from+runeOff(text, restAt+a.start), from+runeOff(text, restAt+a.end)
			p.style(li, s, e, KindCommand)
			p.usage = append(p.usage, usageCmd{li, s, e, a.word})
		}
	}
	if m := reComment.FindStringSubmatchIndex(text); m != nil {
		p.style(li, from+runeOff(text, m[2]), from+runeOff(text, m[3]), KindComment)
	}
	p.styleUsageTokens(li, from, from+utf8.RuneCountInString(text))
}

// styleUsageTokens highlights flags, placeholders and brackets in a range.
func (p *parser) styleUsageTokens(li, from, to int) {
	l := p.d.Lines[li].Text
	text := l[byteOff(l, from):byteOff(l, to)]
	for _, m := range reUsageFlag.FindAllStringSubmatchIndex(text, -1) {
		s, e := from+runeOff(text, m[2]), from+runeOff(text, m[3])
		p.style(li, s, e, KindFlag)
		p.mentions = append(p.mentions, mention{li, s, e, strings.Replace(text[m[2]:m[3]], "[no-]", "", 1), true, true})
	}
	for _, m := range reAngle.FindAllStringIndex(text, -1) {
		p.style(li, from+runeOff(text, m[0]), from+runeOff(text, m[1]), KindPlaceholder)
	}
	for _, m := range reUpper.FindAllStringSubmatchIndex(text, -1) {
		p.style(li, from+runeOff(text, m[2]), from+runeOff(text, m[3]), KindPlaceholder)
	}
	for _, m := range rePunct.FindAllStringIndex(text, -1) {
		p.style(li, from+runeOff(text, m[0]), from+runeOff(text, m[1]), KindPunct)
	}
}

func fieldsIndex(s string) [][2]int {
	var out [][2]int
	start := -1
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' {
			if start >= 0 {
				out = append(out, [2]int{start, i})
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	return out
}

// docResolver lets the shell highlighter recognise this doc's subcommands and
// flags inside examples.
type docResolver struct{ p *parser }

func (r docResolver) Subcommand(path []string, word string) bool {
	if len(path) < len(r.p.path) {
		return r.p.path[len(path)] == word
	}
	if len(path) == len(r.p.path) {
		return r.p.d.FindCommand(word) != nil
	}
	return false
}

func (r docResolver) FlagTakesValue(_ []string, flag string) bool {
	f := r.p.d.FindFlag(flag)
	return f != nil && f.TakesValue()
}

var classKind = map[shell.Class]Kind{
	shell.ClassProgram:    KindProgram,
	shell.ClassSubcommand: KindCommand,
	shell.ClassFlag:       KindFlag,
	shell.ClassValue:      KindArg,
	shell.ClassString:     KindString,
	shell.ClassVariable:   KindVariable,
	shell.ClassAssign:     KindVariable,
	shell.ClassOperator:   KindOperator,
	shell.ClassComment:    KindComment,
}

func (p *parser) finish() {
	d := p.d

	// Without a commands section, fall back to the subcommands named in usage
	// lines, then to a man page's list of them.
	if len(d.Commands) == 0 {
		for _, u := range p.usage {
			if d.FindCommand(u.name) == nil {
				d.Commands = append(d.Commands, Command{Name: u.name, Line: u.line, Fallback: true})
			}
		}
		// When usage lines name the subcommands, a man page list only
		// describes them; otherwise it names them.
		fromUsage := len(p.usage) > 0
		if fromUsage || p.takesCommand() {
			for _, e := range p.definitionList() {
				if c := d.FindCommand(e.name); c != nil {
					if c.Desc == "" {
						c.Desc = e.desc
					}
					c.Aliases = appendUnique(c.Aliases, e.aliases...)
					c.Line = e.line
				} else if fromUsage {
					continue
				} else {
					d.Commands = append(d.Commands, Command{Name: e.name, Aliases: e.aliases, Desc: e.desc, Line: e.line, Fallback: true})
				}
				l := d.Lines[e.line].Text
				s, end := runeOff(l, e.indent), runeOff(l, e.termEnd)
				p.style(e.line, s, end, KindCommand)
				p.target(e.line, s, end, Target{Kind: TargetCommand, Value: e.name})
			}
		}
	}
	d.helpHint = p.usageHelp()
	for _, l := range d.Lines {
		if reHelpHint.MatchString(l.Text) {
			d.helpHint = true
		}
	}
	for _, u := range p.usage {
		if d.FindCommand(u.name) != nil {
			p.target(u.line, u.start, u.end, Target{Kind: TargetCommand, Value: u.name})
		}
	}

	for _, m := range p.mentions {
		f := d.FindFlag(m.name)
		if f == nil {
			continue
		}
		if !m.styled {
			p.style(m.line, m.start, m.end, KindFlag)
		}
		p.target(m.line, m.start, m.end, Target{Kind: TargetFlag, Value: m.name, TakesValue: f.TakesValue()})
	}

	res := docResolver{p}
	for _, ex := range p.examples {
		for i, li := range ex.lines {
			l := d.Lines[li].Text
			start := ex.start[i]
			text := string(p.runes[li][start:])
			if i > 0 || len(ex.lines) == 1 {
				text = strings.TrimSuffix(text, "\\")
			}
			// Highlight each physical line on its own; the first one starts
			// the command.
			var classes []shell.Class
			if i == 0 {
				classes = shell.Classes(text, res)
			} else {
				classes = shell.Classes("x "+text, res)[2:]
			}
			runStart := 0
			for j := 1; j <= len(classes); j++ {
				if j == len(classes) || classes[j] != classes[runStart] {
					if k, ok := classKind[classes[runStart]]; ok {
						p.style(li, start+runStart, start+j, k)
					}
					runStart = j
				}
			}
			end := utf8.RuneCountInString(strings.TrimRight(l, " \\"))
			p.target(li, start, end, Target{Kind: TargetExample, Value: ex.cmd})
		}
	}

	for i := range d.Lines {
		d.Lines[i].Styles = flatten(d.Lines[i].Styles, len(p.runes[i]))
	}
}

// flatten resolves overlapping spans (earlier ones win) into sorted,
// non-overlapping spans.
func flatten(spans []Span, n int) []Span {
	if len(spans) == 0 {
		return nil
	}
	kinds := make([]Kind, n)
	set := make([]bool, n)
	for _, s := range spans {
		for i := max(s.Start, 0); i < s.End && i < n; i++ {
			if !set[i] {
				kinds[i], set[i] = s.Kind, true
			}
		}
	}
	var out []Span
	for i := 0; i < n; {
		if !set[i] || kinds[i] == KindText {
			i++
			continue
		}
		j := i + 1
		for j < n && set[j] && kinds[j] == kinds[i] {
			j++
		}
		out = append(out, Span{i, j, kinds[i]})
		i = j
	}
	return out
}

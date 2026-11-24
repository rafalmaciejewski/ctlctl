package helpdoc

import (
	"regexp"
	"slices"
	"strings"
)

// This file finds subcommands of tools that have no commands section: BSD
// style usage lines such as "cpuctl { offline | online } <cpu>", and man page
// lists such as apachectl's "start  Loads the httpd job".

// placeholderParts are words that stand for an argument rather than name a
// subcommand: "command", "httpd-argument", "options".
var placeholderParts = map[string]bool{
	"arg": true, "args": true, "argument": true, "arguments": true,
	"option": true, "options": true, "opts": true, "flag": true, "flags": true,
	"file": true, "files": true, "filename": true, "path": true, "paths": true,
	"dir": true, "directory": true, "name": true, "value": true, "values": true,
	"command": true, "commands": true, "cmd": true, "subcommand": true, "subcommands": true,
	"action": true, "verb": true, "operation": true, "param": true, "params": true,
	"parameter": true, "parameters": true, "expr": true, "expression": true,
	"pattern": true, "string": true, "etc": true, "pathname": true,
}

func isPlaceholderWord(w string) bool {
	for _, part := range strings.FieldsFunc(w, func(r rune) bool { return r == '-' || r == '_' }) {
		if placeholderParts[part] {
			return true
		}
	}
	return false
}

type usageTok struct {
	start, end int
	s          string
}

func usageTokens(s string) []usageTok {
	const punct = "[]{}()<>|,"
	var toks []usageTok
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == ' ' || c == '\t':
			i++
		case strings.IndexByte(punct, c) >= 0:
			toks = append(toks, usageTok{i, i + 1, s[i : i+1]})
			i++
		default:
			j := i
			for j < len(s) && s[j] != ' ' && s[j] != '\t' && strings.IndexByte(punct, s[j]) < 0 {
				j++
			}
			toks = append(toks, usageTok{i, j, s[i:j]})
			i = j
		}
	}
	return toks
}

type usageAlt struct {
	start, end int // byte offsets
	word       string
}

func isOpen(s string) bool  { return s == "[" || s == "{" || s == "(" || s == "<" }
func isClose(s string) bool { return s == "]" || s == "}" || s == ")" || s == ">" }

// usageAlternatives reads the subcommands at the start of what follows the
// program name in a usage line: "list", "[ list ]", "{ offline | online }",
// "<status|enable|disable>" or "start|stop". Flag groups before them are
// skipped. It returns nothing when the first argument is a placeholder such
// as <command> or [options], or an optional choice after the flags, like
// "[-d] [codec|index]", which picks an argument rather than a subcommand.
func usageAlternatives(s string) []usageAlt {
	toks := usageTokens(s)
	afterFlags := false
	for i := 0; i < len(toks); {
		var group []usageTok
		next := i + 1
		switch t := toks[i]; {
		case isOpen(t.s):
			depth, j := 0, i
			for ; j < len(toks); j++ {
				if isOpen(toks[j].s) {
					depth++
				} else if isClose(toks[j].s) {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if j >= len(toks) {
				return nil
			}
			group, next = toks[i+1:j], j+1
			if len(group) > 0 && strings.HasPrefix(group[0].s, "-") {
				afterFlags = true
				i = next
				continue
			}
			for _, g := range group {
				if isOpen(g.s) {
					return nil
				}
			}
			if t.s == "[" && afterFlags {
				return nil
			}
			if t.s == "<" && !slices.ContainsFunc(group, func(g usageTok) bool { return g.s == "|" }) {
				return nil // <cpu> is a placeholder
			}
		case isClose(t.s) || t.s == "|" || t.s == ",":
			return nil
		default:
			j := i + 1
			group = []usageTok{t}
			for j+1 < len(toks) && toks[j].s == "|" {
				group = append(group, toks[j], toks[j+1])
				j += 2
			}
			next = j
		}

		var alts [][]usageTok
		var cur []usageTok
		for _, g := range group {
			if g.s == "|" || g.s == "," {
				alts = append(alts, cur)
				cur = nil
				continue
			}
			cur = append(cur, g)
		}
		alts = append(alts, cur)

		flags := true
		for _, a := range alts {
			if len(a) == 0 || !strings.HasPrefix(a[0].s, "-") {
				flags = false
			}
		}
		if flags {
			afterFlags = true
			i = next
			continue
		}
		var out []usageAlt
		for _, a := range alts {
			if len(a) != 1 || !reWordCmd.MatchString(a[0].s) || isPlaceholderWord(a[0].s) {
				return nil
			}
			out = append(out, usageAlt{a[0].start, a[0].end, a[0].s})
		}
		return out
	}
	return nil
}

// takesCommand reports whether a synopsis says the command takes a
// subcommand, like "apachectl command" or "cpuctl command [arguments]".
func (p *parser) takesCommand() bool {
	if p.program() == "" {
		return false
	}
	path := ""
	for _, w := range p.path[1:] {
		path += `\s+` + regexp.QuoteMeta(w)
	}
	re := regexp.MustCompile(`(?i)^\s*(?:usage:\s*)?(?:\S*/)?` + regexp.QuoteMeta(p.program()) + path +
		`\s+[\[<{]?\s*(?:sub)?(?:command|cmd|action|verb|operation)s?\s*[\]>}]?(?:\s|$)`)
	for _, l := range p.d.Lines {
		if re.MatchString(l.Text) {
			return true
		}
	}
	return false
}

type defEntry struct {
	line, indent, col int
	termEnd           int // byte offset where the term and its aliases end
	name              string
	aliases           []string
	desc              string
	section           int
}

// definitionList finds man page style lists of subcommands: a lowercase
// term, indented, with its description either on the same line at a common
// column or on the following lines, indented to that column:
//
//	start  Loads the org.apache.httpd launchd job.
//
//	stop, graceful-stop
//	       Unloads the org.apache.httpd launchd job.
//
// Only lists of two or more entries that share a section, an indent and a
// description column count, and only the first such list.
func (p *parser) definitionList() []defEntry {
	lines := p.d.Lines
	var entries []defEntry
	sec, secN := secText, 0
	for i := range lines {
		l := lines[i].Text
		if name, _, ok := header(l); ok {
			sec = classify(name)
			secN++
			continue
		}
		if sec == secUsage || sec == secExamples || sec == secCommands {
			continue
		}
		t := strings.TrimLeft(l, " ")
		indent := len(l) - len(t)
		if t == "" || indent == 0 || indent > 16 {
			continue
		}
		m := reDefTerm.FindStringSubmatchIndex(t)
		if m == nil {
			continue
		}
		rest := t[m[1]:]
		if rest != "" && rest[0] != ' ' {
			continue
		}
		e := defEntry{line: i, indent: indent, termEnd: indent + m[1], name: t[m[2]:m[3]], section: secN}
		if isPlaceholderWord(e.name) {
			continue
		}
		for _, a := range strings.Split(t[m[4]:m[5]], ",") {
			if a = strings.TrimSpace(a); a != "" {
				e.aliases = append(e.aliases, a)
			}
		}

		next := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j].Text) != "" {
				next = j
				break
			}
		}
		nextIndent := -1
		if next >= 0 {
			nt := lines[next].Text
			nextIndent = len(nt) - len(strings.TrimLeft(nt, " "))
		}
		after := strings.TrimLeft(rest, " ")
		gapLen := len(rest) - len(after)
		switch {
		case gapLen >= 2 && after != "":
			e.col = e.termEnd + gapLen
			e.desc = after
		case nextIndent > indent && (after == "" || gapLen == 1):
			e.col = nextIndent
			if after != "" && e.termEnd+1 == nextIndent {
				// The term fills the column: "status Displays a brief...".
				e.desc = after
			} else {
				if len(strings.Fields(after)) > 4 {
					continue
				}
				e.desc = strings.TrimSpace(lines[next].Text)
			}
		default:
			continue
		}
		entries = append(entries, e)
	}

	// Man pages describe the subcommands before anything else, so the first
	// list wins; later lists describe settings or states.
	type key struct{ section, indent, col int }
	count := map[key]int{}
	for _, e := range entries {
		count[key{e.section, e.indent, e.col}]++
	}
	var out []defEntry
	var first *key
	for _, e := range entries {
		k := key{e.section, e.indent, e.col}
		if count[k] < 2 || (first != nil && k != *first) {
			continue
		}
		first = &k
		out = append(out, e)
	}
	return out
}

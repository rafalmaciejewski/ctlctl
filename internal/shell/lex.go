// Package shell splits a command line the way a POSIX shell roughly would and
// classifies its parts for syntax highlighting. It never executes anything.
package shell

import "strings"

// Range is a half-open range of rune offsets.
type Range struct{ Start, End int }

// Word is one shell word, possibly made of several quoted and unquoted parts.
type Word struct {
	Range
	Raw    string  // the word as typed, quotes included
	Value  string  // the word after quote removal (best effort)
	Quoted []Range // quoted segments, quotes included
	Vars   []Range // $VAR, ${VAR} and $(...) segments
}

// ItemKind tells the parts of a command line apart.
type ItemKind uint8

const (
	ItemWord ItemKind = iota
	ItemOperator
	ItemComment
)

// Item is a word, an operator such as | or >, or a trailing comment.
type Item struct {
	Kind ItemKind
	Word Word // set for ItemWord; Range and Raw are set for every kind
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }

func isOpChar(r rune) bool { return r == '|' || r == '&' || r == ';' || r == '<' || r == '>' }

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// Split breaks s into words, operators and comments. Offsets are rune offsets.
// Unterminated quotes run to the end of the input instead of failing, because
// the input is usually still being typed.
func Split(s string) []Item {
	rs := []rune(s)
	n := len(rs)
	var items []Item
	i := 0
	for i < n {
		r := rs[i]
		switch {
		case isSpace(r):
			i++
		case r == '#':
			items = append(items, Item{Kind: ItemComment, Word: Word{Range: Range{i, n}, Raw: string(rs[i:])}})
			i = n
		case isOpChar(r) || (isDigit(r) && i+1 < n && (rs[i+1] == '>' || rs[i+1] == '<') && startsWord(rs, i)):
			start := i
			if isDigit(r) {
				i++
			}
			for i < n && isOpChar(rs[i]) {
				i++
				// Redirections like >&2 and 2>&1 end with a file descriptor.
				if rs[i-1] == '&' && i < n && (isDigit(rs[i]) || rs[i] == '-') && start < i-1 {
					for i < n && (isDigit(rs[i]) || rs[i] == '-') {
						i++
					}
					break
				}
			}
			items = append(items, Item{Kind: ItemOperator, Word: Word{Range: Range{start, i}, Raw: string(rs[start:i])}})
		default:
			w, next := lexWord(rs, i)
			items = append(items, Item{Kind: ItemWord, Word: w})
			i = next
		}
	}
	return items
}

func startsWord(rs []rune, i int) bool { return i == 0 || isSpace(rs[i-1]) }

func lexWord(rs []rune, start int) (Word, int) {
	n := len(rs)
	w := Word{Range: Range{Start: start}}
	var val strings.Builder
	i := start
	for i < n {
		r := rs[i]
		if isSpace(r) || isOpChar(r) {
			break
		}
		switch r {
		case '\'':
			q := i
			i++
			for i < n && rs[i] != '\'' {
				val.WriteRune(rs[i])
				i++
			}
			if i < n {
				i++
			}
			w.Quoted = append(w.Quoted, Range{q, i})
		case '"':
			q := i
			i++
			for i < n && rs[i] != '"' {
				if rs[i] == '\\' && i+1 < n {
					i++
				} else if rs[i] == '$' {
					v := lexVar(rs, i)
					if v > i+1 {
						w.Vars = append(w.Vars, Range{i, v})
						val.WriteString(string(rs[i:v]))
						i = v
						continue
					}
				}
				val.WriteRune(rs[i])
				i++
			}
			if i < n {
				i++
			}
			w.Quoted = append(w.Quoted, Range{q, i})
		case '\\':
			if i+1 < n {
				val.WriteRune(rs[i+1])
				i += 2
			} else {
				i++
			}
		case '$':
			v := lexVar(rs, i)
			if v > i+1 {
				w.Vars = append(w.Vars, Range{i, v})
				val.WriteString(string(rs[i:v]))
				i = v
			} else {
				val.WriteRune(r)
				i++
			}
		default:
			val.WriteRune(r)
			i++
		}
	}
	w.End = i
	w.Raw = string(rs[start:i])
	w.Value = val.String()
	return w, i
}

// lexVar returns the end offset of a variable reference starting at rs[i]=='$',
// or i+1 when the dollar sign does not start one.
func lexVar(rs []rune, i int) int {
	n := len(rs)
	j := i + 1
	if j >= n {
		return j
	}
	switch rs[j] {
	case '{':
		for j < n && rs[j] != '}' {
			j++
		}
		if j < n {
			j++
		}
		return j
	case '(':
		depth := 0
		for j < n {
			if rs[j] == '(' {
				depth++
			} else if rs[j] == ')' {
				depth--
				if depth == 0 {
					return j + 1
				}
			}
			j++
		}
		return j
	}
	if isVarStart(rs[j]) {
		for j < n && isVarChar(rs[j]) {
			j++
		}
		return j
	}
	if isDigit(rs[j]) || strings.ContainsRune("?!@*#$-", rs[j]) {
		return j + 1
	}
	return i + 1
}

func isVarStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isVarChar(r rune) bool { return isVarStart(r) || isDigit(r) }

// Fields returns the unquoted values of the words in s, ignoring operators and
// comments. It is meant for resolving which command a line runs.
func Fields(s string) []string {
	var out []string
	for _, it := range Split(s) {
		if it.Kind == ItemWord {
			out = append(out, it.Word.Value)
		}
	}
	return out
}

// Quote returns s quoted for a POSIX shell when it needs quoting.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(isVarChar(r) || strings.ContainsRune("-_./:=,@%+", r)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

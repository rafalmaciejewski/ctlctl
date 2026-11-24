package shell

import (
	"path/filepath"
	"strings"
)

// Class is the syntax class of a part of a command line.
type Class uint8

const (
	ClassNone Class = iota
	ClassProgram
	ClassSubcommand
	ClassFlag
	ClassValue
	ClassString
	ClassVariable
	ClassOperator
	ClassComment
	ClassArg
	ClassAssign
)

// Span gives a class to a half-open range of rune offsets. Spans returned by
// Highlight may overlap; later spans take precedence.
type Span struct {
	Range
	Class Class
}

// Resolver tells Highlight what it knows about a command's structure.
type Resolver interface {
	// Subcommand reports whether word is a subcommand of the command at path.
	// path starts with the program's base name.
	Subcommand(path []string, word string) bool
	// FlagTakesValue reports whether flag, at path, consumes the next word.
	FlagTakesValue(path []string, flag string) bool
}

// NopResolver knows nothing about any command.
type NopResolver struct{}

func (NopResolver) Subcommand([]string, string) bool     { return false }
func (NopResolver) FlagTakesValue([]string, string) bool { return false }

// Highlight classifies every part of the command line s.
func Highlight(s string, r Resolver) []Span {
	if r == nil {
		r = NopResolver{}
	}
	var spans []Span
	cmdStart := true
	var path []string
	subPhase := false
	expectValue := false
	redirect := false
	endOfFlags := false
	for _, it := range Split(s) {
		switch it.Kind {
		case ItemComment:
			spans = append(spans, Span{it.Word.Range, ClassComment})
			continue
		case ItemOperator:
			spans = append(spans, Span{it.Word.Range, ClassOperator})
			op := it.Word.Raw
			if strings.ContainsAny(op, "<>") {
				// 2>&1 and >&2 name a file descriptor, not a file.
				last := op[len(op)-1]
				redirect = !(isDigit(rune(last)) || last == '-') || !strings.Contains(op, "&")
			} else {
				cmdStart, path, subPhase, expectValue, endOfFlags, redirect = true, nil, false, false, false, false
			}
			continue
		}
		w := it.Word
		switch {
		case redirect:
			redirect = false
			spans = append(spans, Span{w.Range, ClassArg})
		case cmdStart && isAssignment(w.Raw):
			eq := strings.IndexRune(w.Raw, '=')
			spans = append(spans, Span{Range{w.Start, w.Start + len([]rune(w.Raw[:eq]))}, ClassAssign})
			spans = append(spans, Span{Range{w.Start + len([]rune(w.Raw[:eq])) + 1, w.End}, ClassValue})
		case cmdStart:
			spans = append(spans, Span{w.Range, ClassProgram})
			path = []string{filepath.Base(w.Value)}
			cmdStart, subPhase = false, true
		case expectValue:
			expectValue = false
			spans = append(spans, Span{w.Range, ClassValue})
		case !endOfFlags && w.Raw == "--":
			endOfFlags = true
			spans = append(spans, Span{w.Range, ClassFlag})
		case !endOfFlags && len(w.Raw) > 1 && w.Raw[0] == '-' && !isNumber(w.Raw):
			if eq := strings.IndexRune(w.Raw, '='); eq > 0 {
				eqr := w.Start + len([]rune(w.Raw[:eq]))
				spans = append(spans, Span{Range{w.Start, eqr}, ClassFlag})
				spans = append(spans, Span{Range{eqr + 1, w.End}, ClassValue})
			} else {
				spans = append(spans, Span{w.Range, ClassFlag})
				expectValue = r.FlagTakesValue(path, w.Value)
			}
		case subPhase && r.Subcommand(path, w.Value):
			spans = append(spans, Span{w.Range, ClassSubcommand})
			path = append(path, w.Value)
		default:
			subPhase = false
			spans = append(spans, Span{w.Range, ClassArg})
		}
		for _, q := range w.Quoted {
			spans = append(spans, Span{q, ClassString})
		}
		for _, v := range w.Vars {
			spans = append(spans, Span{v, ClassVariable})
		}
	}
	return spans
}

// Classes flattens Highlight's spans into one class per rune of s.
func Classes(s string, r Resolver) []Class {
	n := len([]rune(s))
	out := make([]Class, n)
	for _, sp := range Highlight(s, r) {
		for i := max(sp.Start, 0); i < sp.End && i < n; i++ {
			out[i] = sp.Class
		}
	}
	return out
}

func isAssignment(raw string) bool {
	eq := strings.IndexRune(raw, '=')
	if eq <= 0 {
		return false
	}
	rs := []rune(raw[:eq])
	if !isVarStart(rs[0]) {
		return false
	}
	for _, r := range rs {
		if !isVarChar(r) {
			return false
		}
	}
	return true
}

func isNumber(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isDigit(r) && r != '.' {
			return false
		}
	}
	return true
}

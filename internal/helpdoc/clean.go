package helpdoc

import (
	"regexp"
	"strings"
	"unicode"
)

var ansiRe = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[@-Z\\-_])`)

// Clean turns raw help output into plain text: it removes ANSI escapes and
// man-page overstrikes, expands tabs and drops other control characters and
// leading and trailing blank lines.
func Clean(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = ansiRe.ReplaceAllString(s, "")
	if strings.ContainsRune(s, '\b') {
		s = stripOverstrike(s)
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.LastIndexByte(l, '\r'); j >= 0 {
			l = l[j+1:]
		}
		l = expandTabs(l)
		l = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, l)
		lines[i] = strings.TrimRight(l, " ")
	}
	// man pages separate paragraphs with several blank lines.
	out := make([]string, 0, len(lines))
	for i, l := range lines {
		if l == "" && (i == 0 || lines[i-1] == "") {
			continue
		}
		out = append(out, l)
	}
	lines = out
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// stripOverstrike resolves "X\bX" (bold) and "_\bX" (underline) to "X".
func stripOverstrike(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\b' {
			if len(out) > 0 && out[len(out)-1] != '\n' {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func expandTabs(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// Normalize collapses whitespace so two help texts can be compared.
func Normalize(s string) string {
	return strings.Join(strings.Fields(Clean(s)), " ")
}

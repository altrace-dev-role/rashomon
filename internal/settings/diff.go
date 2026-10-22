package settings

import (
	"fmt"
	"strings"
)

// Elided stands in for every JSON value a ValuesFreeDiff line had: each
// string that is not an object key, and each number, true, false and null.
const Elided = "<v>"

// diffMaxLines bounds what ValuesFreeDiff prints; the rest is counted.
const diffMaxLines = 40

// diffMaxCells bounds the line-by-line comparison table. Past it the diff
// is one hunk from the first differing line to the last, which is still
// correct, only less precise about what in between was the same.
const diffMaxCells = 4 << 20

// ValuesFreeDiff renders a line diff from a to b in which no value from
// either file survives: what is left of a line is its keys, its punctuation
// and its whitespace, which is what a layout the editor did not keep differs
// in. Settings files hold credentials (env, apiKeyHelper), and the diff is
// printed for a bug report that gets pasted somewhere public.
//
// Lines are prefixed "- " (only in a) and "+ " (only in b). Tabs, carriage
// returns, trailing spaces and every byte outside printable ASCII are shown
// escaped, so a difference in them is visible and a key cannot carry a
// terminal control sequence through.
func ValuesFreeDiff(a, b []byte) []string {
	x := strings.Split(string(a), "\n")
	y := strings.Split(string(b), "\n")
	var out []string
	emit := func(prefix, line string) { out = append(out, prefix+renderLine(line)) }

	if len(x)*len(y) > diffMaxCells {
		i := 0
		for i < len(x) && i < len(y) && x[i] == y[i] {
			i++
		}
		j, k := len(x), len(y)
		for j > i && k > i && x[j-1] == y[k-1] {
			j, k = j-1, k-1
		}
		for _, l := range x[i:j] {
			emit("- ", l)
		}
		for _, l := range y[i:k] {
			emit("+ ", l)
		}
		return capLines(out)
	}

	// lcs[i][j] is the longest common subsequence of x[i:] and y[j:].
	lcs := make([][]int32, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			i, j = i+1, j+1
		case j < len(y) && (i == len(x) || lcs[i][j+1] >= lcs[i+1][j]):
			emit("+ ", y[j])
			j++
		default:
			emit("- ", x[i])
			i++
		}
	}
	return capLines(out)
}

func capLines(lines []string) []string {
	if len(lines) <= diffMaxLines {
		return lines
	}
	rest := len(lines) - diffMaxLines
	return append(lines[:diffMaxLines:diffMaxLines], fmt.Sprintf("  (%d more lines differ)", rest))
}

// renderLine elides a line's values and escapes what is left.
func renderLine(line string) string {
	masked := elideValues(line)
	trimmed := strings.TrimRight(masked, " \t\r")
	var b strings.Builder
	for i := 0; i < len(masked); i++ {
		c := masked[i]
		switch {
		case c == ' ' && i >= len(trimmed):
			b.WriteString(`\x20`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\r':
			b.WriteString(`\r`)
		case c < 0x20 || c >= 0x7f:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// elideValues replaces every scalar value on one line of JSON with Elided.
// A string followed, after spaces, by a colon is a key and is kept. It is a
// lexer, not a parser: a line of a valid file is all it is ever given, and a
// string that does not close on its line is elided to the end of the line.
func elideValues(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case c == '"':
			end := closingQuote(line, i)
			if end < 0 {
				b.WriteString(Elided)
				return b.String()
			}
			k := end + 1
			for k < len(line) && (line[k] == ' ' || line[k] == '\t') {
				k++
			}
			if k < len(line) && line[k] == ':' {
				b.WriteString(line[i : end+1])
			} else {
				b.WriteString(Elided)
			}
			i = end + 1
		case c == '-' || (c >= '0' && c <= '9') || c == 't' || c == 'f' || c == 'n':
			j := i
			for j < len(line) && strings.IndexByte("{}[],: \t\r", line[j]) < 0 {
				j++
			}
			b.WriteString(Elided)
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// closingQuote returns the index of the quote that closes the string opening
// at line[open], or -1 if it does not close on this line.
func closingQuote(line string, open int) int {
	for i := open + 1; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

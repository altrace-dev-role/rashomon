package pylite

import "strings"

// maxNest bounds how deep blank follows a string nested in an f-string
// field, which may hold a field in turn (Python 3.12 lets a field hold a
// string of the same quote). Past it the rest of the file is blanked. One
// level of nesting takes two steps here, and CPython stops at 150 levels.
const maxNest = 400

// blank is src with the inside of every string literal and of every comment
// replaced by spaces, before any line is read, so what a string or a comment
// holds reaches no record. Each newline and carriage return stays where it
// is and every other byte becomes one space, so no line and no column
// moves; the quotes, a string's prefix and the # that opens a comment stay,
// so a call keeps its argument count and a comment line stays one.
//
// The extents are found as Python's tokenizer finds them, not by its
// grammar: an optional prefix, a quote or three of one kind, a backslash
// taking the byte after it (in a raw string too, as Python finds a raw
// string's end), and a # outside a string running to its line end. In an
// f-string or a t-string the fields are read only to find where they and
// the string close, a string nested in a field included, and they are
// blanked with the string: a call in a field is not seen. A string in one
// quote that never closes ends at its line end; a triple-quoted one, or a
// field, that never closes runs to the end of the file.
func blank(src string) string {
	b := []byte(src)
	for i := 0; i < len(src); {
		switch src[i] {
		case '#':
			e := lineEnd(src, i)
			spaces(b, i+1, e)
			i = e
		case '\'', '"':
			from, to, next := quoted(src, i, 0)
			spaces(b, from, to)
			i = next
		default:
			i++
		}
	}
	return string(b)
}

// spaces blanks b[from:to] but for its line ends.
func spaces(b []byte, from, to int) {
	for k := from; k < to; k++ {
		if b[k] != '\n' && b[k] != '\r' {
			b[k] = ' '
		}
	}
}

// lineEnd is the index of the first line end in s at or after i, or len(s).
func lineEnd(s string, i int) int {
	if k := strings.IndexAny(s[i:], "\r\n"); k >= 0 {
		return i + k
	}
	return len(s)
}

// quoted reads the string literal whose opening quote is s[i]: its inside
// is s[from:to], and next is the index after its closing quotes, or to when
// it does not close.
func quoted(s string, i, lvl int) (from, to, next int) {
	n := 1
	if i+2 < len(s) && s[i+1] == s[i] && s[i+2] == s[i] {
		n = 3
	}
	cl := s[i : i+n]
	from = i + n
	to = inside(s, from, cl, fstring(s, i), lvl)
	next = to
	if strings.HasPrefix(s[to:], cl) {
		next = to + n
	}
	return from, to, next
}

// inside is the index where the inside of a string that starts at s[j]
// ends, cl being its closing quotes: the index of those quotes, the line end
// of a string in one quote that does not close, or len(s). In an f-string
// (f) a brace opens a field and a doubled brace is text, and a backslash
// takes no brace after it, as Python reads it.
func inside(s string, j int, cl string, f bool, lvl int) int {
	for j < len(s) {
		c := s[j]
		switch {
		case c == '\\' && f && braceAfter(s, j):
			j++
		case c == '\\':
			j = escape(s, j)
		case c == '\n' || c == '\r':
			if len(cl) == 1 {
				return j
			}
			j++
		case strings.HasPrefix(s[j:], cl):
			return j
		case f && strings.HasPrefix(s[j:], "{{"):
			j += 2
		case f && c == '{':
			j = field(s, j+1, cl, lvl+1)
		default:
			j++
		}
	}
	return len(s)
}

// field is the index after the f-string field whose expression starts at
// s[j], cl being the closing quotes of the string that holds it. The
// expression's brackets are counted, a string in it is read as a string,
// a # in it runs to its line end, and a colon outside its brackets opens the
// format spec.
func field(s string, j int, cl string, lvl int) int {
	if lvl > maxNest {
		return len(s)
	}
	depth := 0
	for j < len(s) {
		c := s[j]
		switch {
		case c == '\'' || c == '"':
			_, _, j = quoted(s, j, lvl+1)
		case c == '#':
			j = lineEnd(s, j)
		case c == '(' || c == '[' || c == '{':
			depth++
			j++
		case (c == ')' || c == ']' || c == '}') && depth > 0:
			depth--
			j++
		case c == '}':
			return j + 1
		case c == ':' && depth == 0:
			return spec(s, j+1, cl, lvl)
		default:
			j++
		}
	}
	return len(s)
}

// spec is the index after the field whose format spec starts at s[j]. A
// spec is text: a quote or a # in it opens nothing, a brace in it opens a
// nested field, and in a string in one quote it ends at a line end.
func spec(s string, j int, cl string, lvl int) int {
	for j < len(s) {
		c := s[j]
		switch {
		case c == '\\' && braceAfter(s, j):
			j++
		case c == '\\':
			j = escape(s, j)
		case c == '\n' || c == '\r':
			if len(cl) == 1 {
				return j
			}
			j++
		case strings.HasPrefix(s[j:], cl):
			return j
		case c == '{':
			j = field(s, j+1, cl, lvl+1)
		case c == '}':
			return j + 1
		default:
			j++
		}
	}
	return len(s)
}

// escape is the index after the backslash at s[j] and the byte it takes, a
// CRLF line end taken whole.
func escape(s string, j int) int {
	if strings.HasPrefix(s[j+1:], "\r\n") {
		return j + 3
	}
	return min(j+2, len(s))
}

// braceAfter reports whether a brace follows the backslash at s[j].
func braceAfter(s string, j int) bool {
	return j+1 < len(s) && (s[j+1] == '{' || s[j+1] == '}')
}

// fstring reports whether the quote at s[i] opens an f-string or a
// t-string: the word right before it, a prefix of one or two letters, is
// f or t, alone or with r.
func fstring(s string, i int) bool {
	k := i
	for k > 0 && i-k < 3 && wordByte(s[k-1]) {
		k--
	}
	switch strings.ToLower(s[k:i]) {
	case "f", "t", "fr", "rf", "tr", "rt":
		return true
	}
	return false
}

// wordByte reports whether c may be part of a Python name: a letter, a
// digit, an underscore, or any byte of a character beyond ASCII.
func wordByte(c byte) bool {
	return c == '_' || c >= 0x80 || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

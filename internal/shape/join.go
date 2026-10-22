package shape

import "strings"

// joinContinuations removes every backslash-newline the shell removes before
// it splits a line into words: outside quotes and inside double quotes, but
// not inside single quotes, where a backslash is literal. It returns the
// line with them removed and the offsets in it at which it removed one,
// ascending: the byte there is the one that followed it. Its quote tracking
// is flat, which is right outside a $( ) and may be wrong inside one; see
// comsubEnd.
//
// It keeps the first maxJoins offsets and no more (joined.lost): one per
// continuation, 8 MiB of `a\<newline>` kept 2.8 million of them, 22 MiB,
// in each reading of the line. The line itself is joined whole.
func joinContinuations(s string) joined {
	return joinLines(s, false)
}

// maxJoins is how many offsets joinContinuations keeps: as many as the
// token bound lets a line have tokens (RES-9), 340 KB of them. Past them the
// readers that ask where a backslash-newline was removed, a here-document's
// body and a $( ) inside double quotes, give up rather than guess.
const maxJoins = maxTokens

// joinLines is joinContinuations; with comments, an unquoted # at the start
// of a word runs to the end of its line, as a comment does to the shell, and
// a backslash at its end continues nothing: `# run the tests \` and then
// `make test | tail` on the next line runs make.
func joinLines(s string, comments bool) joined {
	n := strings.Count(s, "\\\n")
	if n == 0 {
		return joined{s: s}
	}
	var (
		b     strings.Builder
		joins = make([]int, 0, min(n, maxJoins))
		lost  bool
	)
	// The line loses bytes, except where a comment or a single quote keeps
	// a backslash-newline, so it is grown once to the line's length.
	b.Grow(len(s))
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case comments && c == '#' && !inSingle && !inDouble && wordStart(b.String()):
			for ; i < len(s) && s[i] != '\n'; i++ {
				b.WriteByte(s[i])
			}
			i--
			continue
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case c == '\\' && i+1 < len(s):
			if s[i+1] == '\n' {
				if len(joins) < maxJoins {
					joins = append(joins, b.Len())
				} else {
					lost = true
				}
				i++
				continue
			}
			b.WriteByte(c)
			i++
			c = s[i]
		case c == '\'' && !inDouble:
			inSingle = true
		case c == '"':
			inDouble = !inDouble
		}
		b.WriteByte(c)
	}
	return joined{s: b.String(), joins: joins, lost: lost}
}

// wordStart reports whether a word would begin after the text written so
// far: at its start, or after a blank or an operator.
func wordStart(written string) bool {
	if written == "" {
		return true
	}
	switch written[len(written)-1] {
	case ' ', '\t', '\n', ';', '&', '|', '(', ')', '<', '>':
		return true
	}
	return false
}

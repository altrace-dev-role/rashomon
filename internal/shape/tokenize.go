package shape

import (
	"errors"
	"strings"
)

// errUnterminated is the only error this file returns, and it is a fixed
// sentinel. An error that quoted the offending input would put a fragment of a
// command line into an error string, which is the shortest path from here to a
// log file.
var errUnterminated = errors.New("unterminated quote")

// tokenize splits a command line the way a POSIX shell would split it, as far
// as quoting and metacharacters go. It does not expand anything: no variables,
// no globs, no command substitution. Expansion would mean evaluating the input,
// and the count of tokens before expansion is both well defined and the only
// count obtainable without running anything.
//
// Argument count is over the whole line, so a pipeline counts its operators and
// every stage. `ls | wc -l` is four tokens. The first token is the program,
// which is why unquoted metacharacters have to break a token: without that,
// `ls|wc` would name a program that does not exist.
//
// On an unterminated quote it returns the tokens completed so far along with
// the error, so a caller can still name the program of a line it could not
// finish counting. The token the quote interrupted is not one of them: it was
// never completed, and for a double quote emitting it would carry bytes from
// inside the quoted string out to the program name.
func tokenize(s string) ([]string, error) {
	var (
		toks    []string
		cur     strings.Builder
		started bool
	)

	flush := func() {
		if started {
			toks = append(toks, cur.String())
			cur.Reset()
			started = false
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()

		case c == '\\':
			if i+1 >= len(s) {
				flush()
				return toks, errUnterminated
			}
			i++
			if s[i] != '\n' { // a backslash-newline is a line continuation
				cur.WriteByte(s[i])
				started = true
			}

		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return toks, errUnterminated
			}
			cur.WriteString(s[i+1 : i+1+j])
			started = true
			i += j + 1

		case c == '"':
			var closed bool
			i++
			for ; i < len(s); i++ {
				if s[i] == '\\' && i+1 < len(s) {
					switch n := s[i+1]; n {
					case '"', '\\', '$', '`':
						cur.WriteByte(n)
						i++
						continue
					case '\n':
						i++
						continue
					}
					cur.WriteByte('\\')
					continue
				}
				if s[i] == '"' {
					closed = true
					break
				}
				cur.WriteByte(s[i])
			}
			if !closed {
				return toks, errUnterminated
			}
			started = true

		case isMeta(c):
			flush()
			j := i + 1
			if j < len(s) && s[j] == c {
				switch c {
				case '|', '&', '>', '<':
					j++
				}
			}
			toks = append(toks, s[i:j])
			i = j - 1

		default:
			cur.WriteByte(c)
			started = true
		}
	}

	flush()
	return toks, nil
}

func isMeta(c byte) bool {
	switch c {
	case '|', '&', ';', '<', '>', '(', ')':
		return true
	}
	return false
}

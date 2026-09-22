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
	toks, _, err := tokenizeMarked(s)
	return toks, err
}

// tokenizeMarked is tokenize plus one bit per token: whether the tokenizer
// emitted it for an unquoted metacharacter.
//
// The bit cannot be recovered from the token text afterwards. A quoted ';' and
// an operator ';' are the same two bytes, so a reader that re-derives the
// answer by looking at the string treats `echo ';' ssh host` as though a new
// command began, and reads ssh as a program that was never run. Only the
// tokenizer knows which it saw, so only the tokenizer can say.
func tokenizeMarked(s string) ([]string, []bool, error) {
	toks, meta, _, err := tokenizeQuoted(s)
	return toks, meta, err
}

// tokenizeQuoted is tokenizeMarked plus, per token, the byte offset within the
// token at which quoting or a backslash escape first contributed, or -1 if
// none did.
//
// Like the metacharacter bit, this cannot be recovered from the text. `{` and
// '{' are the same byte once the quotes are gone, and only the first is the
// brace keyword; FOO=bar and 'FOO=bar' are the same seven, and only the first
// is an assignment -- the second is a command by that name, and the word after
// it is an argument. An offset rather than a bit, because FOO="a b" is still
// an assignment: what matters is whether the quoting starts after the `=`.
func tokenizeQuoted(s string) ([]string, []bool, []int, error) {
	var (
		toks    []string
		meta    []bool
		quoted  []int
		cur     strings.Builder
		started bool
		qat     = -1
	)

	// markQuoted records where quoting first touched the current token.
	markQuoted := func() {
		if qat < 0 {
			qat = cur.Len()
		}
	}
	flush := func() {
		if started {
			toks = append(toks, cur.String())
			meta = append(meta, false)
			quoted = append(quoted, qat)
			cur.Reset()
			started = false
			qat = -1
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
				return toks, meta, quoted, errUnterminated
			}
			i++
			if s[i] != '\n' { // a backslash-newline is a line continuation
				markQuoted()
				cur.WriteByte(s[i])
				started = true
			}

		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return toks, meta, quoted, errUnterminated
			}
			markQuoted()
			cur.WriteString(s[i+1 : i+1+j])
			started = true
			i += j + 1

		case c == '"':
			var closed bool
			markQuoted()
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
				return toks, meta, quoted, errUnterminated
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
			meta = append(meta, true)
			quoted = append(quoted, -1)
			i = j - 1

		default:
			cur.WriteByte(c)
			started = true
		}
	}

	flush()
	return toks, meta, quoted, nil
}

func isMeta(c byte) bool {
	switch c {
	case '|', '&', ';', '<', '>', '(', ')':
		return true
	}
	return false
}

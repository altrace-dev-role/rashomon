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

// token is one word or operator of a command line as the tokenizer saw it.
//
// Each field is something the text alone cannot tell. meta: a QUOTED `;` and
// an operator `;` are the same byte. quotedAt: `{` and '{' are the same byte
// once the quotes are gone, and only the first is the brace keyword; FOO=bar
// and 'FOO=bar' are the same seven, and only the first is an assignment --
// an offset rather than a bit, because FOO="a b" is still an assignment.
// opaque: the word holds an expansion this tokenizer does not parse -- $( ),
// ${ }, $[ ], a backtick, or $” and $"" quoting -- so where the shell would
// end the word is not where this tokenizer ended it. "$(cat "a b")" is one
// shell word and three tokens here.
type token struct {
	text     string
	meta     bool // emitted for an unquoted metacharacter
	quotedAt int  // offset of the first quoted or escaped byte, or -1
	opaque   bool
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
	ts, err := tokenizeShape(s)
	toks := make([]string, len(ts))
	meta := make([]bool, len(ts))
	for i, t := range ts {
		toks[i], meta[i] = t.text, t.meta
	}
	return toks, meta, err
}

// tokenizeShape splits a command line as tokenize does and reports, per
// token, what the text cannot: see token.
func tokenizeShape(s string) ([]token, error) {
	var (
		toks    []token
		cur     strings.Builder
		started bool
		qat     = -1
		opaque  bool
	)

	// markQuoted records where quoting first touched the current token.
	markQuoted := func() {
		if qat < 0 {
			qat = cur.Len()
		}
	}
	// expands reports whether s[i] opens an expansion this tokenizer does
	// not parse. inDouble: inside "", $'' and $"" are not quoting forms.
	expands := func(i int, inDouble bool) bool {
		if s[i] == '`' {
			return true
		}
		if s[i] != '$' || i+1 >= len(s) {
			return false
		}
		switch s[i+1] {
		case '(', '{', '[':
			return true
		case '\'', '"':
			return !inDouble
		}
		return false
	}
	flush := func() {
		if started {
			toks = append(toks, token{text: cur.String(), quotedAt: qat, opaque: opaque})
			cur.Reset()
			started = false
			qat = -1
			opaque = false
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
				markQuoted()
				cur.WriteByte(s[i])
				started = true
			}

		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return toks, errUnterminated
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
				if expands(i, true) {
					opaque = true
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
			toks = append(toks, token{text: s[i:j], meta: true, quotedAt: -1})
			i = j - 1

		default:
			if expands(i, false) {
				opaque = true
			}
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

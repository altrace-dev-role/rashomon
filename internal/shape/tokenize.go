package shape

import (
	"errors"
	"strings"
)

// errUnterminated and errUncertain are the only errors this file returns, and
// both are fixed sentinels. An error that quoted the offending input would put
// a fragment of a command line into an error string, which is the shortest
// path from here to a log file.
var errUnterminated = errors.New("unterminated quote")

// errUncertain is tokenizeProgram's stop at a point from which its reading of
// the line may not be the shell's: a quote bash and zsh read differently, or
// an unterminated quote after an expansion lex does not parse, which may be no
// quote at all to the shell -- "$(echo '"')" is one word. What follows is
// unknown rather than absent, and the tokens are those completed before it.
var errUncertain = errors.New("uncertain reading")

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
	// nlBefore: an unquoted newline came between the previous token and this
	// one. The shell ends a command there; this tokenizer only splits a word.
	nlBefore bool
	// glued: nothing -- no blank, no newline -- came between the previous
	// token and this one. `>|` is one operator and `> |` two; `acme-<1-3>` is
	// one word to zsh only because nothing separates its pieces. The text of
	// neither token says which. False for a line's first token.
	glued bool
	// ticks: how many backticks in the word open or close a command
	// substitution -- unquoted or inside double quotes, and not escaped. An
	// odd count leaves one open at the word's end. '`' and \` are in the
	// text too, and open nothing.
	ticks int
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
	return lex(s, false)
}

// tokenizeProgram is tokenizeShape for the program search, which reads one
// construct as the shell reads it rather than as argc always has: a $'...'
// string ends at its first unescaped quote. tokenizeShape ends it at the first
// quote of any kind, so a $'...' holding an escaped quote is an open quote to
// it and one closed word to the shell -- and what the shell reads next, a `()`
// that makes the line a function definition, it never saw. argc keeps the old
// reading: a count that moved would count the same command differently
// depending on which build recorded it.
//
// Where it cannot tell how the shell reads on, it stops with errUncertain
// rather than guess: see lex.
func tokenizeProgram(s string) ([]token, error) {
	return lex(s, true)
}

// lex is tokenizeShape and tokenizeProgram; program selects the second's
// reading of $'...', and its errUncertain stops:
//
//   - $$' -- bash reads the pair $$, the shell's pid, and then an ordinary
//     quote; zsh reads $ and then a $'...' string. An odd run of $ is $'...'
//     to both. The two readings can end the word in different places.
//   - an unterminated quote on a line that already held an expansion lex
//     does not parse, whose quoting it may have misread.
func lex(s string, program bool) ([]token, error) {
	var (
		toks    []token
		cur     strings.Builder
		started bool
		qat     = -1
		opaque  bool
		ticks   int
		sawNL   bool
		// gap: a blank or newline since the last token ended. glued: the
		// word being built began with no gap before it.
		gap, glued bool
		// dollarAt: the index of the last unquoted, unescaped `$`, and
		// dollars: how many of them run together up to it.
		dollarAt = -1
		dollars  int
		// expanded: a token already emitted was opaque.
		expanded bool
	)

	// begin marks the current word started, noting at its first byte
	// whether it is glued to the token before it.
	begin := func() {
		if !started {
			glued = !gap && len(toks) > 0
			started = true
		}
	}
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
	// unterminated is the error for a quote that never closes. For the
	// program search, after an expansion it is uncertain: see lex.
	unterminated := func() error {
		if program && (opaque || expanded) {
			return errUncertain
		}
		return errUnterminated
	}
	flush := func() {
		if started {
			toks = append(toks, token{text: cur.String(), quotedAt: qat, opaque: opaque, nlBefore: sawNL, glued: glued, ticks: ticks})
			expanded = expanded || opaque
			sawNL, gap = false, false
			cur.Reset()
			started = false
			qat = -1
			opaque = false
			ticks = 0
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
			gap = true
			if c == '\n' {
				sawNL = true
			}

		case c == '\\':
			if i+1 >= len(s) {
				flush()
				return toks, unterminated()
			}
			i++
			if s[i] != '\n' { // a backslash-newline is a line continuation
				begin()
				markQuoted()
				cur.WriteByte(s[i])
			}

		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if program && dollarAt >= 0 && dollarAt == i-1 {
				if dollars%2 == 0 {
					// $$': the shells disagree. See lex.
					return toks, errUncertain
				}
				// $'...', which a backslash-escaped quote does not end.
				j = ansiCEnd(s[i+1:])
			}
			if j < 0 {
				return toks, unterminated()
			}
			begin()
			markQuoted()
			cur.WriteString(s[i+1 : i+1+j])
			i += j + 1

		case c == '"':
			var closed bool
			begin()
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
				if s[i] == '`' {
					ticks++
				}
				cur.WriteByte(s[i])
			}
			if !closed {
				return toks, unterminated()
			}

		case isMeta(c):
			flush()
			j := i + 1
			if j < len(s) && s[j] == c {
				switch c {
				case '|', '&', '>', '<':
					j++
				}
			}
			toks = append(toks, token{text: s[i:j], meta: true, quotedAt: -1, nlBefore: sawNL, glued: !gap && len(toks) > 0})
			sawNL, gap = false, false
			i = j - 1

		default:
			if expands(i, false) {
				opaque = true
			}
			if c == '$' {
				if dollarAt == i-1 {
					dollars++
				} else {
					dollars = 1
				}
				dollarAt = i
			}
			if c == '`' {
				ticks++
			}
			begin()
			cur.WriteByte(c)
		}
	}

	flush()
	return toks, nil
}

// ansiCEnd returns the index of the quote that closes a $'...' string whose
// body s begins -- the first one no backslash escapes -- or -1 if none does.
func ansiCEnd(s string) int {
	for j := 0; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '\'':
			return j
		}
	}
	return -1
}

func isMeta(c byte) bool {
	switch c {
	case '|', '&', ';', '<', '>', '(', ')':
		return true
	}
	return false
}

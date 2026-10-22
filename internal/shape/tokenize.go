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

// errTokenBound is lex's stop at maxTokens: the line goes on past what this
// package will read, and what follows is unknown rather than absent. The
// tokens are the first maxTokens, so a program named before the bound is
// still named.
var errTokenBound = errors.New("token bound")

// The token bound (RES-9). A hook payload is read up to 8 MiB, which bounds
// the bytes and not the work: 8 MiB of one-letter words is four million
// tokens, and lexed whole the shape of that one line allocated 4.5 GB and
// drove a hook to about 1.07 GB of RSS. Lexing stops at maxTokens, derived
// from a memory budget for the shape of one call, shapeBudget, over the cost
// of one token. That cost is the whole Derive's, not the lexer's: Derive reads
// a line three times (tokenizeShape for argc, tokenizeProgram for the program
// and the leading-cd count, tokenizeList for the masked status), so a token
// costs about 252 bytes where one reading of it costs about 93
// (BenchmarkLexTokens). The hook's LeadingDirectory reads the line once more,
// outside Derive. bytesPerToken is BenchmarkDerive/bash_long_4k's 268,944
// B/op over that line's 1,068 tokens (test/perf/budgets.json, measured
// 2026-10-22 under go1.27.3), 251.8, rounded up; it was 395 while a word's
// text was copied byte by byte and the leading-cd count read the line a
// fourth time. TestRES9_BoundMatchesItsArithmetic holds the constant to that
// row, so a change that moves Derive's cost per token restates the bound
// with the row. The bound is 66,576 tokens: a script handed to an
// interpreter as a here-document, which the program search reads word by
// word, runs to a few thousand.
const (
	shapeBudget   = 16 << 20
	bytesPerToken = 252
	maxTokens     = shapeBudget / bytesPerToken
)

// bounded is a lex result held to maxTokens: see errTokenBound. lex stops
// at most one token past the bound, and this takes that one back.
func bounded(toks []token, err error) ([]token, error) {
	return boundedTo(toks, err, maxTokens)
}

// boundedTo is bounded for a lex held to limit tokens rather than maxTokens.
func boundedTo(toks []token, err error, limit int) ([]token, error) {
	if len(toks) > limit {
		return toks[:limit], errTokenBound
	}
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

// word is the lexer's state for the word being built: everything a token
// carries that is a property of that word alone. Emitting the word resets
// all of it at once (reset), so a field added here is cleared with the rest.
// The bytes between words -- whether a blank or a newline came before the next
// one -- are not a word's, and stay with the lexer.
type word struct {
	// The word's text is the slice s[from:to] of the line while every byte
	// of it is the line's own and they lie together -- a plain word, which
	// most are -- and is copied to text only once quote removal or an
	// escape leaves a gap (add). copied says which.
	text     strings.Builder
	from, to int
	copied   bool
	started  bool
	// glued: the word began with no blank or newline before it (token.glued).
	glued    bool
	quotedAt int // token.quotedAt: -1 until quoting touches the word
	opaque   bool
	ticks    int
}

// newWord is the state before a word's first byte.
func newWord() word { return word{quotedAt: -1} }

// reset makes w the state before the next word's first byte.
func (w *word) reset() { *w = newWord() }

// add appends s[from:to], bytes of the line s as they are written, to the
// word's text. Where they follow the slice the word already is, the slice
// grows and nothing is copied: 8 MiB of $a is one word, copied byte by byte
// it allocated five times its length in each of three readings of the line.
// A copy grows by doubling (strings.Builder.Grow), where append's growth of
// a large buffer is a quarter at a time.
func (w *word) add(s string, from, to int) {
	switch {
	case w.copied:
	case w.from == w.to:
		w.from, w.to = from, to
		return
	case w.to == from:
		w.to = to
		return
	default:
		w.copied = true
		w.text.Grow(w.to - w.from + to - from)
		w.text.WriteString(s[w.from:w.to])
	}
	w.text.Grow(to - from)
	w.text.WriteString(s[from:to])
}

// length is how many bytes of text the word has.
func (w *word) length() int {
	if w.copied {
		return w.text.Len()
	}
	return w.to - w.from
}

// string is the word's text, over the line s.
func (w *word) string(s string) string {
	if w.copied {
		return w.text.String()
	}
	return s[w.from:w.to]
}

// tokenizeMarkedTo splits a command line the way a POSIX shell would split it,
// as far as quoting and metacharacters go, and returns one bit per token:
// whether the tokenizer emitted it for an unquoted metacharacter. It does not
// expand anything: no variables, no globs, no command substitution. Expansion
// would mean evaluating the input, and the count of tokens before expansion is
// both well defined and the only count obtainable without running anything.
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
//
// The bit cannot be recovered from the token text afterwards. A quoted ';' and
// an operator ';' are the same two bytes, so a reader that re-derives the
// answer by looking at the string treats `echo ';' ssh host` as though a new
// command began, and reads ssh as a program that was never run. Only the
// tokenizer knows which it saw, so only the tokenizer can say.
//
// It lexes no more than limit tokens, at most maxTokens: a reader's budget is
// shared across lines and may have less than maxTokens left for this one
// (sshCommandHosts). A line that goes on past limit ends in errTokenBound, as
// one past maxTokens does.
func tokenizeMarkedTo(s string, limit int) ([]string, []bool, error) {
	ts, err := lexTo(joined{s: s}, false, false, limit)
	ts, err = boundedTo(ts, err, limit)
	toks := make([]string, len(ts))
	meta := make([]bool, len(ts))
	for i, t := range ts {
		toks[i], meta[i] = t.text, t.meta
	}
	return toks, meta, err
}

// tokenizeShape splits a command line as tokenizeMarkedTo does and reports, per
// token, what the text cannot: see token.
func tokenizeShape(s string) ([]token, error) {
	return bounded(lex(joined{s: s}, false, false))
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
// It also reads a $( ) inside double quotes as the shell does, as a context
// in which quotes nest (see comsubEnd): "$(echo "a;b")" is one word to the
// shell, and to tokenizeShape a string that ends at the first inner quote.
//
// It is handed the line as written and removes every backslash-newline itself
// (joinContinuations), keeping where it removed each, because a here-document
// or a quote inside such a $( ) cannot be followed without: see comsubEnd.
//
// Where it cannot tell how the shell reads on, it stops with errUncertain
// rather than guess: see lex.
func tokenizeProgram(s string) ([]token, error) {
	return bounded(lex(joinContinuations(s), true, false))
}

// tokenizeList is tokenizeProgram for the walk over a line's whole list of
// commands (statusMasked), which has to read past a here-document to the
// commands after it: python3 - <<'E', a script, E, and then `make test |
// tail` is how agents write a line that edits and then checks. Two things
// differ, and only there:
//
//   - A here-document's body is skipped, from the newline after its operator
//     to its delimiter line, found by the same reading comsubEnd follows one
//     with (heredocDelim, bodiesEnd). Where that reading is not certain -- a
//     delimiter it will not match, a body running to the end, a here-document
//     after a paren, a backtick or an expansion lex does not parse, whose
//     extent is then unknown -- it stops with errUncertain.
//   - An unquoted # at the start of a word runs to the end of its line, as a
//     comment does to the shell, so a quote in `# don't` opens nothing, and
//     a backslash at its end joins no line to it (joinLines).
//
// The program search keeps its own reading, which stops at a here-document
// instead: what it names is unchanged.
func tokenizeList(s string) ([]token, error) {
	return bounded(lex(joinLines(s, true), true, true))
}

// lex is tokenizeShape and tokenizeProgram; program selects the second's
// reading of $'...', and its errUncertain stops:
//
//   - $$' -- bash reads the pair $$, the shell's pid, and then an ordinary
//     quote; zsh reads $ and then a $'...' string. An odd run of $ is $'...'
//     to both. The two readings can end the word in different places.
//   - an unterminated quote on a line that already held an expansion lex
//     does not parse, whose quoting it may have misread.
//   - a $( ) inside double quotes whose close comsubEnd cannot find for
//     certain, or whose $ is the second of an even run, as in "$$(": bash
//     reads $$ and then a (, and the $( ) only a misreading.
//
// r is the line, with the offsets at which backslash-newlines were removed
// from it (joinContinuations); none for tokenizeShape, which reads each as
// a continuation where it stands. docs selects tokenizeList's reading of
// here-documents and comments.
func lex(r joined, program, docs bool) ([]token, error) {
	return lexTo(r, program, docs, maxTokens)
}

// lexTo is lex stopping one token past limit rather than past maxTokens, and
// presizing for no more than that; limit is at most maxTokens.
func lexTo(r joined, program, docs bool, limit int) ([]token, error) {
	s := r.s
	l := &lexer{s: s, r: r, program: program, docs: docs, w: newWord(), dollarAt: -1}
	l.toks = make([]token, 0, min(tokenEstimate(s), limit+1))
	for i := 0; i < len(s); i++ {
		if len(l.toks) > limit {
			// Past the token bound: see bounded.
			return l.toks, errTokenBound
		}
		var err error
		switch c := s[i]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i, err = l.blank(i)
		case docs && c == '#' && !l.w.started:
			i = l.comment(i)
		case c == '\\':
			i, err = l.backslash(i)
		case c == '\'':
			i, err = l.single(i)
		case c == '"':
			i, err = l.double(i)
		case isMeta(c):
			i, err = l.operator(i)
		default:
			l.plain(i)
		}
		if err != nil {
			return l.toks, err
		}
	}
	if len(l.pending) > 0 {
		// A here-document on the last line, whose body never came.
		return l.toks, errUncertain
	}

	l.flush()
	return l.toks, nil
}

// lexer is lex's state over the line s: the word being built, w, and the
// state between words and of the line so far. Each method reads the
// construct that begins at s[i] and returns the index of its last byte, from
// which lex moves on; an error stops lex with the tokens emitted so far.
type lexer struct {
	s             string
	program, docs bool
	r             joined
	toks          []token
	w             word
	sawNL         bool
	// gap: a blank or newline since the last token ended.
	gap bool
	// dollarAt: the index of the last unquoted, unescaped `$`, and
	// dollars: how many of them run together up to it.
	dollarAt int
	dollars  int
	// expanded: a token already emitted was opaque.
	expanded bool
	// pending: for docs, the offsets just past each here-document
	// operator on the current line, whose bodies begin at its end.
	// grouped: a paren or a backtick has been seen.
	pending []int
	grouped bool
}

// begin marks the current word started, noting at its first byte whether it
// is glued to the token before it.
func (l *lexer) begin() {
	if w := &l.w; !w.started {
		w.glued = !l.gap && len(l.toks) > 0
		w.started = true
	}
}

// markQuoted records where quoting first touched the current token.
func (l *lexer) markQuoted() {
	if w := &l.w; w.quotedAt < 0 {
		w.quotedAt = w.length()
	}
}

// expands reports whether s[i] opens an expansion this tokenizer does not
// parse. inDouble: inside double quotes, $'...' and $"..." are not quoting
// forms.
func (l *lexer) expands(i int, inDouble bool) bool {
	s := l.s
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

// unterminated is the error for a quote that never closes. For the program
// search, after an expansion it is uncertain: see lex.
func (l *lexer) unterminated() error {
	if l.program && (l.w.opaque || l.expanded) {
		return errUncertain
	}
	return errUnterminated
}

// flush emits the current word, if one was started.
func (l *lexer) flush() {
	if w := &l.w; w.started {
		l.toks = append(l.toks, token{text: w.string(l.s), quotedAt: w.quotedAt, opaque: w.opaque, nlBefore: l.sawNL, glued: w.glued, ticks: w.ticks})
		l.expanded = l.expanded || w.opaque
		l.sawNL, l.gap = false, false
		w.reset()
	}
}

// blank ends the word at the blank or newline s[i]. For docs, a newline after
// a here-document operator skips the bodies that begin after it, to the
// newline that ends the last delimiter line, which the next token comes after.
func (l *lexer) blank(i int) (int, error) {
	l.flush()
	l.gap = true
	if l.s[i] != '\n' {
		return i, nil
	}
	l.sawNL = true
	if len(l.pending) == 0 {
		return i, nil
	}
	bodies := make([]heredoc, 0, len(l.pending))
	for _, at := range l.pending {
		d, end := heredocDelim(l.s, at)
		if end < 0 {
			return i, errUncertain
		}
		bodies = append(bodies, d)
	}
	e := l.r.bodiesEnd(i+1, bodies)
	if e < 0 {
		return i, errUncertain
	}
	l.pending = nil
	return e, nil
}

// comment reads, for docs, an unquoted # at the start of a word to the end of
// its line.
func (l *lexer) comment(i int) int {
	for i+1 < len(l.s) && l.s[i+1] != '\n' {
		i++
	}
	return i
}

// backslash reads the escape at s[i]: the byte after it, taken as quoted, or
// a newline, which the backslash joins to the line.
func (l *lexer) backslash(i int) (int, error) {
	if i+1 >= len(l.s) {
		l.flush()
		return i, l.unterminated()
	}
	i++
	if l.s[i] != '\n' { // a backslash-newline is a line continuation
		l.begin()
		l.markQuoted()
		l.w.add(l.s, i, i+1)
	}
	return i, nil
}

// single reads the single-quoted string that opens at s[i]: to the next
// quote, or for the program search a $'...' string to its first unescaped one.
func (l *lexer) single(i int) (int, error) {
	s := l.s
	j := strings.IndexByte(s[i+1:], '\'')
	if l.program && l.dollarAt >= 0 && l.dollarAt == i-1 {
		if l.dollars%2 == 0 {
			// $$': the shells disagree. See lex.
			return i, errUncertain
		}
		// $'...', which a backslash-escaped quote does not end.
		j = ansiCEnd(s[i+1:])
	}
	if j < 0 {
		return i, l.unterminated()
	}
	l.begin()
	l.markQuoted()
	l.w.add(s, i+1, i+1+j)
	return i + j + 1, nil
}

// double reads the double-quoted string that opens at s[i] to its closing
// quote.
func (l *lexer) double(i int) (int, error) {
	s, w := l.s, &l.w
	var closed bool
	l.begin()
	l.markQuoted()
	i++
	// dq: how many $ run together up to s[i] inside the quotes.
	dq := 0
	for ; i < len(s); i++ {
		if s[i] == '$' {
			dq++
		} else {
			dq = 0
		}
		if s[i] == '\\' && i+1 < len(s) {
			switch n := s[i+1]; n {
			case '"', '\\', '$', '`':
				w.add(s, i+1, i+2)
				i++
				continue
			case '\n':
				i++
				continue
			}
			w.add(s, i, i+1)
			continue
		}
		if s[i] == '"' {
			closed = true
			break
		}
		if l.program && s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {
			// A context of its own, in which quotes nest: the
			// string goes on after the ) that closes it.
			end := -1
			if dq%2 == 1 {
				end = l.r.substEnd(i, 0)
			}
			if end < 0 {
				return i, errUncertain
			}
			w.opaque = true
			w.add(s, i, end+1)
			i = end
			continue
		}
		if l.expands(i, true) {
			w.opaque = true
		}
		if s[i] == '`' {
			w.ticks++
		}
		w.add(s, i, i+1)
	}
	if !closed {
		return i, l.unterminated()
	}
	return i, nil
}

// operator emits the operator at s[i], of one byte or a doubled two, and for
// docs notes a here-document operator, whose body begins at the line's end.
func (l *lexer) operator(i int) (int, error) {
	s, c := l.s, l.s[i]
	l.flush()
	j := i + 1
	if j < len(s) && s[j] == c {
		switch c {
		case '|', '&', '>', '<':
			j++
		}
	}
	l.toks = append(l.toks, token{text: s[i:j], meta: true, quotedAt: -1, nlBefore: l.sawNL, glued: !l.gap && len(l.toks) > 0})
	l.sawNL, l.gap = false, false
	if l.docs && c == '(' {
		l.grouped = true
	}
	if l.docs && c == '<' && j-i == 2 && (j >= len(s) || s[j] != '<') {
		// A here-document operator, and not a here-string's <<<.
		if l.grouped || l.expanded {
			return i, errUncertain
		}
		l.pending = append(l.pending, j)
	}
	return j - 1, nil
}

// plain adds the unquoted byte s[i] to the word.
func (l *lexer) plain(i int) {
	c := l.s[i]
	if l.expands(i, false) {
		l.w.opaque = true
	}
	if c == '$' {
		if l.dollarAt == i-1 {
			l.dollars++
		} else {
			l.dollars = 1
		}
		l.dollarAt = i
	}
	if c == '`' {
		l.w.ticks++
		l.grouped = true
	}
	l.begin()
	l.w.add(l.s, i, i+1)
}

// tokenEstimate is how many tokens lex will emit from s, for the common line
// an upper bound -- one per word, counted at the blank or metacharacter after
// it, one per metacharacter, and one more -- held to one past maxTokens, so
// the token slice is allocated once rather than grown. Grown, append's copies
// of a slice this size cost about four times the slice it leaves, and at the
// token bound that was most of what one reading of a line allocated. Where it
// is short, as on a line of quoted blanks, lex grows the slice as before. An
// escaped blank ends no word: 8 MiB of `\ ` is one word, and counted as
// millions the line was given a slice for the bound's worth of tokens in
// each of its readings.
//
// A blank inside a single or double quote ends no word either, for the same
// reason: it tracks open/close, not full shell quoting ($(...) nesting
// inside double quotes among them), because this is an upper bound and a
// miscount toward UNDER-estimating costs nothing past the grow lex already
// does for a short line. One 8 MiB single-quoted string full of spaces, with
// no tracking, estimated maxTokens+1 words for what lex reads as one, and
// paid for that slice on every one of a line's several readings.
func tokenEstimate(s string) int {
	n, inWord := 1, false
	quote := byte(0) // 0, or the ' or " this position is inside
	for i := 0; i < len(s) && n <= maxTokens; i++ {
		c := s[i]
		if quote != 0 {
			switch {
			case c == quote:
				quote = 0
			case quote == '"' && c == '\\': // only a double quote reads an escape
				i++
			}
			inWord = true
			continue
		}
		switch {
		case c == '\\': // the byte it escapes is the word's, a blank included
			i++
			inWord = true
		case c == '\'' || c == '"':
			quote = c
			inWord = true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inWord {
				n++
			}
			inWord = false
		case isMeta(c): // a token of its own, and the end of a word
			n++
			inWord = false
		default:
			inWord = true
		}
	}
	return min(n, maxTokens+1)
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

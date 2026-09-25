package shape

import "strings"

// A $( ) inside double quotes is a context of its own to the shell: quotes
// inside it nest, parens count by depth, and the string around it goes on
// after the ) that closes it. "$(echo "a;b")" is one word, whose ; ends
// nothing. Read as a double-quoted string alone, it ends at the first inner
// quote, and the ; after it looks like a separator.
//
// comsubEnd and the functions below it find that ), for the program search
// only, and give up -- return -1 -- wherever they cannot be certain the shell
// finds the same one: an unterminated quote or group; a comment, or a case
// statement where a command begins, whose # and ) may hide or fake the close;
// a here-document they cannot follow to its delimiter line; a ${ } or $[ ]
// holding a quote, a paren or a brace; a $(( that may be a subshell rather
// than arithmetic; an even run of $ before a ( { [ or ', which bash reads as
// $$ and then a byte; a backslash-newline the joining misplaced (see joins);
// and nesting deeper than maxNest. The caller reads -1 as errUncertain.
// None of it expands or runs anything: it only finds where a word ends.
//
// They read the line with its backslash-newlines removed (joinContinuations),
// as the shell does before it splits words, so that `<\<newline><EOF` is a
// here-document. That removal tracks quotes flatly, and inside a $( ) it can
// be wrong both ways: "$(: "'"; ...)" makes it take the rest of the line for
// a single-quoted string, where it removes nothing, and a single-quoted
// string or a here-document body it takes for code loses a backslash-newline
// the shell keeps. joins, the offsets at which it removed one, lets them
// refuse both: a backslash-newline still present where the shell would
// remove it, and one removed where the shell keeps it.

// maxNest bounds how deeply quoted substitutions may nest before the reading
// is given up, so that an adversarial line cannot drive the recursion as deep
// as it is long.
const maxNest = 32

// joined is a line with its backslash-newlines removed, and the offsets in it
// at which each was removed, ascending: see joinContinuations.
type joined struct {
	s     string
	joins []int
}

// joinedIn reports a backslash-newline removed between s[from] and s[to]:
// just before a byte after from and no later than to.
func (r joined) joinedIn(from, to int) bool {
	for _, p := range r.joins {
		if p > from && p <= to {
			return true
		}
	}
	return false
}

// continuesAt reports a backslash-newline left at s[i] -- a continuation the
// shell removes wherever the reader stands when it calls this, which the
// joining left because it took the place for a single-quoted string.
func (r joined) continuesAt(i int) bool {
	return r.s[i] == '\\' && i+1 < len(r.s) && r.s[i+1] == '\n'
}

// heredoc is a here-document seen inside a substitution whose body has not
// begun: its delimiter after quote removal, whether leading tabs are stripped
// from its lines (<<-), and whether any of the delimiter was quoted, which
// leaves its body literal.
type heredoc struct {
	delim  string
	tabs   bool
	quoted bool
}

// wordBreak holds the bytes after which, or before which, a word begins or
// ends: blanks, newline and the metacharacters.
const wordBreak = " \t\n;&|()<>"

// substEnd returns the index of the ) that closes the $( or $(( at s[i], or
// -1 if it cannot tell.
func (r joined) substEnd(i, nest int) int {
	if i+2 < len(r.s) && r.s[i+2] == '(' {
		return r.arithEnd(i+3, nest)
	}
	return r.comsubEnd(i+2, nest)
}

// dollarOpens reports, for the `$` at s[i] that ends a run of dollars $
// long, the index of the ) } or ] that closes the expansion it opens, or i
// when it opens none this reader must skip, or -1. An even run before one of
// the bytes that open one is $$ and then that byte to bash, and to zsh a $
// and then an expansion: see lex.
func (r joined) dollarOpens(i, dollars, nest int) int {
	if i+1 >= len(r.s) {
		return i
	}
	switch r.s[i+1] {
	case '(', '{', '[', '\'':
		if dollars%2 == 0 {
			return -1
		}
	}
	switch r.s[i+1] {
	case '(':
		return r.substEnd(i, nest)
	case '{':
		return braceEnd(r.s, i+2)
	case '[':
		return bracketEnd(r.s, i+2)
	}
	return i
}

// comsubEnd returns the index of the ) that closes a command substitution
// whose body begins at s[i], just after its $(, or -1 if it cannot tell.
func (r joined) comsubEnd(i, nest int) int {
	if nest > maxNest {
		return -1
	}
	var (
		s        = r.s
		body     = i
		depth    = 1
		docs     []heredoc
		dollarAt = -1
		dollars  int
		cmd      commandPosition
	)
	// span refuses a quoted span that holds a newline, as written or
	// joined away, while a here-document waits for one: its body might
	// begin inside the quote.
	span := func(from, to int) bool {
		return to >= 0 && !(len(docs) > 0 && (strings.IndexByte(s[from:to], '\n') >= 0 || r.joinedIn(from, to)))
	}
	for ; i < len(s); i++ {
		c := s[i]
		cmd.see(s, i, body)
		switch {
		case c == '\\':
			if i+1 >= len(s) || r.continuesAt(i) {
				return -1
			}
			i++
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if dollarAt == i-1 {
				// $'...', which a backslash-escaped quote does not end.
				j = ansiCEnd(s[i+1:])
			}
			if j < 0 || !span(i, i+1+j) {
				return -1
			}
			i += j + 1
		case c == '"':
			end := r.dquoteEnd(i+1, nest+1)
			if !span(i, end) {
				return -1
			}
			i = end
		case c == '`':
			end := tickEnd(s, i+1)
			if !span(i, end) {
				return -1
			}
			i = end
		case c == '$':
			if dollarAt == i-1 {
				dollars++
			} else {
				dollars = 1
			}
			dollarAt = i
			end := r.dollarOpens(i, dollars, nest+1)
			if end < 0 {
				return -1
			}
			i = end
		case c == '(' || c == ')':
			if len(docs) > 0 {
				// A group opened or closed before a here-document's body
				// began: where that body is read from is not certain.
				return -1
			}
			if c == '(' {
				depth++
			} else if depth--; depth == 0 {
				return i
			}
		case c == '\n' && len(docs) > 0:
			end := r.bodiesEnd(i+1, docs)
			if end < 0 {
				return -1
			}
			docs, i = nil, end-1
		case c == '<' && strings.HasPrefix(s[i:], "<<<"):
			i += 2 // a here-string: its word is an ordinary word
		case c == '<' && strings.HasPrefix(s[i:], "<<"):
			d, end := heredocDelim(s, i+2)
			if end < 0 {
				return -1
			}
			docs, i = append(docs, d), end-1
		case c == '#' && (i == body || strings.IndexByte(wordBreak, s[i-1]) >= 0):
			// A comment, which runs to the end of the line and may hold
			// the ) that looks like the close.
			return -1
		case c == 'c' && cmd.begins() && isCase(s, i, body):
			// A case statement, whose patterns end in a ) that closes
			// nothing.
			return -1
		}
	}
	return -1
}

// commandPosition follows, through a substitution's body, whether the word
// case would begin a command there, where it is a keyword, rather than be an
// argument: `grep -i case file` runs grep. It is conservative: a word is taken
// for the command's name only if it is plainly one, so a case it cannot place
// is taken for a keyword.
type commandPosition struct {
	named  bool // the command has a name, and every word after it is an argument
	prefix bool // a keyword like time, after which a command may still begin
}

// commandKeywords are the words after which a command still begins, in bash
// or zsh. prefixKeywords are those whose own arguments come before it, or
// might: time -p, repeat 3, coproc NAME.
var (
	commandKeywords = map[string]bool{
		"!": true, "{": true, "[[": true, "case": true, "do": true, "done": true,
		"elif": true, "else": true, "esac": true, "fi": true, "for": true,
		"foreach": true, "if": true, "in": true, "select": true, "then": true,
		"until": true, "while": true, "always": true, "end": true,
	}
	prefixKeywords = map[string]bool{
		"time": true, "coproc": true, "repeat": true, "function": true, "nocorrect": true,
		"noglob": true, "-": true, "builtin": true, "command": true, "exec": true,
	}
)

// see updates the position for the byte s[i] of a body beginning at s[body].
func (p *commandPosition) see(s string, i, body int) {
	c := s[i]
	if strings.IndexByte(";&|()\n", c) >= 0 {
		*p = commandPosition{}
		return
	}
	if strings.IndexByte(wordBreak, c) >= 0 || i != body && strings.IndexByte(wordBreak, s[i-1]) < 0 {
		return
	}
	w := s[i:]
	if k := strings.IndexAny(w, wordBreak); k >= 0 {
		w = w[:k]
	}
	switch {
	case w == "}" || w == "]]":
		// Ends a group or a test after which zsh begins a command
		// without a separator, wherever it stands.
		*p = commandPosition{}
	case p.named || p.prefix:
	case prefixKeywords[w]:
		p.prefix = true
	case commandKeywords[w], strings.IndexByte(w, '=') >= 0, redirects(s, i, body, len(w)):
		// Not the command's name: a keyword, an assignment, or a
		// redirect's fd or target.
	default:
		p.named = true
	}
}

// begins reports a command that may still begin at the current word.
func (p commandPosition) begins() bool { return !p.named }

// redirects reports that the word of n bytes at s[i] is a redirect's target
// or its fd -- a redirect operator ends just before it, past blanks, or
// begins just after it.
func redirects(s string, i, body, n int) bool {
	if i+n < len(s) && (s[i+n] == '<' || s[i+n] == '>') {
		return true
	}
	for j := i - 1; j >= body; j-- {
		switch s[j] {
		case ' ', '\t':
			continue
		case '<', '>':
			return true
		}
		break
	}
	return false
}

// isCase reports the word `case` beginning at s[i].
func isCase(s string, i, body int) bool {
	if !strings.HasPrefix(s[i:], "case") || i != body && strings.IndexByte(wordBreak, s[i-1]) < 0 {
		return false
	}
	return i+4 == len(s) || strings.IndexByte(wordBreak, s[i+4]) >= 0
}

// dquoteEnd returns the index of the " that closes a double-quoted string
// whose body begins at s[i] inside a substitution, or -1.
func (r joined) dquoteEnd(i, nest int) int {
	if nest > maxNest {
		return -1
	}
	s := r.s
	dollars := 0
	for ; i < len(s); i++ {
		if s[i] != '$' {
			dollars = 0
		}
		switch s[i] {
		case '\\':
			if r.continuesAt(i) {
				return -1
			}
			i++
		case '"':
			return i
		case '`':
			if i = tickEnd(s, i+1); i < 0 {
				return -1
			}
		case '$':
			dollars++
			if i+1 >= len(s) {
				return -1
			}
			if s[i+1] == '\'' {
				continue // no quote inside double quotes
			}
			end := r.dollarOpens(i, dollars, nest+1)
			if end < 0 {
				return -1
			}
			if end != i {
				i, dollars = end, 0
			}
		}
	}
	return -1
}

// tickEnd returns the index of the backtick that closes a backtick
// substitution whose body begins at s[i] -- the first one no backslash
// escapes -- or -1.
func tickEnd(s string, i int) int {
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '`':
			return i
		}
	}
	return -1
}

// braceEnd returns the index of the } that closes a ${ } whose body begins at
// s[i], or -1 if the body holds anything that could hide the close or a
// paren: a quote, a backtick, a paren or another brace.
func braceEnd(s string, i int) int {
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '}':
			return i
		case '(', ')', '{', '\'', '"', '`':
			return -1
		}
	}
	return -1
}

// bracketEnd returns the index of the ] that closes a $[ ] whose body begins
// at s[i], or -1 if the body holds anything whose reading inside arithmetic
// is not certain in every shell: a quote, a backtick, a backslash, a paren, a
// brace or a newline. `$[ ( ]` is a closed pair to bash, and a ( counted as a
// group would move the close.
func bracketEnd(s string, i int) int {
	depth := 1
	for ; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth--; depth == 0 {
				return i
			}
		case '(', ')', '{', '}', '\'', '"', '`', '\\', '\n':
			return -1
		}
	}
	return -1
}

// arithEnd returns the index of the second ) of the )) that closes a $(( ))
// whose body begins at s[i], or -1. Inside it << is a shift and a ( no group,
// and it is arithmetic only where it cannot be a $( ) holding a subshell
// instead: the ( after $( must close on a ) followed by another, and the body
// must hold nothing a command could and arithmetic cannot -- a quote, a
// backslash, a backtick, a newline, a ;, a comment or the word case -- outside
// the $( ), ${ } and $[ ] it holds, which are read as themselves.
func (r joined) arithEnd(i, nest int) int {
	if nest > maxNest {
		return -1
	}
	s, body, depth, dollars := r.s, i, 0, 0
	for ; i < len(s); i++ {
		if s[i] != '$' {
			dollars = 0
		}
		switch c := s[i]; c {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
				continue
			}
			if i+1 < len(s) && s[i+1] == ')' {
				return i + 1
			}
			return -1
		case '$':
			// A $( ) inside is a context of its own, which ends where
			// it ends whichever of the two the outer one is.
			dollars++
			end := r.dollarOpens(i, dollars, nest+1)
			if end < 0 {
				return -1
			}
			i = end
		case '\'', '"', '`', '\\', '\n', ';':
			return -1
		case '#':
			if i == body || strings.IndexByte(wordBreak, s[i-1]) >= 0 {
				return -1
			}
		case 'c':
			if isCase(s, i, body) {
				return -1
			}
		}
	}
	return -1
}

// heredocDelim reads the delimiter word of a here-document whose operator
// ends just before s[i], and returns it with the index just past it, or -1
// for a delimiter it will not match lines against: empty, holding an
// expansion, a backslash or a newline.
func heredocDelim(s string, i int) (heredoc, int) {
	var d heredoc
	if i < len(s) && s[i] == '-' {
		d.tabs = true
		i++
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	var b strings.Builder
	for ; i < len(s) && strings.IndexByte(wordBreak, s[i]) < 0; i++ {
		switch c := s[i]; c {
		case '\'', '"':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 || strings.ContainsAny(s[i+1:i+1+j], "\\$`\n") {
				return d, -1
			}
			b.WriteString(s[i+1 : i+1+j])
			d.quoted = true
			i += j + 1
		case '\\':
			if i+1 >= len(s) || s[i+1] == '\n' || s[i+1] == '\\' {
				return d, -1
			}
			i++
			b.WriteByte(s[i])
			d.quoted = true
		case '$', '`':
			return d, -1
		default:
			b.WriteByte(c)
		}
	}
	if d.delim = b.String(); d.delim == "" {
		return d, -1
	}
	return d, i
}

// bodiesEnd skips the bodies of the here-documents docs, in order, the first
// beginning at s[i], and returns the index of the newline that ends the last
// delimiter line, or len(s) if the line ends there. A line that begins with a
// delimiter and is not exactly it -- EOF) above all, which some shells take as
// the delimiter and the close -- is not certain, and returns -1, as does a
// body that runs to the end of the line.
//
// The lines are the shell's, as written. A quoted delimiter leaves its body
// literal, a backslash-newline in it included, so a line the joining ran
// together is read as the lines it was; after an unquoted one the shell
// removes them, and whether before it looks for the delimiter is not certain,
// so a body holding one returns -1.
func (r joined) bodiesEnd(i int, docs []heredoc) int {
	s := r.s
	for k, d := range docs {
		for {
			if i > len(s) {
				return -1
			}
			e := strings.IndexByte(s[i:], '\n')
			if e < 0 {
				e = len(s)
			} else {
				e += i
			}
			if !d.quoted && (r.joinedIn(i-1, e) || strings.HasSuffix(s[i:e], "\\")) {
				return -1
			}
			found, ok := r.delimLine(i, e, d)
			if !ok || !found && e == len(s) {
				return -1
			}
			i = e + 1
			if found {
				if k == len(docs)-1 {
					return e
				}
				break
			}
		}
	}
	return -1
}

// delimLine reports whether the line s[i:e] is the delimiter line of d, and
// ok false where that is not certain. A backslash-newline the joining removed
// from the line ended a line of its own as written, one ending in a backslash
// and so never the delimiter, which holds none.
func (r joined) delimLine(i, e int, d heredoc) (found, ok bool) {
	line := func(l string) string {
		if d.tabs {
			return strings.TrimLeft(l, "\t")
		}
		return l
	}
	from := i
	for _, p := range r.joins {
		if p < i || p > e {
			continue
		}
		if strings.HasPrefix(line(r.s[from:p]), d.delim) {
			return false, false
		}
		from = p
	}
	l := line(r.s[from:e])
	return l == d.delim, l == d.delim || !strings.HasPrefix(l, d.delim)
}

package shape

import "strings"

// isComment reports whether a word begins a comment: an unquoted `#` at its
// start.
func isComment(t token) bool {
	return strings.HasPrefix(t.text, "#") && t.quotedAt != 0
}

// isFDPrefix reports whether an unquoted word could be an fd number (2 in 2>)
// or an fd variable ({fd} in {fd}>).
func isFDPrefix(t token) bool {
	if t.quotedAt >= 0 || t.text == "" {
		return false
	}
	if strings.HasPrefix(t.text, "{") && strings.HasSuffix(t.text, "}") {
		return true
	}
	for j := 0; j < len(t.text); j++ {
		if t.text[j] < '0' || t.text[j] > '9' {
			return false
		}
	}
	return true
}

// redirectEnd returns the index of the last token of the redirection whose
// operator is at i -- the operator, the rest of an operator the tokenizer
// emitted as more than one token, and the word it redirects to -- or false
// when where the redirection ends cannot be told, or when it is a syntax error
// (noTarget), which leaves nothing on the line to run.
//
// The target is taken only if it is a word, and only if that word is certainly
// the whole target: not an expansion the tokenizer split, not the start of a
// glob or array (a paren glued to it), and not zsh's `!` -- `>! f` is a
// clobber into f in zsh and a redirect into a file named ! in bash, and the
// two disagree about which word after it is the command.
func redirectEnd(toks []token, i int) (int, bool) {
	i = operatorEnd(toks, i)
	if noTarget(toks, i) {
		return 0, false
	}
	if toks[i+1].meta {
		// A paren where the target belongs -- a zsh glob such as (a|b) or
		// (x).csv -- or a process substitution, <( ) or >( ), whose operator
		// is here and its paren next. Either way the words inside it are not
		// in command position for this line.
		return 0, false
	}
	target := toks[i+1]
	if target.opaque || target.text == "!" && target.quotedAt < 0 {
		return 0, false
	}
	i++
	if i+1 < len(toks) && toks[i+1].meta && toks[i+1].text == "(" {
		return 0, false
	}
	return i, true
}

// noTarget reports a redirection whose operator ends at i with no word where
// its target belongs, which both shells reject, so that nothing on the line
// runs. Read past, the word after it was taken for the program, and in
// `> ; /home/alice/secret.csv x` that word is a path. There is no target when:
//
//   - the line ends there;
//   - the next word is on the next line, where it begins the next command;
//   - a comment begins there (`ls > #x`), which runs to the end of the line;
//   - an operator comes first -- a separator, `> ;` or `< |`, or another
//     redirection, `> > f`, which owns the word after it -- unless it is a
//     paren, a glob or process substitution that the callers refuse or skip,
//     or a process substitution's own `<` or `>`, as in `cat < <(ls)`.
func noTarget(toks []token, i int) bool {
	if i+1 >= len(toks) {
		return true
	}
	next := toks[i+1]
	switch {
	case next.nlBefore:
		return true
	case isComment(next):
		return true
	case !next.meta, next.text == "(":
		return false
	}
	return !procSub(toks, i+1)
}

// procSub reports a process substitution beginning at j: a `<` or `>` with a
// paren glued to it, which bash and zsh read as a word.
func procSub(toks []token, j int) bool {
	op := toks[j].text
	if op != "<" && op != ">" || j+1 >= len(toks) {
		return false
	}
	next := toks[j+1]
	return next.meta && next.glued && next.text == "("
}

// operatorEnd returns the index of the last piece of the redirection operator
// at i.
//
// The tokenizer doubles a metacharacter only when the next byte is the same
// byte, so `>&`, `>|`, `<>`, `<&` and a here-string's `<<<` arrive as two meta
// tokens, and zsh's `>>|`, `>>&`, `>&|` and `>>&|` as two or three. A piece
// counts only glued to the one before it: `>|` is one operator and `> |` is
// two, and the text of the tokens does not say which. With a blank or a
// newline before it, a piece is the next operator, standing where this one's
// target belongs, and noTarget refuses it: `> |` and `>\n&` are a redirect
// with no target and then a separator, and `<< <&` a here-document with no
// delimiter. Joined, the next word read as the target and an argument as the
// program.
func operatorEnd(toks []token, i int) int {
	op := toks[i].text
	// At most one further piece, except after > and >>, which zsh extends
	// by two (>&| and >>&|). A second `<` after `<<` is the next
	// redirection, not more of this one.
	pieces := 1
	if op == ">" || op == ">>" {
		pieces = 2
	}
	for n := 0; n < pieces && i+1 < len(toks) && toks[i+1].meta && toks[i+1].glued && continuesRedirect(op, toks[i+1].text); n++ {
		i++
	}
	return i
}

// continuesRedirect reports whether next is a further piece of the
// redirection operator op, which the tokenizer emitted as separate tokens.
func continuesRedirect(op, next string) bool {
	switch op {
	case ">", ">>":
		return next == "&" || next == "|"
	case "<":
		return next == ">" || next == "&"
	case "<<":
		return next == "<"
	}
	return false
}

// isRedirect reports whether a metacharacter token redirects, and so is
// followed by a filename rather than by a command.
func isRedirect(tok string) bool {
	switch tok {
	case ">", "<", ">>", "<<":
		return true
	}
	return false
}

func isAssignment(tok string) bool {
	i := strings.IndexByte(tok, '=')
	if i <= 0 {
		return false
	}
	for j := 0; j < i; j++ {
		c := tok[j]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c == '_':
		case c >= '0' && c <= '9' && j > 0:
		default:
			return false
		}
	}
	return true
}

// isShellAssignment reports whether a word is a variable assignment the shell
// would perform: NAME=value, NAME+=value, or NAME[subscript]=value, with the
// name, the `[` and the `=` unquoted -- quoted, 'FOO=bar' is a command by that
// name. A subscript may itself hold `=` (arr[i==0]=v), so the `=` looked for is
// the one after the subscript's matching `]`.
//
// It is wider than isAssignment, which decides argc and is left as it was so
// that a count stays the count it has always been. Program detection cannot
// afford that narrowness: an assignment form it did not recognise used to be
// returned as the program, value and all (PATH+=:/home/u/dir named "dir").
func isShellAssignment(t token) bool {
	s := t.text
	j := 0
	for j < len(s) {
		c := s[j]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c >= '0' && c <= '9' && j > 0 {
			j++
			continue
		}
		break
	}
	if j == 0 || j >= len(s) {
		return false
	}
	if s[j] == '[' {
		depth := 0
		k := j
		for ; k < len(s); k++ {
			if s[k] == '[' {
				depth++
			} else if s[k] == ']' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if k >= len(s) {
			return false
		}
		j = k + 1
	}
	eq := j
	if eq < len(s) && s[eq] == '+' {
		eq++
	}
	if eq >= len(s) || s[eq] != '=' {
		return false
	}
	// Nothing before the `=` may be quoted. Bash decides an assignment on
	// the raw word and needs a literal `]` then `=`; a quoted or escaped
	// byte anywhere before it -- arr[0]"="x, arr[0\]=x -- makes the word a
	// command instead, and skipping it would read its argument as the
	// program. A quoted subscript that bash would still accept (arr["k"]=v)
	// is refused too: null is the cautious answer there, not a leak.
	return t.quotedAt < 0 || t.quotedAt > eq
}

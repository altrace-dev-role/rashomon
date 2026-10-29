package shape

import (
	"path"
	"slices"
	"strings"
)

// programToken finds the index of the token naming the program, if one can be
// told for certain. uncertain: the tokens stop where the lexer could no longer
// vouch for its reading (errUncertain), so a command still open there has an
// end nobody saw.
//
// It skips only what it parses completely, and names nothing -- returns false
// -- at the first thing it does not. The skipped things: a leading assignment
// (FOO=bar, FOO+=bar, arr[i]=bar); a separator or a subshell's `(`, after
// which the next word is in command position; a redirection with its whole
// operator and its target; and `{`, the brace-group keyword, where a command
// starts.
//
// Measured on a real session before this existed: three of twenty-six
// declarations recorded a "program" of `&&`, `(` or nothing -- about one in
// eight of the single field that is supposed to say what ran.
//
// The first two versions of this took the next word after whatever they
// skipped, and two adversarial reviews found 190 lines where that word was
// data: an array element, an arithmetic operand, a here-document's body, a
// redirect target under an operator spelling it did not know (zsh's >! and
// >>|), and above all a fragment of a word this tokenizer split where the
// shell does not -- "$(cat "a b")" is one shell word and three tokens here, so
// skipping "one target" left the rest of it to be read as the program. Fifty
// of those leaked on main as well. So the rule is the other way round: any
// word whose extent this tokenizer cannot vouch for (token.opaque), and
// anything else not parsed completely, ends the search with null. Null already
// means "we could not tell" and is read that way; a word from inside a
// construct is read as a fact.
func programToken(toks []token, uncertain bool) (int, bool) {
	at := func(j int) token {
		if j < len(toks) {
			return toks[j]
		}
		return token{quotedAt: -1}
	}
	isOp := func(j int, op string) bool { t := at(j); return t.meta && t.text == op }

	start := true // command position: the line's start, after a separator, `(` or `{`
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.meta {
			switch {
			case t.text == "<" && isNumericGlob(at(i+1)) && at(i+2).meta && strings.HasPrefix(at(i+2).text, ">"):
				// zsh's numeric glob, <1-9> or <->: one word to zsh, two
				// redirects here.
				return 0, false
			case t.text == "<<" && !isOp(i+1, "<"):
				// A here-document. Its body follows on the next lines, up to a
				// delimiter line this search does not look for, so a word
				// after it may be body text.
				return 0, false
			case isRedirect(t.text):
				end, ok := redirectEnd(toks, i)
				if !ok {
					return 0, false
				}
				i, start = end, false
				continue
			case t.text == "(" && isOp(i+1, "("):
				// `((`: arithmetic. Its operands are not commands.
				return 0, false
			case t.text == ")":
				// A close this search did not see open.
				return 0, false
			}
			start = true // a separator, or a subshell's `(`
			continue
		}

		switch {
		case t.opaque:
			return 0, false
		case t.text == "{" && t.quotedAt < 0 && start:
			// The brace keyword, which only a command's first word can be.
			continue
		case isRedirect(at(i+1).text) && at(i+1).meta && isFDPrefix(t):
			// `2>` or `{fd}>`: an fd number or variable written against its
			// redirect, or -- with a blank before the `>` -- a command named
			// 2. The search does not read on past either to a command after
			// it: an fd prefix names nothing.
			return 0, false
		case isShellAssignment(t):
			if isOp(i+1, "(") {
				// An array's elements.
				return 0, false
			}
			start = false
			continue
		}
		if isOp(i+1, "(") || strings.ContainsAny(t.text, "[]{}()") && !plainPunctuation[t.text] {
			// `name()` defines a function rather than running one; a bracket
			// or brace inside a word is a subscript, a glob or an expansion
			// the tokenizer may have split.
			return 0, false
		}
		if eq := strings.IndexByte(t.text, '='); eq >= 0 && (t.quotedAt < 0 || t.quotedAt > eq) {
			// An `=` the shells read differently: café=x and 1=x are
			// assignments to zsh and commands to bash.
			return 0, false
		}
		if definesFunctions(toks, i, uncertain) {
			return 0, false
		}
		if !plainWord(t.text) || runsOn(toks, i) {
			return 0, false
		}
		return i, true
	}
	return 0, false
}

// pastDirectoryChange moves the program from a leading `cd DIR &&` or
// `cd DIR;`, or the same with pushd, to the command that follows it. pushd
// changes the directory as cd does and pushes the old one on the stack, which
// nothing after it here reads; popd is not stepped over, since where it leads
// is not on the line.
//
// `cd` is where a command runs, not what it runs. Measured on a real session:
// 1,397 of 1,495 shell calls were `cd … && <command>`, so reporting the first
// word said "cd" for nearly everything and "by program" said nothing. Only `&&`
// and `;` are followed: a pipe, `||` or `&` after `cd` is left as it was,
// because what follows those is not simply the next command. If the command
// after `cd` cannot be told, neither can the program -- the answer is null,
// not "cd", which would be a confident wrong answer.
//
// The steps are directoryChanges', the one fold the masked parser passes over
// as well (leadingFold), where its own reading steps over the same ones
// (agreedFold).
func pastDirectoryChange(toks []token, i int, uncertain bool) (int, bool) {
	_, _, prog, ok := directoryChanges(toks, i, uncertain)
	return prog, ok
}

// directoryChanges steps from the command whose program is toks[i] over each
// leading `cd ARGS &&` or `cd ARGS;`, and pushd's alike, and returns after,
// the index past the last separator it stepped over (0 when none), steps, how
// many it stepped over, and prog and ok, pastDirectoryChange's answer. after and steps are
// set even where the program after the last step cannot be told.
//
// The separator is found by commandEnd, the scan that decides where a
// command ends for definesFunctions, so a `;` inside `$( )` or backticks ends
// nothing here either. The command after it is searched by programToken and
// held to every rule the line's first command is. A comment before the
// separator runs to the end of its line (lex), the separator with it, and
// `cd` is then the whole command.
//
// It is a loop, one step per cd, rather than a call per cd: a line of tens
// of thousands of them is within the token bound, and its depth is the
// input's to choose (RES-10, CWE-674).
func directoryChanges(toks []token, i int, uncertain bool) (after, steps, prog int, ok bool) {
	for (toks[i].text == "cd" || toks[i].text == "pushd") && toks[i].quotedAt < 0 {
		sep, ok := commandEnd(toks, i, uncertain)
		if !ok {
			return after, steps, 0, false
		}
		if sep < 0 || toks[sep].text != "&&" && toks[sep].text != ";" {
			return after, steps, i, true
		}
		after, steps = sep+1, steps+1
		k, ok := programToken(toks[after:], uncertain)
		if !ok {
			return after, steps, 0, false
		}
		i = after + k
	}
	return after, steps, i, true
}

// leadingFold is the index past the leading cd steps of the line toks that
// the program search steps over (directoryChanges from the line's first
// program), and 0 where there are none. Past MaxLeadingCd steps it is 0: the
// hook records the directory as unknown there, and a runner after the steps
// is not paired (RES-10). The masked parser calls it over its own reading of
// the line (tokenizeList), and the program search steps over the same steps
// in its own (tokenizeProgram), where the two readings agree (agreedFold).
func leadingFold(toks []token, uncertain bool) int {
	i, ok := programToken(toks, uncertain)
	if !ok {
		return 0
	}
	after, steps, _, _ := directoryChanges(toks, i, uncertain)
	if steps > MaxLeadingCd {
		return 0
	}
	return after
}

// agreedFold is leadingFold over the masked parser's reading of the line,
// toks, where its steps are the program search's, fold, token for token, and
// 0 where they are not. The two readings differ only at a here-document,
// whose body tokenizeList skips and tokenizeProgram reads as words (a comment
// both end at its newline since version 9), so the one could step over a cd
// the other does not. Folding
// none there displaces the runner after any cd, so the runner whose identity
// the masked parser records is always the command the program search names.
func agreedFold(toks, fold []token) int {
	after := leadingFold(toks, false)
	if !slices.Equal(toks[:after], fold) {
		return 0
	}
	return after
}

// plainWord reports whether a word in command position is one the search can
// name as it stands: the text the shell runs, not text it computes a command
// from, and not a word the tokenizer ended where the shell does not.
//
// Each rule gives up a program some line really had, to be certain of the
// rest. $EDITOR file and "$PYTHON" x.py name nothing now, which is the honest
// answer: the command is whatever the variable holds.
//
// A word that can never name a program is not a command word either, and
// path.Base would otherwise make one of it.
func plainWord(s string) bool {
	switch {
	case s == "":
		// '' or "": the shell runs nothing by that name, and path.Base
		// named it `.`, which reads as the source builtin.
		return false
	case strings.HasSuffix(s, "/"):
		// A directory, which execve refuses whatever it holds; path.Base
		// named its last component. So is a word ending in `/` that runs
		// into an operator, the front of a directory the shell reads on.
		return false
	case strings.HasPrefix(s, "~") && !strings.Contains(s, "/"):
		// ~user, ~+ and ~- expand to a directory. ~/bin/tool and
		// ~user/bin/tool name a file in one, and still name it.
		return false
	case s != "." && (path.Base(s) == "." || path.Base(s) == ".."):
		// A path ending in . or .. is a directory too, and path.Base named
		// the dot. A lone `.` is the source builtin, a command like any
		// other.
		return false
	case strings.HasPrefix(s, "%"):
		// A jobspec: %1 or %vim resumes that job, as fg does, and names
		// nothing but the job.
		return false
	}
	digits := true
	for j := 0; j < len(s); j++ {
		c := s[j]
		switch {
		case c < 0x21 || c > 0x7e:
			// Outside printable ASCII. A quoted word with a blank or a
			// newline in it is a whole command line to the shell --
			// "cat /etc/passwd" quoted once named "passwd" -- and a byte
			// the shell does not split on is the same thing unquoted:
			// U+00A0 joins git push origin x into one word, arguments and
			// all.
			return false
		case c == '$':
			// An expansion. zsh applies colon modifiers to an unbraced
			// parameter, so $x:gs/SECRET// runs $x with SECRET removed, and
			// path.Base read the substitution's delimiters as a path.
			return false
		case c == '*' || c == '?':
			// A glob: the command is whichever file matches it.
			return false
		case c == '~' && j > 0:
			// zsh's glob exclusion under EXTENDED_GLOB, whose right side is
			// the pattern excluded. Only a leading `~` is a home directory.
			return false
		case c == '^' || c == '#':
			// zsh's other EXTENDED_GLOB operators: ^ negates the pattern
			// after it and # repeats the one before. /usr/bin/^SECRET named
			// ^SECRET.
			return false
		}
		digits = digits && c >= '0' && c <= '9'
	}
	// All digits: an fd number the tokenizer split from its redirect, as in
	// 2&>f, or a word after a separator that was really a redirect's target.
	return !digits
}

// runsOn reports whether the word at i runs on, with nothing between, into
// what the shell reads as more of the same word: a process substitution, <( )
// or >( ), which bash and zsh both keep inside a word, or a zsh numeric glob,
// <n-m> or <->. The word the shell runs is then acme-merger/dev/fd/63 or
// acme-1/run.sh, and the token here only the front of it.
func runsOn(toks []token, i int) bool {
	if i+2 >= len(toks) || !toks[i+1].meta || !toks[i+1].glued || !toks[i+2].glued {
		return false
	}
	op := toks[i+1].text
	if op != "<" && op != ">" {
		return false
	}
	next := toks[i+2]
	return next.meta && next.text == "(" || op == "<" && isNumericGlob(next)
}

// plainPunctuation are the command names made of brackets or braces: the test
// builtins, and a brace that is not in command-start position -- after an
// assignment or a redirect the shell runs `{` as an ordinary command.
var plainPunctuation = map[string]bool{"[": true, "[[": true, "{": true, "}": true}

// isNumericGlob reports whether an unquoted word is the inside of a zsh
// numeric glob: digits, a dash, digits, either side possibly empty.
func isNumericGlob(t token) bool {
	if t.meta || t.quotedAt >= 0 || !strings.Contains(t.text, "-") {
		return false
	}
	for j := 0; j < len(t.text); j++ {
		if c := t.text[j]; c != '-' && (c < '0' || c > '9') {
			return false
		}
	}
	return strings.Count(t.text, "-") == 1
}

// definesFunctions reports whether the word at i cannot be named as the
// command it begins: the command runs into an empty `()`, or the search for
// one meets what it cannot read past.
//
// zsh's `f g () { ... }` defines functions f and g and runs neither. Nor does
// `f >x g () { ... }` or `f $(date) () { ... }`: a redirection, an expansion
// or a glob between the names does not end the command. A separator does,
// and so does a newline -- but not one inside a group, which is skipped to
// its close first: a paren that does not close at once opens one, and so
// covers $( ), <( ), >( ) and a glob's @( ); and backticks form one.
// `f $(x; y) () { ... }` is one command, and so a function definition.
//
// Nothing is certain, and nothing is named, when the scan cannot find where
// the command ends: a group that never closes; a redirection that is a
// syntax error; a group whose depth the tokens do not show (see
// unknownDepth); a ${ the word does not close, which is a group of its own in
// bash 5.3 and zsh (${ cmd; }) or hides a paren (${x#)}); and tokens that stop
// where the lexer's reading did (uncertain) with the command still open.
func definesFunctions(toks []token, i int, uncertain bool) bool {
	_, ok := commandEnd(toks, i, uncertain)
	return !ok
}

// commandEnd finds where the command whose word is at i ends, by the scan
// definesFunctions describes. sep is the index of the separator that ends it,
// or -1 when a newline or the end of the line does; ok is false where
// definesFunctions names nothing.
func commandEnd(toks []token, i int, uncertain bool) (sep int, ok bool) {
	var (
		open    []byte // the groups around the token, innermost last: '(' or '`'
		heredoc bool   // a here-document was passed: its body starts at the next newline
	)
	for j := i + 1; j < len(toks); j++ {
		t := toks[j]
		if openBrace(t) {
			return 0, false
		}
		if len(open) > 0 {
			if unknownDepth(toks, j, heredoc) {
				return 0, false
			}
			open = inGroup(open, t)
			continue
		}
		switch {
		case t.nlBefore:
			return -1, true
		case t.ticks%2 == 1:
			open = append(open, '`')
		case !t.meta:
		case t.text == "(":
			if j+1 < len(toks) && toks[j+1].meta && toks[j+1].text == ")" {
				return 0, false
			}
			open = append(open, '(')
		case t.text == "&" && j+1 < len(toks) && toks[j+1].meta && toks[j+1].glued && strings.HasPrefix(toks[j+1].text, ">"):
			// &> and &>>: a redirection, not the background separator.
			j++
			fallthrough
		case isRedirect(t.text):
			end := operatorEnd(toks, j)
			if noTarget(toks, end) {
				// A syntax error to both shells: nothing on the line runs.
				return 0, false
			}
			heredoc = heredoc || hereDoc(toks, j)
			j = end
		case t.text == ";" || t.text == "&&" || t.text == "||" || t.text == "|" || t.text == "&":
			return j, true
		}
	}
	if len(open) > 0 || uncertain {
		return 0, false
	}
	return -1, true
}

// unknownDepth reports a token inside a group from which the group's depth
// cannot be told, because what follows may hold a paren, a quote or a
// backtick that counts for nothing: a case statement, whose
// patterns end in a `)` that closes no group; a here-document, whose body is
// text; and, once a here-document has been passed (heredoc), a newline, after
// which its body may begin.
func unknownDepth(toks []token, j int, heredoc bool) bool {
	t := toks[j]
	switch {
	case !t.meta && t.quotedAt < 0 && t.text == "case":
		return true
	case hereDoc(toks, j):
		return true
	case heredoc && t.nlBefore:
		return true
	}
	return false
}

// hereDoc reports a here-document operator at j: `<<`, and not the
// here-string `<<<`, which arrives as `<<` and a glued `<`.
func hereDoc(toks []token, j int) bool {
	if !toks[j].meta || toks[j].text != "<<" {
		return false
	}
	next := j + 1
	return next >= len(toks) || !(toks[next].meta && toks[next].glued && toks[next].text == "<")
}

// openBrace reports a word holding a ${ that it does not close: the tokenizer
// split the expansion, so its end is in a later token that the shell reads as
// part of it, or never came.
func openBrace(t token) bool {
	k := strings.Index(t.text, "${")
	return t.opaque && k >= 0 && strings.Count(t.text[k:], "{") > strings.Count(t.text[k:], "}")
}

// inGroup returns the groups open after the token t, given those open before
// it. Inside backticks only a backtick counts: the first live one closes them,
// whatever parens came between. Elsewhere a paren opens or closes by depth,
// and a backtick opens a group inside the paren's.
func inGroup(open []byte, t token) []byte {
	top := open[len(open)-1]
	switch {
	case t.ticks%2 == 1 && top == '`':
		return open[:len(open)-1]
	case t.ticks%2 == 1:
		return append(open, '`')
	case top == '`' || !t.meta:
	case t.text == "(":
		return append(open, '(')
	case t.text == ")":
		return open[:len(open)-1]
	}
	return open
}

// closesGroups reports whether the command whose program is the token at i
// closes the groups the line opens before it, and no others, so that the
// groups' status is the command's: each `(` open before i is closed by a `)`
// after it, each `{` by a `; }`, and no `)` or `}` after it closes a group
// not opened before it (`go test ./... )` and `(go test ./...))` are syntax
// errors that run nothing). Before i a closer closes only an opened group, so
// the `)` of `cd /a) && go test ./...` counts for nothing here, and
// directoryChanges, which counts no groups, reads that line as one cd step.
//
// After i each command's end is commandEnd's, as for every other reader of
// the line: a `;` inside `$( )` ends nothing, and a separator is passed only
// where it is the `;` of a `; }` closing an opened brace. A comment is no
// token (lex), so nothing after a `#` on the line counts (`go test ./... #
// )`). Counts, not a stack: `{ (go test ./...; } )`, an error to the
// shell, reads as closed -- the over-claim.
func closesGroups(toks []token, i int) bool {
	parens, braces := 0, 0
	for _, t := range toks[:i] {
		switch {
		case t.meta && t.text == "(":
			parens++
		case t.meta && t.text == ")" && parens > 0:
			parens--
		case bareWord(t, "{"):
			braces++
		case bareWord(t, "}") && braces > 0:
			braces--
		}
	}
	for {
		sep, ok := commandEnd(toks, i, false)
		if !ok {
			return false
		}
		end := sep
		if sep < 0 {
			end = len(toks)
		}
		if parens, ok = closedParens(toks[i+1:end], parens); !ok {
			return false
		}
		if sep < 0 {
			return sep < 0 && parens == 0 && braces == 0
		}
		if toks[sep].text != ";" || braces == 0 || sep+1 == len(toks) || !bareWord(toks[sep+1], "}") {
			return false
		}
		i, braces = sep+1, braces-1
	}
}

// closedParens reads the words of one command after its program, with parens
// `(` groups open before it, and returns how many are still open after them;
// ok is false where a `)` closes none of the parens. A group the words open
// themselves ($( ), <( ), a glob's @( ), backticks) closes within them, as
// commandEnd found, and inside backticks a paren counts for nothing, as in
// inGroup.
func closedParens(toks []token, parens int) (open int, ok bool) {
	depth, ticks := 0, false
	for _, t := range toks {
		switch {
		case ticks:
			ticks = t.ticks%2 == 0
		case t.ticks%2 == 1:
			ticks = true
		case !t.meta:
		case t.text == "(":
			depth++
		case t.text == ")" && depth > 0:
			depth--
		case t.text == ")" && parens == 0:
			return 0, false
		case t.text == ")":
			parens--
		}
	}
	return parens, true
}

// bareWord reports the unquoted word w: `{` and `}` open and close a brace
// group only so, as programToken reads the brace keyword.
func bareWord(t token, w string) bool { return !t.meta && t.quotedAt < 0 && t.text == w }

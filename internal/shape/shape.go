// Package shape derives the recorded shape of a tool call.
//
// This package is the only one that looks inside tool_input, and it exists so
// that exactly one file has to be audited to believe the no-content guarantee.
// Everything it returns is either drawn from a closed vocabulary, a count, or a
// keyed digest. No substring of the input reaches the result.
package shape

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"strings"
)

// Verb classes. A closed set: an unrecognized program is "execute" and an
// unrecognized tool is "unknown", so the vocabulary never grows to fit the
// input and can never carry a value from it.
const (
	VerbRead    = "read"
	VerbWrite   = "write"
	VerbNetwork = "network"
	VerbExecute = "execute"
	VerbVCS     = "vcs"
	VerbPackage = "package"
	VerbAgent   = "agent"
	VerbMCP     = "mcp"
	VerbUnknown = "unknown"
)

// Shape is the derived description of one tool call.
//
// Program and Argc are pointers because "we could not tell" is a real outcome
// and has to be distinguishable from a real answer. A command that will not
// tokenize has an unknown argument count, which is null -- never 0, because 0
// is a count and we do not have one.
type Shape struct {
	Program   *string `json:"program"`
	VerbClass string  `json:"verb_class"`
	Argc      *int    `json:"argc"`
	Digest    string  `json:"digest"`
}

// Derive builds the shape of a call to toolName with the given raw tool_input.
//
// key is the per-install HMAC key. Two installs holding different keys produce
// different digests for identical input, so a store cannot be used as a lookup
// table against a dictionary of known commands.
func Derive(toolName string, toolInput json.RawMessage, key []byte) Shape {
	s := Shape{VerbClass: verbForTool(toolName)}

	// Only a shell tool's input carries a command line. An MCP tool or a
	// custom tool may have a "command" field with any meaning at all, and
	// tokenizing it would both misclassify the call and count tokens of
	// something that is not a shell line.
	var (
		cmd     string
		isShell bool
	)
	if s.VerbClass == VerbExecute {
		cmd, isShell = commandField(toolInput)
	}
	if !isShell {
		s.Digest = digest(key, toolName, canonical(toolInput))
		return s
	}

	// A shell tool digests its command line and nothing else. Claude Code
	// attaches a free-text description to a Bash call, so a digest over the
	// whole input would move whenever the wording did and would group nothing.
	s.Digest = digest(key, toolName, []byte(cmd))

	shaped, err := tokenizeShape(cmd)
	toks := make([]string, len(shaped))
	for i, t := range shaped {
		toks[i] = t.text
	}

	// The program is looked for in the line as the shell reads it: with every
	// backslash-newline removed, which the shell does before it splits words
	// -- so `<\<newline><EOF` is a here-document and `$\<newline>{x}` an
	// expansion -- and with a $'...' string read to its first unescaped quote
	// (tokenizeProgram), which does the joining. argc is counted over the
	// tokens above, as it always was.
	pshaped, perr := tokenizeProgram(cmd)

	uncertain := perr == errUncertain
	if i, ok := programToken(pshaped, uncertain); ok && !controlByte(cmd) {
		if i, ok = pastDirectoryChange(pshaped, i, uncertain); ok {
			prog := path.Base(pshaped[i].text)
			s.Program = &prog
			s.VerbClass = verbForProgram(prog)
		}
	}
	if err == nil {
		n := len(dropLeadingAssignments(toks))
		s.Argc = &n
	}
	return s
}

// controlByte reports a line holding a control byte, in which no word can be
// vouched for whatever the program search found.
//
// A carriage return is a word character to the shell and whitespace to the
// tokenizer, so a line holding one is split where the shell does not split
// it. A NUL ends the C string every consumer of the line receives, argv and
// bash -c alike, so nothing after it is read -- and the tokenizer took
// `\x00#hunter2` for a word rather than a comment, and a NUL for a redirect's
// target so that the real target was named. The other C0 controls and DEL are
// word bytes to the shell that nobody types into a command meaning a word.
// Tab and newline are the shell's own separators and are read as such.
func controlByte(cmd string) bool {
	for i := 0; i < len(cmd); i++ {
		if c := cmd[i]; c < 0x20 && c != '\t' && c != '\n' || c == 0x7f {
			return true
		}
	}
	return false
}

// dropLeadingAssignments removes a `FOO=bar` prefix, which is environment
// setting rather than the program being run. Only the token count changes; no
// assignment value is read.
//
// argc has always been counted without this prefix. Finding the program no
// longer needs it dropped, but the count still does: without it the same
// command counts differently depending on which build recorded it.
func dropLeadingAssignments(toks []string) []string {
	for len(toks) > 0 && isAssignment(toks[0]) {
		toks = toks[1:]
	}
	return toks
}

// commandField reports the `command` string of a tool input, which is the only
// field any tool is read for. A tool input that is not an object, or has no
// string `command`, simply has no command line.
func commandField(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return "", false
	}
	v, ok := obj["command"]
	if !ok {
		return "", false
	}
	var cmd string
	if json.Unmarshal(v, &cmd) != nil {
		return "", false
	}
	return cmd, true
}

// digest is HMAC-SHA256 over the tool name and a body, hex encoded. Its width
// is fixed at 64 characters regardless of how large the body was, which is what
// lets a record's serialized length be independent of the size of what it
// describes.
func digest(key []byte, toolName string, body []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(toolName))
	m.Write([]byte{0})
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// canonical re-encodes JSON so that two inputs differing only in key order or
// insignificant whitespace digest identically. Input that is not valid JSON is
// digested as received.
func canonical(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	// encoding/json emits object keys in sorted order.
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

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
		case t.opaque, isComment(t):
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
// `cd DIR;` to the command that follows it.
//
// `cd` is where a command runs, not what it runs. Measured on a real session:
// 1,397 of 1,495 shell calls were `cd … && <command>`, so reporting the first
// word said "cd" for nearly everything and "by program" said nothing. Only `&&`
// and `;` are followed: a pipe, `||` or `&` after `cd` is left as it was,
// because what follows those is not simply the next command. If the command
// after `cd` cannot be told, neither can the program -- the answer is null,
// not "cd", which would be a confident wrong answer.
//
// The separator is found by commandEnd, the scan that decides where a
// command ends for definesFunctions, so a `;` inside `$( )` or backticks ends
// nothing here either. The command after it is searched by programToken and
// held to every rule the line's first command is. A comment before the
// separator makes it comment text, and `cd` is then the whole command.
func pastDirectoryChange(toks []token, i int, uncertain bool) (int, bool) {
	if toks[i].text != "cd" || toks[i].quotedAt >= 0 {
		return i, true
	}
	sep, ok := commandEnd(toks, i, uncertain)
	if !ok {
		return 0, false
	}
	if sep < 0 || toks[sep].text != "&&" && toks[sep].text != ";" {
		return i, true
	}
	for j := i + 1; j < sep; j++ {
		if isComment(toks[j]) {
			return i, true
		}
	}
	k, ok := programToken(toks[sep+1:], uncertain)
	if !ok {
		return 0, false
	}
	return pastDirectoryChange(toks, sep+1+k, uncertain)
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
// backtick that counts for nothing: a comment; a case statement, whose
// patterns end in a `)` that closes no group; a here-document, whose body is
// text; and, once a here-document has been passed (heredoc), a newline, after
// which its body may begin.
func unknownDepth(toks []token, j int, heredoc bool) bool {
	t := toks[j]
	switch {
	case isComment(t):
		return true
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

// joinContinuations removes every backslash-newline the shell removes before
// it splits a line into words: outside quotes and inside double quotes, but
// not inside single quotes, where a backslash is literal. It returns the
// offsets in the result at which it removed one, ascending: the byte there is
// the one that followed it. Its quote tracking is flat, which is right outside
// a $( ) and may be wrong inside one; see comsubEnd.
func joinContinuations(s string) (string, []int) {
	if !strings.Contains(s, "\\\n") {
		return s, nil
	}
	var (
		b     strings.Builder
		joins []int
	)
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case c == '\\' && i+1 < len(s):
			if s[i+1] == '\n' {
				joins = append(joins, b.Len())
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
	return b.String(), joins
}

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

func verbForTool(name string) string {
	if strings.HasPrefix(name, "mcp__") {
		return VerbMCP
	}
	switch name {
	case "Read", "Glob", "Grep", "NotebookRead":
		return VerbRead
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		return VerbWrite
	case "WebFetch", "WebSearch":
		return VerbNetwork
	case "Bash", "BashOutput", "KillShell":
		return VerbExecute
	case "Task", "Agent":
		return VerbAgent
	default:
		return VerbUnknown
	}
}

// programVerb is a coarse classification and is documented as such. It exists to
// make a declaration readable at a glance, not to be a security boundary: an
// unrecognized program is "execute", which is the truthful answer for anything
// being run.
var programVerb = map[string]string{
	"cat": VerbRead, "less": VerbRead, "more": VerbRead, "head": VerbRead,
	"tail": VerbRead, "ls": VerbRead, "find": VerbRead, "grep": VerbRead,
	"rg": VerbRead, "wc": VerbRead, "stat": VerbRead, "file": VerbRead,
	"tree": VerbRead, "diff": VerbRead, "realpath": VerbRead, "pwd": VerbRead,

	"rm": VerbWrite, "mv": VerbWrite, "cp": VerbWrite, "mkdir": VerbWrite,
	"rmdir": VerbWrite, "touch": VerbWrite, "tee": VerbWrite, "ln": VerbWrite,
	"chmod": VerbWrite, "chown": VerbWrite, "truncate": VerbWrite,

	"curl": VerbNetwork, "wget": VerbNetwork, "ssh": VerbNetwork,
	"scp": VerbNetwork, "rsync": VerbNetwork, "nc": VerbNetwork,
	"dig": VerbNetwork, "host": VerbNetwork, "ping": VerbNetwork,

	"git": VerbVCS, "gh": VerbVCS, "hg": VerbVCS, "svn": VerbVCS,

	"npm": VerbPackage, "npx": VerbPackage, "yarn": VerbPackage,
	"pnpm": VerbPackage, "pip": VerbPackage, "pip3": VerbPackage,
	"go": VerbPackage, "cargo": VerbPackage, "gem": VerbPackage,
	"bundle": VerbPackage, "brew": VerbPackage, "apt": VerbPackage,
	"apt-get": VerbPackage, "uv": VerbPackage, "poetry": VerbPackage,
}

func verbForProgram(prog string) string {
	if v, ok := programVerb[prog]; ok {
		return v
	}
	return VerbExecute
}

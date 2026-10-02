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

	// VerbTest is a shell call whose command position holds a recognised
	// test runner (testCommands). It is decided against a fixed list, and the
	// words that decided it are compared and dropped: the record carries the
	// word "test" and nothing else, so `go test ./secret/...` and `go test`
	// store the same class.
	VerbTest = "test"
)

// VerbClasses is the vocabulary, for the schema enum test to compare against,
// for the reason Labels() exists: a closed set that is not exported through a
// function drifts from the published schema in silence.
func VerbClasses() []string {
	return []string{
		VerbRead, VerbWrite, VerbNetwork, VerbExecute, VerbVCS,
		VerbPackage, VerbAgent, VerbMCP, VerbUnknown, VerbTest,
	}
}

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

	// MayWrite is set on a shell call whose line may write files whatever
	// its class says (v3; see mayWrite): `find . -delete` is class read and
	// deletes, `grep -rl x | xargs sed -i` is read and rewrites. One bit, and
	// only the bit: the words that set it are compared and dropped. Always
	// false on a tool that is not a shell, whose class already says what it
	// does, and on a record written before v3, which could not say.
	MayWrite bool `json:"may_write"`
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
	// A line whose program cannot be named, or which the lexer could not
	// read to its end, may write: nobody saw what it runs.
	s.MayWrite = true
	if i, ok := programToken(pshaped, uncertain); ok && !controlByte(cmd) {
		if i, ok = pastDirectoryChange(pshaped, i, uncertain); ok {
			prog := path.Base(pshaped[i].text)
			s.Program = &prog
			s.MayWrite = perr != nil || mayWrite(pshaped, i)
			s.VerbClass = verbForProgram(prog)
			if runsTests(pshaped, i, prog, perr == nil) && !backgrounded(toolInput) {
				s.VerbClass = VerbTest
			}
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

// CWDDigest is the keyed digest of the working directory a call was declared
// in: HMAC under the same per-install key as the shape digest, so two calls'
// directories compare equal or not and a store cannot be tested against a
// guessed path. The empty string for an empty cwd, which is "not known" and
// digests to nothing rather than to the digest of nothing.
//
// The domain is a name no tool can have -- it starts with a NUL -- so a
// directory never digests equal to a tool call whose input happens to be the
// same bytes.
func CWDDigest(key []byte, cwd string) string {
	if cwd == "" {
		return ""
	}
	return digest(key, "\x00cwd", []byte(cwd))
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

// LeadingDirectory reports the directories a shell call's line moves through
// before anything else runs, when the line begins with plain literal
// `cd DIR &&` steps: each DIR as written, in order, which the caller folds
// onto the call's cwd. The command after each `&&` runs only if its cd
// succeeded, so the runner starts where the last one leads.
//
// Only a word the shell takes as it stands: nothing it expands or unquotes
// (`$`, backticks, a glob, `~`, quotes or an escape), not `-`, which is the
// previous directory, and not a comment. `cd DIR;` is left out, since what
// follows a `;` runs whether the cd succeeded or not. A step in the leading
// run that starts with cd, pushd or popd and is not such a cd spoils the
// run, since the directory it leads to is not on the line. Anything else
// reports false, and the call keeps the payload's cwd.
func LeadingDirectory(toolName string, toolInput json.RawMessage) ([]string, bool) {
	if verbForTool(toolName) != VerbExecute {
		return nil, false
	}
	cmd, ok := commandField(toolInput)
	if !ok || controlByte(cmd) {
		return nil, false
	}
	toks, err := tokenizeProgram(cmd)
	if err != nil {
		return nil, false
	}
	var dirs []string
	for i := 0; i < len(toks); i += 3 {
		switch toks[i].text {
		case "cd", "pushd", "popd":
		default:
			return dirs, len(dirs) > 0
		}
		dir, ok := leadingCd(toks[i:])
		if !ok {
			return nil, false
		}
		dirs = append(dirs, dir)
	}
	return dirs, len(dirs) > 0
}

// leadingCd reports the DIR of a plain literal `cd DIR &&` at the start of
// toks, by LeadingDirectory's rules.
func leadingCd(toks []token) (string, bool) {
	if len(toks) < 3 {
		return "", false
	}
	cd, dir, and := toks[0], toks[1], toks[2]
	if cd.text != "cd" || cd.quotedAt >= 0 || cd.opaque {
		return "", false
	}
	if dir.meta || dir.quotedAt >= 0 || dir.opaque || dir.ticks > 0 || dir.nlBefore || isComment(dir) ||
		dir.text == "" || strings.HasPrefix(dir.text, "-") || strings.HasPrefix(dir.text, "~") ||
		strings.ContainsAny(dir.text, "$`*?[{\\") {
		return "", false
	}
	if !and.meta || and.text != "&&" || and.nlBefore {
		return "", false
	}
	return dir.text, true
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

// testCommands are the test runners the test verb class recognises: the
// program, as path.Base names it, and then the plain words that must follow
// it exactly. A runner that is its own program is a list of one.
//
// A fixed list, compared and discarded. Nothing from the line is kept but the
// class; an argument is looked at only to be told equal to one of these.
// Anything this list does not name -- `make -C dir test`, `npx --yes jest`,
// `go test` behind `nice` -- stays what it was before: execute, or package
// for go and cargo. That is an under-claim, and the detections built on this
// class are worth only as much as their refusal to over-claim.
//
// The wrappers on the list are the ones whose exit status is the runner's:
// a project's own gradlew and mvnw, npm's `t` alias, `npx jest` and `npx
// vitest`, `uv run pytest`, `poetry run pytest` and `bundle exec rspec`. So
// are the `timeout N` and `time` prefixes (runnerPrefix), except for
// timeout's own 124, which the report reads as no result. A pipe or a list
// after the runner is still refused (wholeCommand): without pipefail its
// status is the last stage's.
//
// The rule applied: a runner is on the list when it is a known test tool, a
// build tool or launcher given its test command (`go test`, `npm t`, `npm run
// test`, `python -m pytest`), or a listed wrapper that passes its runner's
// exit status through. `make check` is not, since check is not make's test
// command. That rule does not keep lint out: go test runs vet first, an npm `pretest`
// script runs before `npm test`, tox's default envlist and a make `test`
// target can each include lint, and a lint failure fixed only in a file
// named like a test then reads as the tests-only pattern.
var testCommands = [][]string{
	{"pytest"}, {"jest"}, {"vitest"}, {"mocha"}, {"rspec"}, {"phpunit"},
	{"ctest"}, {"tox"}, {"nox"},

	{"go", "test"}, {"cargo", "test"}, {"npm", "test"}, {"npm", "t"}, {"npm", "run", "test"},
	{"yarn", "test"}, {"pnpm", "test"}, {"bun", "test"}, {"dotnet", "test"},
	{"mvn", "test"}, {"mvnw", "test"}, {"gradle", "test"}, {"gradlew", "test"}, {"make", "test"},
	{"python", "-m", "pytest"}, {"python3", "-m", "pytest"},

	{"npx", "jest"}, {"npx", "vitest"}, {"uv", "run", "pytest"}, {"poetry", "run", "pytest"},
	{"bundle", "exec", "rspec"},
}

// refusalsOf names the notARun entry a testCommands row is checked against:
// its own program's, or for a wrapper the runner it wraps, since `npx jest
// --listTests` lists as `jest --listTests` does. gradlew and mvnw take
// gradle's and mvn's.
func refusalsOf(c []string) string {
	switch c[0] {
	case "npx", "uv", "poetry", "bundle":
		return c[len(c)-1]
	case "gradlew":
		return "gradle"
	case "mvnw":
		return "mvn"
	}
	return c[0]
}

// notARun is, per runner by its first word, the arguments with which it does
// something other than run the tests: compile them (`go test -c`, `cargo test
// --no-run`), list them (`-list`, `--collect-only`, `--listTests`), print or
// check what it would do (`make -n`, `make -q`, `--dry-run`), skip them
// (`-DskipTests`, `tox --notest`, `nox --install-only`), set up an
// environment instead (`tox devenv`), watch for changes (`gradle -t`), or
// show its version (`-v` where that is what -v means). Each of those ends ok
// or failed on something no test decided, and as a test run it would
// complete a pattern on that outcome. A word ending in * is a prefix:
// `-list=Foo`, `-elint`, matched whatever follows it. A flag is matched in
// its `flag=value` form too (`-c=true`, `--watch=true`), unless the value is
// false (onList). Maven's skip properties are refused by a rule of their own
// (mavenSkip): `-DskipTests=false` runs the tests.
//
// The lists are the spellings named here, not every spelling a runner
// accepts: a combined short flag (`make -nk`), an option set in a config file
// or an environment variable (other than PYTEST_ADDOPTS, below), or a
// spelling not listed still counts as a test run.
//
// tox and nox are here with their targets, not their listings only: `tox -e
// lint` and `nox -s lint` run a lint session, and a target cannot be told to
// be tests from its name without a guess. Plain `tox` and `nox` run the
// project's default sessions and stay test.
//
// Compared and dropped, like the runner's own words: a match refuses the
// class, and the word is not kept. A word is compared whether quoted or not,
// since the shell passes `"--watch"` as --watch; refusing on it is the
// under-claim.
var notARun = map[string][]string{
	"go":      {"-c", "--c", "-n", "--n", "-list*", "--list*"},
	"cargo":   {"--no-run", "--list", "-V"},
	"pytest":  pytestNotARun,
	"python":  pytestNotARun,
	"python3": pytestNotARun,
	"jest":    {"--listTests", "--showConfig", "--clearCache", "--init", "-v"},
	"vitest":  {"watch", "dev", "list", "bench", "init", "-w", "-v"},
	"mocha":   {"-w", "-V", "--dry-run", "--list-reporters", "--list-interfaces"},
	"rspec":   {"--dry-run", "--init", "-v"},
	"phpunit": {"--list-*", "--generate-configuration", "--migrate-configuration", "--check-version"},
	"ctest":   {"-N", "--show-only*"},
	"dotnet":  {"--list-tests", "-t"},
	"mvn":     {"-v"},
	"gradle":  {"--dry-run", "-m", "-v", "-t", "--continuous"},
	"make":    {"-n", "--just-print", "--dry-run", "--recon", "-q", "--question", "-t", "--touch", "-v"},
	"tox": {"-e*", "--env*", "-m", "-f", "-l", "-a", "--listenvs*", "--showconfig", "--help-ini", "--notest",
		"--devenv*", "list", "l", "config", "c", "depends", "de", "quickstart", "q", "exec", "e", "devenv", "d"},
	"nox": {"-s*", "--session*", "-e*", "-k*", "--keywords*", "-t*", "--tags*", "-l", "--list*", "--install-only"},
}

var pytestNotARun = []string{
	"--collect-only", "--co", "-V", "--fixtures*", "--markers", "--setup-plan", "--setup-only", "--cache-show*",
}

// notARunAny is the spellings of help, version and watch mode that are
// checked for every runner: -h, -help, --help, --version, --watch and
// --watchAll. Watch mode never ends on a result of its own and records the
// launch's instead. A runner's own spelling of these (`-v`, `-V`, `-w`, `-t`,
// `vitest watch`) is on its notARun entry or is not refused at all; the list
// does not make every runner's help, version or watch mode refused.
var notARunAny = []string{"-h", "-help", "--help", "--version", "--watch", "--watchAll"}

// refusesRun reports an argument in args that is on runner's notARun list or
// on notARunAny, or, for pytest, a word starting PYTEST_ADDOPTS= earlier on
// the line (before), an assignment or an export's argument: its value is more
// arguments, which may be `--co`, and they are not read, so any value
// refuses. One exported by an earlier call is not on the line and is not
// seen.
func refusesRun(runner string, before, args []token) bool {
	for _, t := range args {
		if onList(t.text, notARun[runner]) || onList(t.text, notARunAny) || runner == "mvn" && mavenSkip(t.text) {
			return true
		}
	}
	if onList(runner, []string{"pytest", "python", "python3"}) {
		for _, t := range before {
			if strings.HasPrefix(t.text, "PYTEST_ADDOPTS=") {
				return true
			}
		}
	}
	return false
}

// onList reports word equal to an entry of list, or starting with the part
// of one before its trailing *. A word `flag=value` matches the entry flag
// unless value is a false the flag parsers read as false (isFalse):
// `--watchAll=false` runs the tests once, `--watchAll=true` watches, and
// `-c=true` compiles. A prefix entry has no such exception, since the flags
// it names take a pattern or a name: `go test -list=0` lists the tests
// matching 0, and `tox -e=0` runs an environment named 0.
func onList(word string, list []string) bool {
	name, value, hasValue := strings.Cut(word, "=")
	for _, w := range list {
		if p, ok := strings.CutSuffix(w, "*"); ok {
			if strings.HasPrefix(word, p) {
				return true
			}
		} else if word == w {
			return true
		} else if hasValue && strings.HasPrefix(w, "-") && name == w && !isFalse(value) {
			return true
		}
	}
	return false
}

// isFalse reports the false spellings of Go's strconv.ParseBool, for Go's and
// yargs's boolean flags; a yargs flag given another non-true value is still
// refused. Maven's properties follow Java instead (mavenSkip).
func isFalse(v string) bool {
	switch v {
	case "false", "False", "FALSE", "f", "F", "0":
		return true
	}
	return false
}

// mavenSkip reports a Maven property that skips the tests: -DskipTests,
// -Dmaven.test.skip or -Dmaven.test.skip.exec, with no `=value` or with a
// value equal to true ignoring case. Maven reads them with Java's
// Boolean.valueOf, where nothing else is true, so `-DskipTests=false`, `=1`
// and `=no` all run the tests.
func mavenSkip(word string) bool {
	name, value, hasValue := strings.Cut(word, "=")
	switch name {
	case "-DskipTests", "-Dmaven.test.skip", "-Dmaven.test.skip.exec":
		return !hasValue || strings.EqualFold(value, "true")
	}
	return false
}

// runsTests reports whether the command whose program is the token at i,
// named prog, is one of testCommands, and the whole line (wholeCommand), so
// that the line's exit status is the runner's.
//
// The program was found under programToken's rules, so it is certain; each
// word after it must be certain in the same way before it is compared: an
// unquoted word of this same command, and not running on into a process
// substitution the shell reads as more of the same word. A word on the next
// line is the next command's, and wholeCommand refuses any line that has one.
// An operator or an expansion keeps its bytes in the token's text ($, `, ${ ),
// so equality to a list word already refuses those. programToken has refused
// a command whose end it could not find, so the words up to that end are ones
// the lexer vouched for -- and a line the lexer could not finish at all
// (whole) is refused outright: `go test\` ends in a backslash the tokenizer
// dropped and the shell keeps.
//
// Quoted is refused although `go "test"` runs the tests: the rule is "a plain
// word equal to the list", and a quote is where plain stops. And a runner on
// the list is still refused when an argument after its words is on notARun.
func runsTests(toks []token, i int, prog string, whole bool) bool {
	if !whole {
		return false
	}
	i, prog, ok := runnerPrefix(toks, i, prog)
	if !ok {
		return false
	}
next:
	for _, c := range testCommands {
		if c[0] != prog {
			continue
		}
		for n, want := range c[1:] {
			k := i + 1 + n
			if k >= len(toks) {
				continue next
			}
			t := toks[k]
			if t.quotedAt >= 0 || t.text != want || runsOn(toks, k) {
				continue next
			}
		}
		if refusesRun(refusalsOf(c), toks[:i], toks[i+len(c):]) {
			return false
		}
		return wholeCommand(toks, i)
	}
	return false
}

// runnerPrefix steps past the prefixes that pass a runner's exit status
// through: `timeout DURATION`, then `time` (`time -p`), each at most once
// and in that order. It returns the index and name of the word after them,
// or i and prog unchanged when there is no prefix; ok is false when a prefix
// is not followed by a word this search can vouch for.
//
// timeout is taken only as `timeout DURATION`: an option before the duration
// (`-k 5`, `-s KILL`, `--preserve-status`) changes what status it exits
// with, and is refused rather than read. Its own 124, when it fires, is the
// timeout's status and not the runner's; the report reads that as no result.
// The recorded program stays timeout or time, the first word.
func runnerPrefix(toks []token, i int, prog string) (int, string, bool) {
	// A plain word of this command. The duration is all digits, which
	// plainWord refuses in command position, so it is held only to the rest.
	plain := func(j int) bool {
		if j >= len(toks) {
			return false
		}
		t := toks[j]
		return !t.meta && !t.nlBefore && !t.opaque && t.quotedAt < 0 && !runsOn(toks, j)
	}
	word := func(j int) bool { return plain(j) && plainWord(toks[j].text) }
	if prog == "timeout" {
		if !plain(i+1) || !isDuration(toks[i+1].text) || !word(i+2) {
			return i, prog, false
		}
		i += 2
		prog = path.Base(toks[i].text)
	}
	if prog == "time" {
		next := i + 1
		if next < len(toks) && toks[next].text == "-p" && word(next) {
			next++
		}
		if !word(next) {
			return i, prog, false
		}
		i = next
		prog = path.Base(toks[i].text)
	}
	return i, prog, true
}

// isDuration reports a timeout(1) duration: digits, an optional fraction, and
// an optional unit.
func isDuration(w string) bool {
	if n := len(w); n > 0 && strings.IndexByte("smhd", w[n-1]) >= 0 {
		w = w[:n-1]
	}
	whole, frac, dot := strings.Cut(w, ".")
	digits := func(s string) bool {
		for i := 0; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	}
	return whole != "" && digits(whole) && digits(frac) && (!dot || frac != "")
}

// wholeCommand reports whether the runner whose program is the token at i is
// the whole line: nothing follows its own words, arguments and redirections
// but the end of the line.
//
// The recorded outcome is the line's exit status, and it is the runner's only
// when the runner ran last and in the foreground. `go test ./... 2>&1 | tail`
// ends with tail's status, `|| true` and `; echo done` with true's and echo's,
// `&` with the fork's, and `| grep FAIL` inverts it: grep exits 1 when the
// tests passed. `&&` is refused as well, although a failure there is the
// runner's: its success is whatever ran after it. So any separator, and any
// next line, leaves the class what it was before -- the under-claim. The end
// is found by commandEnd, the scan pastDirectoryChange uses, so a `;` inside
// `$( )` ends nothing; a newline anywhere after the runner refuses, whether
// commandEnd reached it or stopped at it.
func wholeCommand(toks []token, i int) bool {
	sep, ok := commandEnd(toks, i, false)
	if !ok || sep >= 0 {
		return false
	}
	for _, t := range toks[i+1:] {
		if t.nlBefore {
			return false
		}
	}
	return true
}

// backgrounded reports a shell call Claude Code was asked to run in the
// background: its tool_input's run_in_background is the JSON boolean true.
//
// Such a call's PostToolUse fires when the shell is launched, not when the
// command ends, so its recorded outcome is the launch's: ok, before any test
// has run. The digest covers the command line only, so the launch digests
// equal to the same command run in the foreground, and as a test run it would
// pair with a real run as passing. It keeps the class the program gave it.
// The field is read as a boolean and nothing else, like the command field is
// read for its words and dropped.
//
// A command Claude Code moves to the background itself, when it reaches its
// timeout or on Ctrl+B, asked for nothing: its tool_input has no such field,
// so it is a test run here. Its execution record says backgrounded instead
// (read from the response on the post path; on Ctrl+B only if Claude Code
// marks it with backgroundTaskId or backgroundedByUser, which was not
// measured), and the report reads such a run as outcome unobserved, as it
// does a launch declared here.
func backgrounded(raw json.RawMessage) bool {
	var obj struct {
		RunInBackground json.RawMessage `json:"run_in_background"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return false
	}
	var b bool
	return json.Unmarshal(obj.RunInBackground, &b) == nil && b
}

// mayWrite reports a shell line whose program is the token at i and which may
// write files whatever class that program gives it: an output redirection to
// a file (not a descriptor duplication such as 2>&1, and not /dev/null); a
// word that writes by itself -- find's -delete, -exec, -execdir, -ok and
// -okdir, xargs, tee, wget, rsync or scp; curl with an output flag (-o, -O,
// --output, --remote-name, or a run of short flags that reaches o or O, with
// or without the file attached); a command or process substitution, whose
// command is not looked into; or a later pipeline or list stage whose program
// is outside the read class, or cannot be named.
//
// The words are compared wherever they stand, quoted or not, and not parsed
// as the program would parse them: `grep -rn xargs .` sets the bit. That is
// the direction to be wrong in. The bit makes a call count as a possible file
// edit (the report's mayEdit), and an edit counted that did not happen stops
// a test-bending pair, while one missed completes a pair over it.
//
// Stages before i are scanned for writer words and redirects, but not classed
// by program: they are the `cd DIR &&` that pastDirectoryChange stepped over,
// and a directory change is told apart by the declarations' directory
// digests, not as a write.
func mayWrite(toks []token, i int) bool {
	curl, curlOut := false, false
	for j, t := range toks {
		if opensSubstitution(toks, j) {
			return true
		}
		if t.meta {
			if outputRedirect(toks, j) {
				return true
			}
			continue
		}
		if t.ticks > 0 || strings.Contains(t.text, "$(") {
			return true
		}
		switch path.Base(t.text) {
		case "xargs", "tee", "wget", "rsync", "scp":
			return true
		case "curl":
			curl = true
		}
		switch t.text {
		case "-delete", "-exec", "-execdir", "-ok", "-okdir":
			return true
		}
		curlOut = curlOut || curlOutputFlag(t.text)
	}
	if curl && curlOut {
		return true
	}
	for j := i + 1; j < len(toks); j++ {
		start := j
		switch {
		case stageSeparator(toks, j):
			start = j + 1
		case toks[j].nlBefore:
		default:
			continue
		}
		if start >= len(toks) {
			break
		}
		k, ok := programToken(toks[start:], false)
		if !ok || verbForProgram(path.Base(toks[start+k].text)) != VerbRead {
			return true
		}
	}
	return false
}

// outputRedirect reports an output redirection whose operator starts at j and
// whose target is a file: `>`, `>>`, `>|`, `&>`, `<>`, and `>&` onto a word
// that is not a descriptor number or `-`. /dev/null, /dev/stdout,
// /dev/stderr and /dev/tty are not files it writes.
func outputRedirect(toks []token, j int) bool {
	t := toks[j]
	if t.text != ">" && t.text != ">>" {
		return false
	}
	if j > 0 && toks[j-1].meta && toks[j-1].text == "<" && t.glued {
		return true // <>: opened for reading and writing, and created
	}
	end := operatorEnd(toks, j)
	dup := false
	for k := j + 1; k <= end; k++ {
		dup = dup || toks[k].text == "&"
	}
	if end+1 >= len(toks) || toks[end+1].meta {
		// No target: nothing on the line runs. A process substitution
		// writes into a command, which may itself write a file, so it
		// counts as a possible write.
		return end+1 < len(toks)
	}
	target := toks[end+1].text
	if dup && (target == "-" || isFDPrefix(toks[end+1])) {
		return false
	}
	switch target {
	case "/dev/null", "/dev/stdout", "/dev/stderr", "/dev/tty":
		return false
	}
	return true
}

// opensSubstitution reports a token at j that opens a command or process
// substitution: an unquoted `$`, `<` or `>` with a `(` glued after it. The
// command inside is not classed.
func opensSubstitution(toks []token, j int) bool {
	t := toks[j]
	if j+1 >= len(toks) || !toks[j+1].meta || !toks[j+1].glued || toks[j+1].text != "(" {
		return false
	}
	if t.meta {
		return t.text == "<" || t.text == ">"
	}
	return strings.HasSuffix(t.text, "$")
}

// stageSeparator reports a token at j that ends one pipeline or list stage:
// `;`, `&&`, `||`, `|`, or a lone `&` -- not the `&` of `>&`, `&>` or `|&`,
// which belongs to a redirection or a pipe.
func stageSeparator(toks []token, j int) bool {
	t := toks[j]
	if !t.meta {
		return false
	}
	switch t.text {
	case ";", "&&", "||", "|":
		return true
	case "&":
		if j > 0 && t.glued && toks[j-1].meta && (toks[j-1].text == ">" || toks[j-1].text == ">>" || toks[j-1].text == "|") {
			return false
		}
		if next := j + 1; next < len(toks) && toks[next].meta && toks[next].glued && strings.HasPrefix(toks[next].text, ">") {
			return false
		}
		return true
	}
	return false
}

// curlOutputFlag reports a curl argument that names an output file: -o, -O,
// their long forms, or a short-option word that reaches o or O. The word is
// walked as curl reads it. A letter, a digit, # or : that takes no argument is
// a flag, and the walk goes on past it; an option that takes an argument ends
// the walk, since the rest of the word is its value (`-XPOST`, `-dfoo=bar`),
// and so does any other character. Whatever follows o is its file, so
// `-o./calc.go`, `-#O` and `-sSLotestdata/x.json` name files.
func curlOutputFlag(w string) bool {
	switch {
	case w == "--output", w == "--remote-name", w == "--remote-name-all", w == "--output-dir",
		strings.HasPrefix(w, "--output="), strings.HasPrefix(w, "--output-dir="):
		return true
	case len(w) > 1 && w[0] == '-' && w[1] != '-':
		for _, c := range w[1:] {
			switch {
			case c == 'o' || c == 'O':
				return true
			case strings.ContainsRune(curlArgumentOptions, c):
				return false
			case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '#', c == ':':
			default:
				return false
			}
		}
	}
	return false
}

// curlArgumentOptions are curl's short options that take an argument, per
// `curl --help all`.
const curlArgumentOptions = "AbcCdDeEFhHKmPQrtTuUwxXyYz"

func verbForProgram(prog string) string {
	if v, ok := programVerb[prog]; ok {
		return v
	}
	return VerbExecute
}

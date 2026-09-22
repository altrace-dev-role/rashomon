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
	// expansion. argc is counted over the tokens above, as it always was.
	pshaped := shaped
	if joined := joinContinuations(cmd); joined != cmd {
		pshaped, _ = tokenizeShape(joined)
	}

	// A carriage return is a word character to the shell and whitespace to
	// the tokenizer, so a line holding one is split where the shell does not
	// split it, and no word in it can be vouched for.
	if i, ok := programToken(pshaped); ok && !strings.ContainsRune(cmd, '\r') {
		prog := path.Base(pshaped[i].text)
		s.Program = &prog
		s.VerbClass = verbForProgram(prog)
	}
	if err == nil {
		n := len(dropLeadingAssignments(toks))
		s.Argc = &n
	}
	return s
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
// told for certain.
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
func programToken(toks []token) (int, bool) {
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
				// A here-document. Its body follows on the next lines, and the
				// tokenizer does not keep line boundaries.
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
			// redirect -- or a command named 2 followed by a space; the
			// tokenizer does not keep which.
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
		if definesFunctions(toks, i) {
			return 0, false
		}
		if strings.ContainsAny(t.text, " \t\n\r") {
			// A quoted word with whitespace in it: the shell would run a
			// command by that whole name, which is a command line, not a
			// program. "cat /etc/passwd" quoted once named "passwd".
			return 0, false
		}
		return i, true
	}
	return 0, false
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

// definesFunctions reports whether the words from i run into an empty `()`:
// zsh's `f g () { ... }` defines functions f and g and runs neither.
func definesFunctions(toks []token, i int) bool {
	for j := i + 1; j < len(toks); j++ {
		if toks[j].meta {
			return toks[j].text == "(" && j+1 < len(toks) && toks[j+1].meta && toks[j+1].text == ")"
		}
	}
	return false
}

// joinContinuations removes every backslash-newline the shell removes before
// it splits a line into words: outside quotes and inside double quotes, but
// not inside single quotes, where a backslash is literal.
func joinContinuations(s string) string {
	if !strings.Contains(s, "\\\n") {
		return s
	}
	var b strings.Builder
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
	return b.String()
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
// when where the redirection ends cannot be told.
//
// The tokenizer doubles a metacharacter only when the next byte is the same
// byte, so `>&`, `>|`, `<>`, `<&` and a here-string's `<<<` arrive as two meta
// tokens, and zsh's `>>|`, `>>&`, `>&|` and `>>&|` as two or three. The target
// is taken only if it is a word, and only if that word is certainly the whole
// target: not an expansion the tokenizer split, not a comment, not the start
// of a glob or array (a paren glued to it), and not zsh's `!` -- `>! f` is a
// clobber into f in zsh and a redirect into a file named ! in bash, and the
// two disagree about which word after it is the command.
func redirectEnd(toks []token, i int) (int, bool) {
	op := toks[i].text
	// At most one further piece, except after > and >>, which zsh extends
	// by two (>&| and >>&|). A second `<` after `<<` is the next
	// redirection, not more of this one.
	pieces := 1
	if op == ">" || op == ">>" {
		pieces = 2
	}
	for n := 0; n < pieces && i+1 < len(toks) && toks[i+1].meta && continuesRedirect(op, toks[i+1].text); n++ {
		i++
	}
	if i+1 < len(toks) && toks[i+1].meta && toks[i+1].text == "(" {
		// A paren where the target belongs: a process substitution, or a
		// zsh glob such as (a|b) or (x).csv. Either way the words inside it
		// are not in command position for this line.
		return 0, false
	}
	if i+1 >= len(toks) || toks[i+1].meta {
		// No target word here: a separator. The search reads on.
		return i, true
	}
	target := toks[i+1]
	// A target on the next line is not a target: a redirect at the end of a
	// line is a syntax error, and the word after the newline is the next
	// command's -- taking it left that command's argument to be read as the
	// program.
	if target.opaque || isComment(target) || target.nlBefore || target.text == "!" && target.quotedAt < 0 {
		return 0, false
	}
	i++
	if i+1 < len(toks) && toks[i+1].meta && toks[i+1].text == "(" {
		return 0, false
	}
	return i, true
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

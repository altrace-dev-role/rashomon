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

	toks, meta, quoted, err := tokenizeQuoted(cmd)

	if i, ok := programToken(toks, meta, quoted); ok {
		prog := path.Base(toks[i])
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
// (FOO=bar, FOO+=bar, arr[0]=bar); a separator or a subshell's `(`, after
// which the next word is in command position; a redirection with its operator
// and its target; and an unquoted `{`, the brace-group keyword.
//
// Measured on a real session before this existed: three of twenty-six
// declarations recorded a "program" of `&&`, `(` or nothing -- about one in
// eight of the single field that is supposed to say what ran. `( cd x && ls )`
// recorded `(`, and a brace group recorded `{`.
//
// The first version of this skipped whatever could not be a program and took
// the next word, and the next word was repeatedly inside something the skip
// did not parse: an array assignment's elements (arr=(a b)), an arithmetic
// expression's operand ($(( n % 97 )) and (( n > 3 ))), a here-document's
// body, a zsh redirect's target (>>| file), a backtick substitution's
// argument. Each put data in the program field, where main had recorded a
// metacharacter: useless, but no content. So the rule is the other way round
// now. Null already means "we could not tell" and is read that way; a word
// from inside a construct this function did not parse is read as a fact.
//
// The metacharacter bits and quoting offsets come from the tokenizer rather
// than from the token's text, because a QUOTED `;` and an operator `;` are
// the same byte, and so are '{' and the keyword `{`, and only the tokenizer
// knows which it saw.
func programToken(toks []string, meta []bool, quoted []int) (int, bool) {
	marked := func(j int) bool { return j < len(meta) && meta[j] }
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		if marked(i) {
			next := ""
			if marked(i + 1) {
				next = toks[i+1]
			}
			switch {
			case tok == "<<" && next != "<":
				// A here-document. Its body follows on the next lines, and the
				// tokenizer does not keep line boundaries, so any word after
				// this may be body text.
				return 0, false
			case isRedirect(tok):
				i = redirectEnd(toks, meta, i)
			case tok == "(" && next == "(":
				// `((`: arithmetic. Its operands are not commands.
				return 0, false
			case tok == ")":
				// A close this function did not see open: it is inside
				// something it did not parse.
				return 0, false
			}
			// Otherwise a separator or a subshell's `(`: the next word is in
			// command position.
			continue
		}

		q := -1
		if i < len(quoted) {
			q = quoted[i]
		}
		switch {
		case tok == "{" && q < 0:
			continue
		case isShellAssignment(tok, q):
			// A value that opens a backtick substitution, or an assignment
			// followed by `(` -- an array's elements, or the $( and $(( the
			// tokenizer split after the `$` -- continues into words that are
			// data or a command this function does not parse.
			if strings.ContainsRune(tok, '`') || marked(i+1) && toks[i+1] == "(" {
				return 0, false
			}
			continue
		case strings.ContainsRune(tok, '`'):
			// A backtick substitution: what runs is inside it.
			return 0, false
		case strings.HasPrefix(tok, "#") && q != 0:
			// A comment.
			return 0, false
		}
		return i, true
	}
	return 0, false
}

// redirectEnd returns the index of the last token of the redirection whose
// operator is at i: the operator, the rest of an operator the tokenizer
// emitted as more than one token, and the word it redirects to.
//
// The tokenizer doubles a metacharacter only when the next byte is the same
// byte, so `>&`, `>|`, `<>`, `<&` and a here-string's `<<<` arrive as two meta
// tokens, and zsh's `>>|`, `>>&`, `>&|` and `>>&|` as two or three. Stopping
// after the first token takes the operator's next piece for the target, and
// leaves the real target -- a filename, or a here-string's literal text -- to
// be read as the program. That shipped once, and put a filename in the store.
//
// The target is taken only if it is a word. Anything else is not a target: a
// `(` after `<` opens a process substitution whose command is the next word,
// and a separator ends the command, putting the word after it in command
// position. Consuming either would read the word after THAT as the program.
func redirectEnd(toks []string, meta []bool, i int) int {
	marked := func(j int) bool { return j < len(meta) && meta[j] }
	op := toks[i]
	for n := 0; n < 2 && marked(i+1) && continuesRedirect(op, toks[i+1]); n++ {
		i++
	}
	if j := i + 1; j < len(toks) && !marked(j) {
		i = j
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

// isShellAssignment reports whether a word the shell would read as a variable
// assignment is one: NAME=value, NAME+=value, or NAME[subscript]=value, with
// the name and the `=` unquoted -- quoted, 'FOO=bar' is a command by that name.
// quoted is the tokenizer's offset of the first quoted byte, or -1.
//
// It is wider than isAssignment, which decides argc and is left as it was so
// that a count stays the count it has always been. Program detection cannot
// afford that narrowness: an assignment form it did not recognise used to be
// returned as the program, value and all (PATH+=:/home/u/dir named "dir").
func isShellAssignment(tok string, quoted int) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	name := tok[:eq]
	name = strings.TrimSuffix(name, "+")
	sub := strings.IndexByte(name, '[')
	if sub >= 0 {
		if !strings.HasSuffix(name, "]") {
			return false
		}
		name = name[:sub]
		// A quoted subscript is still an assignment; the name must not be.
		if quoted >= 0 && quoted < sub {
			return false
		}
	} else if quoted >= 0 && quoted <= eq {
		return false
	}
	if name == "" {
		return false
	}
	for j := 0; j < len(name); j++ {
		c := name[j]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c == '_':
		case c >= '0' && c <= '9' && j > 0:
		default:
			return false
		}
	}
	return true
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

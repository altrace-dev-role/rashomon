// Package shape derives the recorded shape of a tool call.
//
// This package is the only one that looks inside tool_input, and it exists so
// that exactly one package has to be audited to believe the no-content
// guarantee.
// Everything it returns is either drawn from a closed vocabulary, a count, or a
// keyed digest. No substring of the input reaches the result.
package shape

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"path"
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

	// StatusMasked says whether a recognised build or test runner on a shell
	// line has an exit status the line does not return (see statusMasked):
	// the line exits 0 when the runner fails, or the same either way.
	// MaskedTest for `make test 2>&1 | tail -40` or `make test &`,
	// MaskedBuild for `make release; ls dist`, MaskedNone for a line read to
	// its end on which every runner's failure is the line's, or which has no
	// runner. The call's own outcome is recorded as it was; this says that,
	// for the runner, an ok outcome is not a pass. A closed word, decided
	// against fixed lists whose words are compared and dropped.
	//
	// Null on a tool that is not a shell, on a line that could not be read
	// to its end or on which the parser is not sure the runner's status
	// reaches the line's, and on a record written before this field existed,
	// which could not say: each is "not known", never "none".
	StatusMasked *string `json:"status_masked"`

	// RunnerDigest is the keyed digest of the words of that hidden runner,
	// or, when none is hidden, of the runner whose failure the line returns
	// (the last, if several) -- redirections left out. It lets a reader tell
	// that a later `make test` or `make test && echo ok` ran the same runner
	// as an earlier `make test 2>&1 | tail`, whose whole lines digest apart.
	// Null when there is no such runner, when two runners are hidden (no one
	// later run follows both up), and when a cd that the call's cwd digest
	// does not hold ran before it, since where it ran is then not known.
	RunnerDigest *string `json:"runner_digest"`
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
		s.Digest = canonicalDigest(key, toolName, toolInput)
		return s
	}

	// A shell tool digests its command line and nothing else. Claude Code
	// attaches a free-text description to a Bash call, so a digest over the
	// whole input would move whenever the wording did and would group nothing.
	s.Digest = digest(key, toolName, cmd)

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

	// A line read only to the token bound is as uncertain as one the lexer
	// stopped in: what follows is unknown, not absent.
	uncertain := perr == errUncertain || perr == errTokenBound
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
	dirs, _ := leadingRun(pshaped, perr)
	s.StatusMasked, s.RunnerDigest = statusMasked(cmd, key, leadingSteps(dirs))
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
	s, ok := inputField(raw, "command")
	if !ok {
		return "", false
	}
	return stringValue(raw, s)
}

// digest is HMAC-SHA256 over the tool name and a body, hex encoded. Its width
// is fixed at 64 characters regardless of how large the body was, which is what
// lets a record's serialized length be independent of the size of what it
// describes.
//
// body is written through a buffer of at most 4 KiB rather than converted
// whole: []byte(body) would copy the whole command line, 8 MiB at the payload
// cap, to hash it once (RES-9's memory test counts that copy). A body that
// fits is copied once, as the conversion did.
func digest(key []byte, toolName, body string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(toolName))
	m.Write([]byte{0})
	buf := make([]byte, min(len(body), 4<<10))
	for len(body) > 0 {
		n := copy(buf, body)
		m.Write(buf[:n])
		body = body[n:]
	}
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
	return digest(key, "\x00cwd", cwd)
}

// canonical re-encodes JSON so that two inputs differing only in key order or
// insignificant whitespace digest identically: it writes to w the bytes
// json.Marshal gives for raw decoded into any, without building them
// (canonEncoder), and reports false for input that is not valid JSON, which
// is digested as received. The decode into any is the one RES-5 allows,
// kept for digest compatibility until a streaming canonicaliser passes
// RES-48.
func canonical(raw json.RawMessage, w hash.Hash) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	e := canonEncoder{w: w}
	e.value(v)
	e.flush()
	return true
}

// canonicalDigest is the digest of a non-shell tool input: the HMAC of the
// tool name and canonical(toolInput), streamed into the MAC.
func canonicalDigest(key []byte, toolName string, toolInput json.RawMessage) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(toolName))
	m.Write([]byte{0})
	if !canonical(toolInput, m) {
		m.Write(toolInput)
	}
	return hex.EncodeToString(m.Sum(nil))
}

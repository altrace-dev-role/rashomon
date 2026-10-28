// Package shape derives the recorded shape of a tool call.
//
// This package is the only one that looks inside tool_input, and it exists so
// that exactly one package has to be audited to believe the no-content
// guarantee.
// Everything it returns is drawn from a closed vocabulary, a count or a keyed
// digest, with two exceptions, each returned by one named function: the
// hostnames a call names (Hosts), which the report lists and forget --host
// matches as written, and the path a file-changing tool names (DeclaredPath),
// which the declaration carries so that a reader can tell which file a call
// touched. No other substring of the input reaches a result.
//
// Every function here that takes a tool_input (Derive, LeadingDirectory,
// Hosts, Label, DeclaredPath) requires one JSON value that json.Valid
// accepts, as the tool_input span of a payload hookio.Intake returned is. Its
// fields are located by jsonscan.Field, which trusts that validation: on bytes
// that did not pass it, a field can be found where a decoder would have
// refused the whole input, bytes after the object among them.
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
	// later run follows both up), and when a cd, pushd or popd ran before it
	// past the line's leading fold, since where it ran is then not known. The
	// leading cd steps the program search steps over are passed over, read
	// or not, at most MaxLeadingCd of them: past that bound no step is, and
	// a runner after the steps is null (RES-10); nor is any step where the
	// line's two readings step over different ones (agreedFold). Where it ran
	// is the cwd digest's to say.
	RunnerDigest *string `json:"runner_digest"`

	// RunnerKind names the list RunnerDigest's runner is on: RunnerTest or
	// RunnerBuild. Set exactly when RunnerDigest is, from the same runner, so
	// a reader that finds the digest finds the kind (D9). It is the field
	// that says a line ran a test runner whatever followed the runner:
	// `go test ./... && echo ok` is class package and masked none (B27).
	// Null wherever RunnerDigest is, and on a record written before schema
	// 6, which could not say.
	RunnerKind *string `json:"runner_kind"`
}

// Derive builds the shape of a call to toolName with the given raw tool_input.
//
// key is the per-install HMAC key. Two installs holding different keys produce
// different digests for identical input, so a store cannot be used as a lookup
// table against a dictionary of known commands.
func Derive(toolName string, toolInput json.RawMessage, key []byte) Shape {
	// A record below schema 5 has no field for the cut, and the hook writes
	// those: see DeriveCut, which is what a writer that can carry it calls.
	s, _ := DeriveCut(toolName, toolInput, key)
	return s
}

// DeriveCut is Derive, and whether the shape was derived from a shell line
// read only in part: to the token bound (RES-9) or past the leading-cd fold
// (RES-10), where the hook records the start directory as unknown. What
// follows the cut is unknown rather than absent, so a path or a host the
// line names there is not in what was derived, and a reader must not take a
// short list for a complete one. It is schema 5's shape.truncated (Archive
// 85.3.10, "Token or cd bound"), which P-11 serialises; a v4 record does not
// carry it. Always false for a tool that is not a shell.
func DeriveCut(toolName string, toolInput json.RawMessage, key []byte) (Shape, bool) {
	s, cut, _, _ := deriveCut(toolName, toolInput, key)
	return s, cut
}

// DeriveWithDirs is DeriveCut, plus the leading `cd DIR &&` directories
// (LeadingDirectory's own answer) this already read while deriving the
// shape, and unread, the view of the leading cd steps the program search
// steps over where leadingRun reads none of them (unreadFold): "" where the
// steps are read, past MaxLeadingCd or not, and where there are none.
// shape.go's own reading of the program search (pshaped, perr) is exactly
// LeadingDirectory's starting point, so a caller that wants both -- the
// hook's Locate, the only one -- does not tokenize the program search a
// second time just to get what this function already held. A fifth result
// would make these a struct.
func DeriveWithDirs(toolName string, toolInput json.RawMessage, key []byte) (s Shape, cut bool, dirs []string, unread string) {
	return deriveCut(toolName, toolInput, key)
}

// deriveCut is DeriveCut and DeriveWithDirs's shared body.
func deriveCut(toolName string, toolInput json.RawMessage, key []byte) (Shape, bool, []string, string) {
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
		return s, false, nil, ""
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
	ctl := controlByte(cmd)
	// A line whose program cannot be named, or which the lexer could not
	// read to its end, may write: nobody saw what it runs.
	s.MayWrite = true
	if i, ok := programToken(pshaped, uncertain); ok && !ctl {
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
	// A line holding a control byte names no directory, as LeadingDirectory
	// says: the shell does not split it where the tokenizer does, so `cd
	// /repo\r` never enters /repo, and it has no unread fold either, since
	// the program search does not read it. Where the line cannot be read,
	// dirs is empty too.
	var dirs []string
	if !ctl {
		dirs, _ = leadingRun(pshaped, perr)
	}
	// leadingRun reads every step of a leading run or none of it, so where it
	// reads none, any step the program search stepped over is unread.
	var unread string
	if !ctl && len(dirs) == 0 {
		unread = unreadFold(pshaped, uncertain)
	}
	s.StatusMasked, s.RunnerDigest, s.RunnerKind = statusMasked(cmd, key, pshaped[:leadingFold(pshaped, uncertain)])
	cut := err == errTokenBound || perr == errTokenBound || len(dirs) > MaxLeadingCd
	return s, cut, dirs, unread
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
//
// A digest is not a secret and authenticates nothing, which is why the report
// compares digests with == rather than hmac.Equal (S1 review, 2026-10-13). The
// key hides a command's text from whoever reads the store; the digest itself
// is an equality key between two records of the same install, both already
// in the reader's hands. No comparison of two digests decides access, and a
// timing difference could tell its observer only whether two records it has
// already read match -- which the comparison's result tells it anyway.
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

// UnreadCWDDigest is the location of a call whose leading cd steps the
// program search steps over and LeadingDirectory does not read: fold is
// DeriveWithDirs's unread view of them. Where the runner started is then not
// known, so the location is keyed on what is, the payload's cwd and the fold,
// under the same key as CWDDigest, in a domain of its own. The two domains'
// streams differ at their fourth byte -- CWDDigest's name is followed by the
// NUL digest writes, this one's by "-unread" -- so no read directory's digest
// equals an unread one. The same fold from the same cwd is one location;
// another cwd or another fold is another. Never "": where there is no
// payload cwd its caller, hook.Locate, records "" itself and does not call
// it, since keyed on the fold alone one line run in two projects would be
// one location. The body is the cwd, a NUL,
// then the fold: a path holds no NUL, so the cwd decodes one way, and the
// fold's words hold none either, since a line holding a control byte folds
// nothing (controlByte). Only a payload cwd no directory can have, one with a
// NUL in it, could make two bodies alike.
func UnreadCWDDigest(key []byte, cwd, fold string) string {
	return digest(key, "\x00cwd-unread", cwd+"\x00"+fold)
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

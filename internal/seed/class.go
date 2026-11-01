package seed

import (
	"encoding/json"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// kind is a correction episode's kind: the person stopped the agent, said no
// at the prompt, pushed back in words; their own rules refused a call; or a
// call hit a tool error and the agent tried the same tool again.
type kind uint8

const (
	interrupted kind = iota
	rejected
	blocked
	pushedBack
	failedAgain
	numKinds
)

// class is what the agent was doing, or which tool error it hit, in words
// this package owns. A closed set, as shape's verb classes are: an unknown
// tool is "another tool call", so the vocabulary never grows to fit the
// input and a card can never carry a value from it (PF-AU2).
type class uint8

const (
	cReply class = iota
	cShell
	cShellRead
	cShellWrite
	cVCS
	cPackage
	cNetwork
	cTestRun
	cBuildRun
	cEdit
	cEditTest
	cEditSecret
	cWrite
	cWriteTest
	cWriteSecret
	cRead
	cSearch
	cWeb
	cAgent
	cMCP
	cPlan
	cQuestion
	cOther
	eEditNotFound
	eEditMany
	eEditSame
	eStale
	eUnread
	eTooLarge
	eNoFile
	eBadInput
	numClasses
)

func (c class) text() string {
	switch c {
	case cReply:
		return "a reply"
	case cShell:
		return "a shell command"
	case cShellRead:
		return "a shell read"
	case cShellWrite:
		return "a shell write"
	case cVCS:
		return "a version-control command"
	case cPackage:
		return "a package or toolchain command"
	case cNetwork:
		return "a network command"
	case cTestRun:
		return "a test run"
	case cBuildRun:
		return "a build run"
	case cEdit:
		return "an edit"
	case cEditTest:
		return "an edit to a test file"
	case cEditSecret:
		return "an edit to a secrets file"
	case cWrite:
		return "a file write"
	case cWriteTest:
		return "a write to a test file"
	case cWriteSecret:
		return "a write to a secrets file"
	case cRead:
		return "a file read"
	case cSearch:
		return "a file search"
	case cWeb:
		return "a web fetch or search"
	case cAgent:
		return "a subagent"
	case cMCP:
		return "an MCP tool call"
	case cPlan:
		return "its plan"
	case cQuestion:
		return "a question to you"
	case eEditNotFound:
		return "an edit whose text was not in the file"
	case eEditMany:
		return "an edit whose text matched more than once"
	case eEditSame:
		return "an edit that changed nothing"
	case eStale:
		return "a write to a file changed since it was read"
	case eUnread:
		return "a write to a file it had not read"
	case eTooLarge:
		return "a read of a file too large to read whole"
	case eNoFile:
		return "a call on a file that does not exist"
	case eBadInput:
		return "a call the tool refused as malformed"
	}
	return "another tool call"
}

// rowText is a signature's line on the card.
func rowText(k kind, c class) string {
	switch k {
	case interrupted:
		if c == cReply {
			return "you stopped it mid-reply"
		}
		return "you stopped it after " + c.text()
	case rejected:
		return "you said no to " + c.text()
	case blocked:
		return "your rules blocked " + c.text()
	case pushedBack:
		return "you pushed back after " + c.text()
	}
	return c.text() + ", then tried again"
}

// classOf is a call's class from its tool's name and, for a shell or a file
// tool, its input as shape reads it: the runner or the verb class of the
// command, the label of the file's name. Nothing of the input is kept.
func classOf(name string, input json.RawMessage) class {
	if strings.HasPrefix(name, "mcp__") {
		return cMCP
	}
	switch name {
	case "Bash":
		return shellClass(input)
	case "BashOutput", "KillShell":
		return cShell
	case "Edit", "MultiEdit", "NotebookEdit":
		return fileClass(name, input, cEdit)
	case "Write":
		return fileClass(name, input, cWrite)
	case "Read", "NotebookRead":
		return cRead
	case "Glob", "Grep":
		return cSearch
	case "WebFetch", "WebSearch":
		return cWeb
	case "Task", "Agent":
		return cAgent
	case "ExitPlanMode":
		return cPlan
	case "AskUserQuestion":
		return cQuestion
	}
	return cOther
}

func shellClass(input json.RawMessage) class {
	s, _ := shape.DeriveCut("Bash", input, nil)
	if s.RunnerKind != nil {
		if *s.RunnerKind == shape.RunnerBuild {
			return cBuildRun
		}
		return cTestRun
	}
	switch s.VerbClass {
	case shape.VerbRead:
		return cShellRead
	case shape.VerbWrite:
		return cShellWrite
	case shape.VerbVCS:
		return cVCS
	case shape.VerbPackage:
		return cPackage
	case shape.VerbNetwork:
		return cNetwork
	case shape.VerbTest:
		return cTestRun
	}
	return cShell
}

// fileClass is plain, or the test-file or secrets-file class after it (the
// const block keeps each tool's three in that order), by shape's label.
func fileClass(name string, input json.RawMessage, plain class) class {
	switch shape.Label(name, input) {
	case shape.LabelTestFile:
		return plain + 1
	case shape.LabelSSHKey, shape.LabelCredentialShaped, shape.LabelCloudConfig, shape.LabelEnvFile, shape.LabelCertificate:
		return plain + 2
	}
	return plain
}

// errorClass is the class of a tool error by the opening Claude Code writes
// for it, those measured on real transcripts (SEED1 approach note), and
// whether it is one. A command's own failure (Exit code N) is not: a failing
// test run is often the work, not a mistake.
func errorClass(text string) (class, bool) {
	t := strings.TrimPrefix(text, "<tool_use_error>")
	if len(t) > 160 {
		t = t[:160]
	}
	switch {
	case strings.HasPrefix(t, "String to replace not found in file"):
		return eEditNotFound, true
	case strings.HasPrefix(t, "Found ") && strings.Contains(t, " matches of the string to replace"):
		return eEditMany, true
	case strings.HasPrefix(t, "No changes to make: old_string and new_string are exactly the same"):
		return eEditSame, true
	case strings.HasPrefix(t, "File has been modified since read"):
		return eStale, true
	case strings.HasPrefix(t, "File has not been read yet"):
		return eUnread, true
	case strings.HasPrefix(t, "File content (") && strings.Contains(t, "exceeds maximum"):
		return eTooLarge, true
	case strings.HasPrefix(t, "File does not exist"):
		return eNoFile, true
	case strings.HasPrefix(t, "InputValidationError: "):
		return eBadInput, true
	}
	return 0, false
}

// pushback is whether a line the person typed opens by turning the agent
// back: no, nope, stop, undo or revert as its first word (not "no problem"
// and its kin), or "why did you" or "I asked you/for" in its first 200
// bytes. English only; the other kinds are language-free.
func pushback(text string) bool {
	l := strings.ToLower(strings.TrimSpace(text))
	if len(l) > 200 {
		l = l[:200]
	}
	first, rest := word(l)
	switch first {
	case "no", "nope":
		next, _ := word(strings.TrimLeft(rest, " ,.!;:-"))
		switch next {
		case "problem", "worries", "rush", "thanks", "need":
			return false
		}
		return true
	case "stop", "undo", "revert":
		return true
	}
	return strings.Contains(l, "why did you") || strings.Contains(l, "i asked you") ||
		strings.Contains(l, "i asked for") || strings.Contains(l, "i already asked")
}

// startsNo is whether the line opens with no or nope: an answer, not a
// correction, when the agent's last block asked a question.
func startsNo(text string) bool {
	first, _ := word(strings.ToLower(strings.TrimSpace(text)))
	return first == "no" || first == "nope"
}

// word is s's leading run of ASCII letters and what follows it.
func word(s string) (string, string) {
	i := strings.IndexFunc(s, func(r rune) bool { return r < 'a' || r > 'z' })
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

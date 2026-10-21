package hook

import "github.com/altrace-dev-role/rashomon/internal/store"

// Harness names the agent whose hook system invoked this process.
//
// It is passed on the installed command line, never inferred from the
// payload. Two agents can send payloads with the same keys and different
// meanings -- Claude Code's PostToolUse means the call succeeded, Codex's
// says only that it ended -- so guessing from the shape would be guessing at
// the one thing the record has to get right.
type Harness string

const (
	// ClaudeCode is the harness of a command line that names none: every entry
	// installed before the flag existed, and every one watch installs.
	ClaudeCode Harness = ""
	// Codex is the Codex CLI.
	Codex Harness = "codex"
)

// HarnessFlag introduces the harness on a hook command line.
const HarnessFlag = "--harness"

// UnknownHarnessLine is the whole of what an entry naming a harness this build
// does not know says. Hook stderr reaches the agent's debug log, so the value
// that was passed is not repeated.
const UnknownHarnessLine = "rashomon: unknown harness; standing down"

// HarnessArg returns the harness a hook command line names, and false when it
// names one this build does not know, or gives the flag no value.
//
// An unknown harness stands down rather than falling back to Claude Code: a
// payload read under the wrong harness's rules records a fact that is wrong,
// and writing nothing is the smaller error.
func HarnessArg(args []string) (Harness, bool) {
	for i, a := range args {
		if a != HarnessFlag {
			continue
		}
		if i+1 < len(args) && Harness(args[i+1]) == Codex {
			return Codex, true
		}
		return "", false
	}
	return ClaudeCode, true
}

// unobserved clears everything a Codex execution record would otherwise claim
// about how the call ended.
//
// Codex fires PostToolUse for a call that failed as well as for one that
// succeeded -- measured: `python3 missing.py` failed and its payload was a
// plain PostToolUse, with no failure event, no exit code and no interrupt
// flag (test/fixtures/codex). Only the agent's own closing message named a
// status. Read under Claude Code's rules that payload is
// a success, which is the one wrong answer this record exists to prevent. So
// the outcome is left empty, which every reader already renders as outcome
// unobserved; the exit code and interrupt bit are null; and the background
// bit, read from keys Claude Code puts in its tool_response, is false. That
// holds whatever the event is called: a failure event from Codex would be as
// unmeasured as the rest.
//
// What the record still says is true under both harnesses: that the call
// ran, its id and tool, and the digest of the input as it ran.
func unobserved(rec *store.Execution) {
	rec.Outcome = ""
	rec.ExitCode = nil
	rec.IsInterrupt = nil
	rec.Backgrounded = false
}

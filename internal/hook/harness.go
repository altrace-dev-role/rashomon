package hook

import (
	"encoding/json"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

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

// codexTurn is the prompt_id a Codex declaration is recorded under.
//
// Codex has no prompt_id. It has turn_id, which names the same thing for the
// main agent: one id from the UserPromptSubmit that opens a turn, through
// every PreToolUse and PostToolUse in it, to the Stop that closes it
// (test/fixtures/codex). Every reader of prompt_id -- the turn digest, recap's
// once-per-turn key, the report's chains -- groups by it as a turn.
//
// A subagent's calls are the exception. Codex gives a subagent a turn_id of
// its own, and no payload a subagent sends names the turn that spawned it. The
// digest groups a turn by prompt_id ALONE on the rule that a subagent's calls
// carry the parent's (H-89); copied straight across, a Codex subagent's calls
// would form a turn of their own and the parent's turn would read "subagents:
// 0", a false zero. So a subagent call takes the prompt_id of the main agent's
// most recent declaration in this run, which is the spawn or a call after it.
// That is wrong only for a subagent still calling tools after the user has
// started a new turn, and is documented as the approximation it is. When the
// run holds no main-agent declaration to borrow from, the subagent's own
// turn_id is kept: a true value, if not the parent's.
//
// raw has already decoded once as a Payload; a turn_id that is absent or not
// a string leaves prompt_id null, as an absent prompt_id does.
func codexTurn(st *store.Store, sessionID, agentID string, raw []byte) *string {
	var t struct {
		TurnID string `json:"turn_id"`
	}
	if json.Unmarshal(raw, &t) != nil || t.TurnID == "" {
		return nil
	}
	if agentID != "" {
		if parent, ok := st.LatestMainPromptID(sessionID); ok {
			return &parent
		}
	}
	return &t.TurnID
}

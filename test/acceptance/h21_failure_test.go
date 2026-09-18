package acceptance

// H-21 — a failed tool call is recorded as failed.
//
// Before PostToolUseFailure was subscribed, this was not a gap in the data, it
// was the absence of the data: Claude Code fires that event INSTEAD of
// PostToolUse when a call ends badly, so every failed call left a declaration
// with no execution beside it. On disk that is the same shape as a call the
// user denied and a call whose execution went unrecorded, so nothing could tell
// the three apart -- and the report line that matters most, "N calls failed and
// the final message mentions none of it", would have counted zero forever.

import (
	"bytes"
	"encoding/json"
	"testing"
)

// failurePayload builds a PostToolUseFailure body in the shape measured on
// Claude Code 2.1.258: error "Exit code N", is_interrupt, duration_ms, and no
// tool_response at all.
func failurePayload(t *testing.T, toolUseID, errMsg string, interrupt bool, durationMS int64) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUseFailure",
		"session_id":      testSession,
		"transcript_path": "/tmp/transcripts/sess-1.jsonl",
		"cwd":             "/tmp",
		"permission_mode": "default",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "false"},
		"tool_use_id":     toolUseID,
		"error":           errMsg,
		"is_interrupt":    interrupt,
		"duration_ms":     durationMS,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestH21_FailedCallRecordsOutcomeAndExitCode(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))

	execs := e.executions(testSession)
	if len(execs) != 1 {
		t.Fatalf("got %d execution records, want 1: the failure event must produce an "+
			"execution, or a failed call is indistinguishable from a denied one", len(execs))
	}
	got := execs[0].fields

	if got["outcome"] != "failed" {
		t.Errorf("outcome = %v, want \"failed\"", got["outcome"])
	}
	if got["exit_code"] != float64(1) {
		t.Errorf("exit_code = %v, want 1", got["exit_code"])
	}
	if got["is_interrupt"] != false {
		t.Errorf("is_interrupt = %v, want false", got["is_interrupt"])
	}
	if got["duration_ms"] != float64(30) {
		t.Errorf("duration_ms = %v, want 30", got["duration_ms"])
	}
}

// TestH21_SucceedingCallRecordsOK is the healthy twin. An outcome field that
// only ever said "failed" would make every report claim total failure, and the
// success case is the one that runs thousands of times a session.
func TestH21_SucceedingCallRecordsOK(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	execs := e.executions(testSession)
	if len(execs) != 1 {
		t.Fatalf("got %d execution records, want 1", len(execs))
	}
	got := execs[0].fields
	if got["outcome"] != "ok" {
		t.Errorf("outcome = %v, want \"ok\"", got["outcome"])
	}
	// Null, not 0. A zero exit code is a statement that the process succeeded;
	// here nobody stated one, and the two must not be the same record.
	if got["exit_code"] != nil {
		t.Errorf("exit_code = %v, want null on a successful call", got["exit_code"])
	}
	if got["is_interrupt"] != nil {
		t.Errorf("is_interrupt = %v, want null on a successful call", got["is_interrupt"])
	}
}

// TestH21_InterruptIsItsOwnOutcome keeps a user's interruption out of the
// silent-failure count. Counting "you pressed escape" as a failure the agent
// concealed would be the report accusing the operator of its own action.
func TestH21_InterruptIsItsOwnOutcome(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(failurePayload(t, testToolUseID, "", true, 4200))

	execs := e.executions(testSession)
	if len(execs) != 1 {
		t.Fatalf("got %d execution records, want 1", len(execs))
	}
	got := execs[0].fields
	if got["outcome"] != "interrupted" {
		t.Errorf("outcome = %v, want \"interrupted\"", got["outcome"])
	}
	if got["is_interrupt"] != true {
		t.Errorf("is_interrupt = %v, want true", got["is_interrupt"])
	}
}

// TestH21_UnparseableErrorLeavesExitCodeNull is the "never render zero where we
// mean unknown" rule on this field. It also pins the security property: the
// code is parsed from a FIXED PREFIX, so a message that merely contains those
// words cannot put a number in the record, and no other part of the message
// survives.
func TestH21_UnparseableErrorLeavesExitCodeNull(t *testing.T) {
	for _, tc := range []struct {
		name   string
		errMsg string
	}{
		{name: "no code at all", errMsg: "the tool could not run"},
		{name: "code not a number", errMsg: "Exit code SIGSEGV"},
		{name: "code is zero", errMsg: "Exit code 0"},
		{name: "words appear mid-message", errMsg: "build log said Exit code 137 somewhere"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)
			e.mustHook(defaultPayload().build(t))
			e.mustPost(failurePayload(t, testToolUseID, tc.errMsg, false, 12))

			execs := e.executions(testSession)
			if len(execs) != 1 {
				t.Fatalf("got %d execution records, want 1", len(execs))
			}
			got := execs[0].fields
			if got["exit_code"] != nil {
				t.Errorf("exit_code = %v, want null: an unrecognised message yields no "+
					"code, and 0 would assert the process succeeded", got["exit_code"])
			}
			// The outcome still says failed. Failing to parse the code does not
			// make the call a success.
			if got["outcome"] != "failed" {
				t.Errorf("outcome = %v, want \"failed\"", got["outcome"])
			}
		})
	}
}

// TestH21_FailureMessageNeverReachesDisk is the content guarantee for the one
// field on this path that can carry command output. The error message is read
// in order to produce an integer; nothing else from it may survive.
func TestH21_FailureMessageNeverReachesDisk(t *testing.T) {
	const canary = "CANARY-8f21c3-do-not-persist"

	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 2: "+canary, false, 7))

	for rel, f := range walkStore(t, e.home) {
		if bytes.Contains(f.body, []byte(canary)) {
			t.Errorf("%s contains the failure message; the message is read to parse an "+
				"exit code and has no record field it could be assigned to", rel)
		}
	}
	// And the prefixed form did not parse, because the code was not the whole
	// remainder of the message.
	if got := e.executions(testSession)[0].fields["exit_code"]; got != nil {
		t.Errorf("exit_code = %v, want null for a message that carries trailing text", got)
	}
}

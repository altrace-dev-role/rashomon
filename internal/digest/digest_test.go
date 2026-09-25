package digest

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestBuild_ZeroDeclarationsWithHealthyCoverageIsAKnownZero is H-85's first
// half: tools were recorded elsewhere in this session (proving the recorder
// works), this turn simply made none, and that renders as a real zero.
func TestBuild_ZeroDeclarationsWithHealthyCoverageIsAKnownZero(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100),
		},
		Coverage: []store.Coverage{startCoverage("inst-1")},
	}
	// prompt-2 named explicitly, recorded nothing: the empty turn under test.
	d := build(run, nil, "prompt-2", "", time.Now())

	if d.Declarations.Recorded != 0 {
		t.Fatalf("premise: recorded = %d, want 0", d.Declarations.Recorded)
	}
	if d.Unknown {
		t.Errorf("unknown = true, want false: the session's own coverage is clean, so a turn "+
			"that made no calls is a real zero, not a sign the recorder was off. coverage = %+v",
			d.Coverage)
	}
}

// TestBuild_ZeroDeclarationsWithADeadRecorderIsUnknown is H-85's other half,
// and the break the item names directly: collapsing this into a plain zero
// makes "a session with a dead recorder read as a session with a
// well-behaved agent".
func TestBuild_ZeroDeclarationsWithADeadRecorderIsUnknown(t *testing.T) {
	// No coverage records at all: the probe never fired, so there is no
	// evidence the recorder was engaged for this session, let alone this
	// turn.
	run := &store.Run{}
	d := build(run, nil, "prompt-missing", "", time.Now())

	if d.Declarations.Recorded != 0 {
		t.Fatalf("premise: recorded = %d, want 0", d.Declarations.Recorded)
	}
	if !d.Unknown {
		t.Error("unknown = false for a zero-call turn with NO evidence the recorder ever ran -- " +
			"this is a session with a dead recorder reading as a session with a well-behaved agent")
	}
}

// TestBuild_NonZeroDeclarationsAreNeverUnknown: a real count is a real count,
// whatever coverage says about it -- a coverage problem beside it is a
// coverage finding, not reason to call the count itself unknown.
func TestBuild_NonZeroDeclarationsAreNeverUnknown(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100)},
		// No coverage records at all: this turn's coverage will be unverified.
	}
	d := build(run, nil, "prompt-1", "", time.Now())

	if d.Declarations.Recorded == 0 {
		t.Fatal("premise: recorded should be 1")
	}
	if d.Coverage.State == store.StateVerified {
		t.Fatal("premise: coverage should NOT be verified with no probe record at all")
	}
	if d.Unknown {
		t.Error("unknown = true for a turn with a real, non-zero recorded count")
	}
}

// TestBuild_SubagentCallsAreCountedInTheTurnAndSeparately is H-89 end to end
// at the build() level: subagent declarations land inside Declarations.Recorded
// (the parent turn) AND are named apart in Subagents.
func TestBuild_SubagentCallsAreCountedInTheTurnAndSeparately(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			decl(1, "m1", "Bash", "prompt-1", "/main.jsonl", 100),
			subagentDecl(2, "a1", "Bash", "prompt-1", "/main.jsonl/subagents/agent-1.jsonl", "agent-1", 101),
			subagentDecl(3, "a2", "Read", "prompt-1", "/main.jsonl/subagents/agent-1.jsonl", "agent-1", 102),
		},
		Executions: []store.Execution{
			{ToolUseID: "m1", Outcome: store.ExecOK},
			{ToolUseID: "a1", Outcome: store.ExecOK},
		},
	}
	d := build(run, nil, "prompt-1", "", time.Now())

	if d.Declarations.Recorded != 3 {
		t.Errorf("recorded = %d, want 3 (main + both subagent calls, one turn)", d.Declarations.Recorded)
	}
	if d.Subagents.Declarations != 2 {
		t.Errorf("subagents.declarations = %d, want 2", d.Subagents.Declarations)
	}
	if d.Subagents.Executions != 1 {
		t.Errorf("subagents.executions = %d, want 1", d.Subagents.Executions)
	}
	if d.Executions.Recorded != 2 {
		t.Errorf("executions.recorded = %d, want 2 (main + subagent, same turn)", d.Executions.Recorded)
	}
}

// TestBuild_SilentFailuresIsTurnScopedAndOpensNoTranscript is the fix for the
// cross-turn/racy Account concern: failures are counted from the turn's own
// executions, and the final message comes from the argument, never a file.
func TestBuild_SilentFailuresIsTurnScopedAndOpensNoTranscript(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			decl(1, "a1", "Bash", "prompt-1", "/does/not/exist.jsonl", 100),
			decl(2, "b1", "Bash", "prompt-2", "/does/not/exist.jsonl", 200),
		},
		Executions: []store.Execution{
			{ToolUseID: "a1", Outcome: store.ExecFailed},
			{ToolUseID: "b1", Outcome: store.ExecFailed},
		},
	}
	// prompt-1's own failure, described honestly.
	d1 := build(run, nil, "prompt-1", "The command failed as expected.", time.Now())
	if d1.SilentFailures.Failed != 1 {
		t.Errorf("prompt-1 failed = %d, want 1 (its own call only)", d1.SilentFailures.Failed)
	}
	if d1.SilentFailures.Fires {
		t.Error("prompt-1 fired although its own message acknowledges the failure")
	}

	// prompt-2, given NO message at all, must not inherit prompt-1's failure
	// count or its acknowledging text.
	d2 := build(run, nil, "prompt-2", "", time.Now())
	if d2.SilentFailures.Failed != 1 {
		t.Errorf("prompt-2 failed = %d, want 1 (its own call, not both turns')", d2.SilentFailures.Failed)
	}
	if d2.SilentFailures.FinalMessageAvailable {
		t.Error("final_message_available = true with no message supplied")
	}
	if d2.SilentFailures.Fires {
		t.Error("fires = true with no message to judge against -- there is no claim to set the count against")
	}
}

// TestBuild_SkippedRecordsMarksCoverageUnverified is the H-100 unit-level
// guard: a line this read could not parse must not be silently absorbed into
// a clean-looking count.
func TestBuild_SkippedRecordsMarksCoverageUnverified(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100)},
		Coverage:     []store.Coverage{startCoverage("inst-1")},
		Skipped:      1,
	}
	d := build(run, nil, "prompt-1", "", time.Now())
	if d.Coverage.State == store.StateVerified {
		t.Error("state = verified with a skipped, unparseable record in this session's own read")
	}
	if !hasReason(d.Coverage.Reasons, ReasonRecordsSkipped) {
		t.Errorf("reasons = %v, want records_skipped", d.Coverage.Reasons)
	}
	if d.SkippedRecords != 1 {
		t.Errorf("skipped_records = %d, want 1", d.SkippedRecords)
	}
}

// TestBuild_DuplicateDeclarationsMakeTheTurnUnverified is H-103 at the turn
// digest. The migration window (a settings install and the plugin both live)
// double-fires one tool call through both origins, and the digest counts with
// the same report.CountDeclarations report does, so its totals double exactly
// as report's do. Report names that with duplicate_declarations; before this
// the digest did not, and rendered the doubled turn as coverage: verified,
// reasons: none -- the state H-103 exists to prevent, on the surface the
// Stop-hook recap is built from.
//
// The second half keeps the reason turn-scoped: a clean turn in the same
// session carries no duplicate of its own and must not inherit the other
// turn's.
func TestBuild_DuplicateDeclarationsMakeTheTurnUnverified(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			decl(1, "t1", "Bash", "prompt-1", "/t.jsonl", 100),
			decl(2, "t1", "Bash", "prompt-1", "/t.jsonl", 101),
			decl(3, "t2", "Bash", "prompt-2", "/t.jsonl", 500),
		},
		Executions: []store.Execution{
			{ToolUseID: "t1", Outcome: store.ExecOK},
			{ToolUseID: "t1", Outcome: store.ExecOK},
			{ToolUseID: "t2", Outcome: store.ExecOK},
		},
		Coverage: []store.Coverage{startCoverage("inst-1")},
	}

	d := build(run, nil, "prompt-1", "", time.Now())
	if d.Declarations.Recorded != 2 {
		t.Fatalf("premise: recorded = %d, want 2 (both copies, counted honestly)", d.Declarations.Recorded)
	}
	if !hasReason(d.Coverage.Reasons, report.ReasonDuplicateDeclarations) {
		t.Errorf("reasons = %v, want duplicate_declarations: two declarations share one tool_use_id "+
			"in this turn, so its counts are doubled", d.Coverage.Reasons)
	}
	if d.Coverage.State == store.StateVerified {
		t.Errorf("state = verified for a turn whose every count is doubled by a second recorder")
	}

	clean := build(run, nil, "prompt-2", "", time.Now())
	if hasReason(clean.Coverage.Reasons, report.ReasonDuplicateDeclarations) {
		t.Errorf("prompt-2's reasons = %v, carry prompt-1's duplicate", clean.Coverage.Reasons)
	}
	if clean.Coverage.State != store.StateVerified {
		t.Errorf("prompt-2's state = %q (%v), want verified", clean.Coverage.State, clean.Coverage.Reasons)
	}
}

// TestEmpty_IsUnknownAndWritesNoStoreReason.
func TestEmpty_IsUnknownAndWritesNoStoreReason(t *testing.T) {
	d := Empty(time.Now(), "sess-1", "")
	if !d.Unknown {
		t.Error("Empty() is not marked unknown")
	}
	if !hasReason(d.Coverage.Reasons, ReasonNoStore) {
		t.Errorf("reasons = %v, want no_store", d.Coverage.Reasons)
	}
	if d.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want the name it was asked about", d.SessionID)
	}
}

// TestSessionless_IsUnknownAndCarriesOnlyTheNoSessionIDReason: the digest a
// turn gets when its payload named no session (H-107). Unknown, because
// nothing about the turn was read; no_session_id and nothing else, because
// no_store would be false -- a store exists, there was simply no session to
// look for in it -- and a second reason would dilute the one that is true.
func TestSessionless_IsUnknownAndCarriesOnlyTheNoSessionIDReason(t *testing.T) {
	d := Sessionless(time.Now())
	if !d.Unknown {
		t.Error("Sessionless() is not marked unknown")
	}
	if d.Coverage.State != store.StateUnverified {
		t.Errorf("state = %q, want unverified", d.Coverage.State)
	}
	if len(d.Coverage.Reasons) != 1 || d.Coverage.Reasons[0] != ReasonNoSessionID {
		t.Errorf("reasons = %v, want exactly [%s]", d.Coverage.Reasons, ReasonNoSessionID)
	}
	if d.SessionID != "" || d.PromptID != "" {
		t.Errorf("session_id = %q, prompt_id = %q, want both empty: nothing named them",
			d.SessionID, d.PromptID)
	}
	// The same shape as every other digest, so a consumer iterating a list
	// does not meet null where it meets [] elsewhere.
	if d.Declarations.WithoutExecution == nil || d.Declarations.ByTool == nil || d.Gaps == nil {
		t.Errorf("a list or map is nil: %+v", d.Declarations)
	}
}

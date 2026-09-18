package acceptance

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// H-20 is the follow-on the specification deferred: a declaration is a request,
// and closing it needs PostToolUse. It continues the numbering rather than
// amending an item above, because nothing above it changes -- the declaration
// records are what they were, and these are the records that say which of them
// ran.
//
// The risk this file exists to hold down is the payload's tool_response. It is
// tool output, it is the largest thing any hook payload carries, and none of it
// may reach the store, the debug log or the report.

var postInjectionPoints = []string{pointPostStart, pointPostParsed}

// postIDs runs the post path once per id against a transcript path, as the
// installed PostToolUse entry does once each of those calls has returned.
func (e *env) postIDs(transcript string, ids ...string) {
	e.t.Helper()
	for _, id := range ids {
		p := defaultPost()
		p.ToolUseID = id
		p.TranscriptPath = transcript
		e.mustPost(p.build(e.t))
	}
}

// TestH20_PostRecordsTheExecution is the healthy path: one declaration, one
// terminal, one execution, in that order and in one ordered stream.
func TestH20_PostRecordsTheExecution(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	res := e.post(defaultPost().build(t))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0 (stderr: %q)", res.exitCode, res.stderr)
	}
	assertNoTraceback(t, res)
	e.probe("end", testSession)

	execs := e.executions(testSession)
	if len(execs) != 1 {
		t.Fatalf("got %d execution records, want exactly 1", len(execs))
	}
	if got := execs[0].str("tool_use_id"); got != testToolUseID {
		t.Errorf("execution tool_use_id is %q, want %q; an execution that cannot be joined to its declaration closes nothing", got, testToolUseID)
	}
	if got := execs[0].str("tool_name"); got != "Bash" {
		t.Errorf("execution tool_name is %q, want \"Bash\"", got)
	}
	if got := execs[0].fields["seq"]; got != float64(3) {
		t.Errorf("seq is %v, want 3 (declaration 1, terminal 2): the execution shares the run's total order", got)
	}
	assertPhaseCoverage(t, e, testSession, "post", "verified", "")

	rep := e.report(testSession)
	if rep.Executions.Recorded != 1 {
		t.Errorf("executions.recorded is %d, want 1", rep.Executions.Recorded)
	}
	if len(rep.Declarations.WithoutExecution) != 0 {
		t.Errorf("without_execution is %v, want empty: the declaration has its execution", rep.Declarations.WithoutExecution)
	}
	if rep.Coverage.State != "verified" {
		t.Errorf("coverage is %s (%v), want verified", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH20_DeclarationWithoutAnExecutionIsNamed: the store holds declarations
// for calls the user denied, and this is the list that says which declarations
// have no execution beside them -- with the mode each was declared in, because
// that is what a consumer needs to exclude the modes in which nothing is ever
// denied. It is not a coverage failure: a denial is the system working.
func TestH20_DeclarationWithoutAnExecutionIsNamed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.PermissionMode = "acceptEdits"
	e.mustHook(p.build(t))
	e.probe("end", testSession)

	rep := e.report(testSession)
	if rep.Executions.Recorded != 0 {
		t.Errorf("executions.recorded is %d, want 0", rep.Executions.Recorded)
	}
	if len(rep.Declarations.WithoutExecution) != 1 {
		t.Fatalf("without_execution is %v, want the one declaration", rep.Declarations.WithoutExecution)
	}
	got := rep.Declarations.WithoutExecution[0]
	if got.ToolUseID != testToolUseID {
		t.Errorf("without_execution names %q, want %q", got.ToolUseID, testToolUseID)
	}
	if got.PermissionMode != "acceptEdits" {
		t.Errorf("without_execution carries permission_mode %q, want %q: the mode comes from the declaration", got.PermissionMode, "acceptEdits")
	}
	if rep.Coverage.State != "verified" {
		t.Errorf("coverage is %s (%v); a declaration with no execution is not, on its own, a coverage failure", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH20_ExecutionKeySetIsClosed is H-13's rule applied to the new record.
// The assertion is on the key set: asserting that "tool_response" is absent
// stays vacuously true while a differently named field carrying it is added.
func TestH20_ExecutionKeySetIsClosed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	execs := e.executions(testSession)
	if len(execs) != 1 {
		t.Fatalf("got %d execution records, want 1", len(execs))
	}
	assertKeySet(t, execs[0], executionKeys)
}

// TestH20_ToolResponseNeverReachesDisk is H-13's canary sweep pointed at the
// field this item adds. It is a second line of defence and only that: absence
// of a literal proves nothing against an encoding, which is what the width
// assertion below is for.
func TestH20_ToolResponseNeverReachesDisk(t *testing.T) {
	const canary = "CANARY-9d2c4a81-response-must-not-persist"
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	big := canary + strings.Repeat("x", 20480) + canary
	p := defaultPost()
	p.ToolResponse = map[string]any{
		"stdout":      big,
		"stderr":      canary,
		"interrupted": false,
		"nested":      map[string]any{"deep": []any{canary, map[string]any{"deeper": big}}},
	}
	res := e.post(p.build(t))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}
	e.probe("end", testSession)
	// Both forms: the text renderer reads the same records through its own
	// code, and a renderer is where a field nobody meant to print gets printed.
	repJSON := e.run("", nil, "report", "--json", "--session", testSession)
	repText := e.run("", nil, "report", "--session", testSession)

	// Guard the premise: a canary is absent from a store nothing was written
	// to, and that would pass every assertion below.
	if got := e.executions(testSession); len(got) != 1 {
		t.Fatalf("got %d execution records, want 1; the sweep below would be vacuous", len(got))
	}

	for rel, f := range walkStore(t, e.home) {
		if bytes.Contains(f.body, []byte(canary)) {
			t.Errorf("%s contains the canary", rel)
		}
	}
	// Hook stdout and stderr reach Claude Code's debug log, which puts them on
	// disk as surely as the store does. And report is output.
	for name, s := range map[string]string{
		"post stdout":   res.stdout,
		"post stderr":   res.stderr,
		"report --json": repJSON.stdout,
		"report text":   repText.stdout,
	} {
		if strings.Contains(s, canary) {
			t.Errorf("%s contains the canary", name)
		}
	}
}

// TestH20_ExecutionWidthIsIndependentOfTheResponse is the assertion that
// survives an encoding. The record has no field whose width a response could
// move, so three orders of magnitude of tool output must not change one byte.
func TestH20_ExecutionWidthIsIndependentOfTheResponse(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	short := defaultPost()
	short.ToolResponse = map[string]any{"stdout": strings.Repeat("a", 20)}
	long := defaultPost()
	long.ToolResponse = map[string]any{"stdout": strings.Repeat("a", 20480)}
	e.mustPost(short.build(t))
	e.mustPost(long.build(t))

	execs := e.executions(testSession)
	if len(execs) != 2 {
		t.Fatalf("got %d execution records, want 2", len(execs))
	}
	a, b := execs[0], execs[1]
	if len(a.raw) != len(b.raw) {
		t.Errorf("record width tracks the response size: %d bytes for a 20-byte tool_response, %d bytes for a 20-KB one",
			len(a.raw), len(b.raw))
	}
}

// TestH20_PostFromAnotherInstallStandsDown: two installs' entries can sit in
// one settings file and Claude Code runs both with one environment, so the
// post path has to stand down on the same rule as the hook path -- or every
// call is recorded as having run twice.
func TestH20_PostFromAnotherInstallStandsDown(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	before := walkStore(t, e.home)

	res := e.run(defaultPost().build(t), nil, "post", "--install", otherInstall)
	if res.exitCode != 0 {
		t.Fatalf("standing down: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("stdout is %q, which Claude Code parses as control output", res.stdout)
	}
	if got := e.executions(testSession); len(got) != 0 {
		t.Errorf("another install's entry recorded %d executions into this store", len(got))
	}

	after := walkStore(t, e.home)
	for rel, a := range after {
		switch b, ok := before[rel]; {
		case !ok:
			t.Errorf("a stand-down created %s", rel)
		case !bytes.Equal(a.body, b.body):
			t.Errorf("a stand-down wrote to %s", rel)
		}
	}
	for rel := range before {
		if _, ok := after[rel]; !ok {
			t.Errorf("a stand-down removed %s", rel)
		}
	}
}

// TestH20_PostCoverageReadsItsOwnEntry: the post phase resolves the PostToolUse
// entry, not the recorder's. A configuration carrying one and not the other is
// exactly the arrangement in which reading the wrong one lies -- declarations
// arrive in full, executions arrive not at all, and a run reading the
// recorder's entry would call itself covered.
func TestH20_PostCoverageReadsItsOwnEntry(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	var doc map[string]any
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok || hooks["PostToolUse"] == nil {
		t.Fatalf("premise broken: watch installed no PostToolUse entry:\n%s", e.settingsBytes())
	}
	delete(hooks, "PostToolUse")
	out, _ := json.MarshalIndent(doc, "", "  ")
	e.writeSettings(string(out) + "\n")

	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	assertPhaseCoverage(t, e, testSession, "call", "verified", "")
	assertPhaseCoverage(t, e, testSession, "post", "unverified", "hook_entry_absent")
	if got := e.coverage(testSession, "post")[0].str("hook_entry"); got != "absent" {
		t.Errorf("post coverage records hook_entry %q, want \"absent\"", got)
	}
}

// TestH20_ExecutionAccounting is H-10's equation with the execution half added.
// The two directions are not the same claim, and only one of them is a coverage
// failure.
func TestH20_ExecutionAccounting(t *testing.T) {
	t.Run("a result with no execution record is a coverage failure", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		ids := []string{"toolu_a", "toolu_b", "toolu_c", "toolu_d"}
		transcript := writeTranscript(t, t.TempDir(), testSession, ids, ids, nil)
		e.hookIDs(transcript, ids...)
		e.postIDs(transcript, "toolu_a", "toolu_b", "toolu_c") // toolu_d ran; nothing recorded it
		e.probe("end", testSession)

		rep := e.report(testSession)
		if len(rep.Transcripts) != 1 {
			t.Fatalf("got %d accounting groups, want 1: %+v", len(rep.Transcripts), rep.Transcripts)
		}
		tr := rep.Transcripts[0]
		if tr.IDsExecuted != 3 {
			t.Errorf("ids_executed is %d, want 3", tr.IDsExecuted)
		}
		if tr.ResultsInTranscript == nil || *tr.ResultsInTranscript != 4 {
			t.Errorf("results_in_transcript is %v, want 4", tr.ResultsInTranscript)
		}
		if !reflect.DeepEqual(tr.ExecutedButUnrecorded, []string{"toolu_d"}) {
			t.Errorf("executed_but_unrecorded is %v, want [toolu_d]", tr.ExecutedButUnrecorded)
		}
		if len(tr.DeclaredWithoutResult) != 0 {
			t.Errorf("declared_without_result is %v, want none: every declaration has a result", tr.DeclaredWithoutResult)
		}
		if !e.hasReason(rep, "execution_mismatch") || rep.Coverage.State != "unverified" {
			t.Errorf("coverage is %s (%v), want unverified with execution_mismatch", rep.Coverage.State, rep.Coverage.Reasons)
		}
		if len(rep.Declarations.WithoutExecution) != 1 || rep.Declarations.WithoutExecution[0].ToolUseID != "toolu_d" {
			t.Errorf("without_execution is %v, want the one id", rep.Declarations.WithoutExecution)
		}
	})

	t.Run("a declaration with no result is not", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		ids := []string{"toolu_a", "toolu_b", "toolu_c", "toolu_e"}
		done := []string{"toolu_a", "toolu_b", "toolu_c"}
		transcript := writeTranscript(t, t.TempDir(), testSession, ids, done, nil)
		e.hookIDs(transcript, ids...)
		e.postIDs(transcript, done...) // toolu_e was denied, failed, or has not come back
		e.probe("end", testSession)

		rep := e.report(testSession)
		if len(rep.Transcripts) != 1 {
			t.Fatalf("got %d accounting groups, want 1: %+v", len(rep.Transcripts), rep.Transcripts)
		}
		tr := rep.Transcripts[0]
		if !reflect.DeepEqual(tr.DeclaredWithoutResult, []string{"toolu_e"}) {
			t.Errorf("declared_without_result is %v, want [toolu_e]", tr.DeclaredWithoutResult)
		}
		if len(tr.ExecutedButUnrecorded) != 0 {
			t.Errorf("executed_but_unrecorded is %v, want none", tr.ExecutedButUnrecorded)
		}
		if tr.IDsExecuted != 3 {
			t.Errorf("ids_executed is %d, want 3", tr.IDsExecuted)
		}
		if e.hasReason(rep, "execution_mismatch") || rep.Coverage.State != "verified" {
			t.Errorf("coverage is %s (%v); a declaration with no result in the transcript is a denial, a tool error or a transcript that has not caught up, and none of those is this recorder failing",
				rep.Coverage.State, rep.Coverage.Reasons)
		}
		if len(rep.Declarations.WithoutExecution) != 1 || rep.Declarations.WithoutExecution[0].ToolUseID != "toolu_e" {
			t.Errorf("without_execution is %v, want the one id", rep.Declarations.WithoutExecution)
		}
	})

	t.Run("an unreadable transcript renders no execution counts", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		e.mustHook(defaultPayload().build(t)) // the default transcript path is not written
		e.mustPost(defaultPost().build(t))
		e.probe("end", testSession)

		rep := e.report(testSession)
		if len(rep.Transcripts) != 1 {
			t.Fatalf("got %d accounting groups, want 1: %+v", len(rep.Transcripts), rep.Transcripts)
		}
		tr := rep.Transcripts[0]
		if tr.Readable {
			t.Fatal("premise broken: the transcript reads as readable")
		}
		if tr.ResultsInTranscript != nil {
			t.Errorf("an unreadable transcript rendered results_in_transcript %v; unknown must be null", tr.ResultsInTranscript)
		}
		if tr.ExecutedButUnrecorded != nil || tr.DeclaredWithoutResult != nil {
			t.Errorf("an unreadable transcript rendered execution sets: %v / %v", tr.ExecutedButUnrecorded, tr.DeclaredWithoutResult)
		}
		if tr.IDsExecuted != 1 {
			t.Errorf("ids_executed is %d, want 1: it counts this store's records and is known whatever the transcript does", tr.IDsExecuted)
		}
	})
}

// TestH20_NoFaultOnThePostPathReachesTheAgent is H-1's contract for the new
// path. The call has already run by the time PostToolUse fires, so a 2 here
// cannot block it; what it does instead is put this program's failure in front
// of the model as an error against the user's tool call, which is the same bug
// wearing different clothes.
func TestH20_NoFaultOnThePostPathReachesTheAgent(t *testing.T) {
	for _, fault := range panickingFaults {
		for _, point := range postInjectionPoints {
			t.Run(fault+"@"+point, func(t *testing.T) {
				e := newEnv(t)
				e.watched(testSession)
				e.mustHook(defaultPayload().build(t))

				res := e.post(defaultPost().build(t), "RASHOMON_FAULT="+point+":"+fault)
				if res.exitCode != 0 {
					t.Fatalf("exit code %d, want 0 whatever went wrong inside this program", res.exitCode)
				}
				assertNoTraceback(t, res)

				// The fault must have fired, and the run must admit that this
				// execution was not recorded rather than pass over it.
				session := testSession
				if point == pointPostStart {
					session = "unattributed" // the payload was never parsed
				}
				if got := e.executions(session); len(got) != 0 {
					t.Errorf("an execution record was written despite a fault at %s", point)
				}
				assertPhaseCoverage(t, e, session, "post", "unverified", "internal_error")
			})
		}
	}
}

// TestH20_GoroutinePanicOnThePostPathIsContained: the correct outcome differs,
// as it does in H-1. A goroutine panic contained where it happened costs
// nothing, and the execution record still lands.
func TestH20_GoroutinePanicOnThePostPathIsContained(t *testing.T) {
	for _, point := range postInjectionPoints {
		t.Run(point, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)
			e.mustHook(defaultPayload().build(t))

			res := e.post(defaultPost().build(t), "RASHOMON_FAULT="+point+":goroutine_panic")
			if res.exitCode != 0 {
				t.Fatalf("exit code %d, want 0; a goroutine panic escaped its recover", res.exitCode)
			}
			assertNoTraceback(t, res)
			if got := e.executions(testSession); len(got) != 1 {
				t.Errorf("got %d execution records, want 1: a contained goroutine panic should not cost the record", len(got))
			}
			assertPhaseCoverage(t, e, testSession, "post", "verified", "")
		})
	}
}

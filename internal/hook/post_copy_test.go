package hook

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/store/recordtest"
)

// The fields of the execution record's copy from the shape of the input that
// ran (Post.capture, under `if len(pl.ToolInput) > 0`): the masked triple and
// the shape's digest as executed_digest are carried (postCarried), the
// shape's other fields are not read (postNotCarried), and the rest of the
// record is not the shape's (postWithheld). A field added to shape.Shape or
// store.Execution goes in one of them, here, before this test passes again.
var (
	postCarried    = []string{"StatusMasked", "RunnerDigest", "RunnerKind", "ExecutedDigest=Digest"}
	postNotCarried = []string{"Program", "VerbClass", "Argc", "MayWrite"}
	postWithheld   = []string{"Type", "SchemaVersion", "Seq", "RecordedAtMS", "ToolUseID", "SessionID", "ToolName",
		"Outcome", "ExitCode", "IsInterrupt", "DurationMS", "RuleMatch", "Backgrounded", "RulesVersion",
		"Writer", "ExecutedCommand", "ExecutedCommandReason", "ExecutedCommandBytes", "ExecutedCommandCut", "ExecutedMasked",
		"Failure", "OutputEvidence", "EditMarks", "HeadAfter", "GateOutput", "CredMech", "ExecutedCommandWithheld",
		"FailureWithheld", "OutputEvidenceWithheld", "EditMarksWithheld", "HeadAfterWithheld", "HarnessID",
		"HarnessToolName", "DeclaredVia", "OutcomeSource", "FailureType", "MCPServerSource"}
)

// TestB39_PostCopiesTheShapesMaskedTriple: the execution record takes
// status_masked, runner_digest and runner_kind, and executed_digest from the
// digest, from the shape of the input that ran, and nothing else the record
// holds depends on that input. The shape is the one Post derives, over a line
// that sets all four and no two alike; the record before the copy is the
// same call's record with no tool_input (same clock, same tool_use_id, its
// own store), which skips the derivation, and after is the record with it.
// Every field of shape.Shape and of store.Execution is in one of the lists: a
// field added to either fails here, by name, until it is placed. Break: drop
// RunnerKind from the copy, and the row names RunnerKind; swap RunnerDigest
// and RunnerKind, and it names RunnerDigest.
func TestB39_PostCopiesTheShapesMaskedTriple(t *testing.T) {
	const line = "make test && echo ok"
	at := time.UnixMilli(1_700_000_000_000)
	record := func(input bool) (store.Execution, []byte) {
		t.Helper()
		t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
		st := mustOpen(t)
		payload := map[string]any{"session_id": "s-b39", "tool_use_id": "toolu_b39", "tool_name": "Bash",
			"hook_event_name": "PostToolUse", "tool_response": map[string]any{"stdout": ""}}
		if input {
			payload["tool_input"] = map[string]any{"command": line}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		p := NewPost(st, func() time.Time { return at })
		p.Close(&Signals{}, p.Capture(bytes.NewReader(raw)))
		run, err := st.ReadRun("s-b39")
		if err != nil || len(run.Executions) != 1 {
			t.Fatalf("input %v: %d executions, %v", input, len(run.Executions), err)
		}
		return run.Executions[0], st.Key()
	}
	before, _ := record(false)
	after, key := record(true)
	in, err := json.Marshal(map[string]string{"command": line})
	if err != nil {
		t.Fatal(err)
	}
	s := shape.Derive("Bash", in, key)
	// The line sets all four, no two alike, so a dropped or swapped one shows.
	if s.StatusMasked == nil || *s.StatusMasked != shape.MaskedNone || s.RunnerKind == nil || *s.RunnerKind != shape.RunnerTest ||
		s.RunnerDigest == nil || *s.RunnerDigest == "" || s.Digest == "" || *s.RunnerDigest == s.Digest {
		t.Fatalf("%q: the shape does not set the four fields apart", line)
	}
	recordtest.AssertCopy(t, reflect.ValueOf(s), reflect.ValueOf(before), reflect.ValueOf(after),
		postCarried, postNotCarried, postWithheld)

	// A withheld field: ToolName is the record's own above; rewritten, the
	// check names it.
	set := after
	set.ToolName = "Edit"
	r := &recordtest.Recorder{}
	recordtest.AssertCopy(r, reflect.ValueOf(s), reflect.ValueOf(before), reflect.ValueOf(set),
		postCarried, postNotCarried, postWithheld)
	if want := []string{`store.Execution.ToolName is "Edit", not "Bash" as before: withheld, the copy must leave it`}; !slices.Equal(r.Errors, want) {
		t.Errorf("ToolName rewritten: AssertCopy says %q; want %q", r.Errors, want)
	}

	// A field of the shape in neither list: the lists without MayWrite name
	// it, which is where a field the shape gains is caught at this copy.
	r = &recordtest.Recorder{}
	recordtest.AssertCopy(r, reflect.ValueOf(s), reflect.ValueOf(before), reflect.ValueOf(after),
		postCarried, postNotCarried[:len(postNotCarried)-1], postWithheld)
	if want := []string{"shape.Shape.MayWrite is in neither list: say whether the copy carries it or not"}; !slices.Equal(r.Errors, want) {
		t.Errorf("MayWrite unlisted: AssertCopy says %q; want %q", r.Errors, want)
	}
}

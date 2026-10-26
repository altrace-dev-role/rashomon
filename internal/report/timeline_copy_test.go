package report

import (
	"reflect"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/store/recordtest"
)

// The fields of MaskingAsRan's copy: the masked triple comes from the as-ran
// execution record (maskingCarried), the record's other fields are not read
// (maskingNotCarried), and the rest of the declaration's shape is its own
// (maskingWithheld). A field added to store.Execution or shape.Shape goes in
// one of them, here, before this test passes again.
var (
	maskingCarried    = []string{"StatusMasked", "RunnerDigest", "RunnerKind"}
	maskingNotCarried = []string{"Type", "SchemaVersion", "Seq", "RecordedAtMS", "ToolUseID", "SessionID", "ToolName",
		"Outcome", "ExitCode", "IsInterrupt", "DurationMS", "ExecutedDigest", "RuleMatch", "Backgrounded", "RulesVersion",
		"Writer", "ExecutedCommand", "ExecutedCommandReason", "ExecutedCommandBytes", "ExecutedCommandCut", "ExecutedMasked",
		"Failure", "OutputEvidence", "EditMarks", "HeadAfter", "GateOutput", "CredMech", "ExecutedCommandWithheld",
		"FailureWithheld", "OutputEvidenceWithheld", "EditMarksWithheld", "HeadAfterWithheld", "HarnessID",
		"HarnessToolName", "DeclaredVia", "OutcomeSource", "FailureType", "MCPServerSource"}
	maskingWithheld = []string{"Program", "VerbClass", "Argc", "Digest", "MayWrite"}
)

// TestB39_MaskingAsRanCopiesTheTriple: MaskingAsRan sets a declaration's
// status_masked, runner_digest and runner_kind from its call's as-ran
// execution record, and leaves the rest of the shape as the declaration had
// it. The record and the declaration's shape are both filled by reflection,
// every field set and named by its path, so no two hold the same value: a
// field the copy drops keeps the declaration's value where the record's
// differs, two it swaps differ, and a withheld field it rewrites is not the
// declaration's. Every field of store.Execution and of shape.Shape is in one
// of the lists: a field added to either fails here, by name, until it is
// placed. Break: drop RunnerKind from the copy, and the row names
// RunnerKind; swap RunnerDigest and RunnerKind, and it names RunnerDigest;
// rebuild the shape from the record alone, and it names Program.
func TestB39_MaskingAsRanCopiesTheTriple(t *testing.T) {
	var rec store.Execution
	recordtest.Fill(reflect.ValueOf(&rec).Elem())
	// What MaskingAsRan keys on, which Fill sets: the record is its call's
	// outcome record, at schema 4 or later, and saw a tool_input.
	if rec.SchemaVersion < 4 || rec.ExecutedDigest == "" {
		t.Fatalf("filled record: schema %d, executed digest %q; want 4 or later and set", rec.SchemaVersion, rec.ExecutedDigest)
	}
	decl := store.Declaration{ToolUseID: rec.ToolUseID}
	recordtest.Fill(reflect.ValueOf(&decl.Shape).Elem())
	before := decl.Shape
	run := &store.Run{Declarations: []store.Declaration{decl}, Executions: []store.Execution{rec}}
	MaskingAsRan(run)
	after := run.Declarations[0].Shape
	recordtest.AssertCopy(t, reflect.ValueOf(rec), reflect.ValueOf(before), reflect.ValueOf(after),
		maskingCarried, maskingNotCarried, maskingWithheld)

	// A withheld field: Program is the declaration's above; rewritten, the
	// check names it.
	set := after
	program := "make"
	set.Program = &program
	r := &recordtest.Recorder{}
	recordtest.AssertCopy(r, reflect.ValueOf(rec), reflect.ValueOf(before), reflect.ValueOf(set),
		maskingCarried, maskingNotCarried, maskingWithheld)
	if want := []string{`shape.Shape.Program is "make", not "Shape.Program" as before: withheld, the copy must leave it`}; !slices.Equal(r.Errors, want) {
		t.Errorf("Program rewritten: AssertCopy says %q; want %q", r.Errors, want)
	}

	// A field of either struct in neither list: the lists without MayWrite,
	// and without Backgrounded, name it.
	r = &recordtest.Recorder{}
	recordtest.AssertCopy(r, reflect.ValueOf(rec), reflect.ValueOf(before), reflect.ValueOf(after),
		maskingCarried, maskingNotCarried, maskingWithheld[:len(maskingWithheld)-1])
	if want := []string{"shape.Shape.MayWrite is in neither list: say whether the copy carries it or withholds it"}; !slices.Equal(r.Errors, want) {
		t.Errorf("MayWrite unlisted: AssertCopy says %q; want %q", r.Errors, want)
	}
	r = &recordtest.Recorder{}
	recordtest.AssertCopy(r, reflect.ValueOf(rec), reflect.ValueOf(before), reflect.ValueOf(after),
		maskingCarried, slices.DeleteFunc(slices.Clone(maskingNotCarried), func(n string) bool { return n == "Backgrounded" }), maskingWithheld)
	if want := []string{"store.Execution.Backgrounded is in neither list: say whether the copy carries it or not"}; !slices.Equal(r.Errors, want) {
		t.Errorf("Backgrounded unlisted: AssertCopy says %q; want %q", r.Errors, want)
	}
}

package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// MASKED-OUTCOME1 (D229): a masked run whose pipe kept the runner's summary,
// read as a pass or a failure, is no run whose exit status the record cannot
// back, so the masked-runs sentence leaves it to verify's line, which names
// the runner and says it was read from its output.

func summaryRun(by *string, rules, schema int, kind string) *store.Run {
	run := tbRun(maskedCall(1, shape.MaskedTest, "r", store.ExecOK))
	for i := range run.Executions {
		run.Executions[i].SchemaVersion, run.Executions[i].RulesVersion, run.Executions[i].MaskedBy = schema, rules, by
		if kind != "" {
			run.Executions[i].GateOutput = &store.GateOutput{Kind: kind, Parser: "pytest"}
		}
	}
	return run
}

// TestMO1_ASummaryReadMaskedRunIsNotCounted: under a pass claim, a masked run
// whose tail kept a summary_pass or summary_fail at rules version 11, call
// schema 8, is not counted; every other masked run still is. Break: drop the
// readFromSummary skip and the false alarm Rishi saw comes back; or drop its
// version, stage or kind conditions and a run the output did not decide stops
// firing.
func TestMO1_ASummaryReadMaskedRunIsNotCounted(t *testing.T) {
	tail, head := "tail", "head"
	const msg = "Done. All tests pass."
	for _, tc := range []struct {
		name         string
		by           *string
		rules, schem int
		kind         string
		runs         int
	}{
		{"a summary_pass under tail", &tail, 11, 8, "summary_pass", 0},
		{"a summary_fail under tail", &tail, 11, 8, "summary_fail", 0},
		{"a later rules version", &tail, 12, 8, "summary_pass", 0},
		{"a rules version before 11", &tail, 10, 8, "summary_pass", 1},
		{"a record before schema 8", &tail, 11, 7, "summary_pass", 1},
		{"a stage that drops the summary", &head, 11, 8, "summary_pass", 1},
		{"no stage", nil, 11, 8, "summary_pass", 1},
		{"a fail line", &tail, 11, 8, "fail_line", 1},
		{"an empty run", &tail, 11, 8, "empty_run", 1},
		{"no summary", &tail, 11, 8, "no_summary", 1},
		{"no output class", &tail, 11, 8, "", 1},
	} {
		got := BuildMaskedRuns(summaryRun(tc.by, tc.rules, tc.schem, tc.kind), AccountFromMessage(msg))
		if got.Runs != tc.runs || got.Fires != (tc.runs > 0) {
			t.Errorf("%s: runs %d fires %v, want %d runs", tc.name, got.Runs, got.Fires, tc.runs)
		}
	}
}

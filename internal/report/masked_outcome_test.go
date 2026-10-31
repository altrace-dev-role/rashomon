package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// MASKED-OUTCOME1 (D229): a masked run whose pipe kept a summary that says it
// passed is no run the record cannot back, so the masked-runs sentence leaves
// it to verify's line, which names the runner and says it was read from its
// output. A summary that says it failed is read the same way
// (MASKED-FAIL-LINE1): the claim over it is raised as "done was claimed".

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
// whose tail kept a summary_pass or a summary_fail at rules version 11, call
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

// TestMO1_ADelegatedSummaryIsNotASummaryRead: a delegate run's pass line is one
// step's, and verify leaves it masked, so the sentence still counts it. Break:
// drop the Delegated check in readFromSummary.
func TestMO1_ADelegatedSummaryIsNotASummaryRead(t *testing.T) {
	tail := "tail"
	for _, tc := range []struct {
		runner string
		runs   int
	}{
		{"make test", 1}, {"npm run test", 1}, {"yarn test", 1}, {"pnpm test", 1},
		{"python3 -m pytest", 0}, {"go test", 0},
	} {
		run := summaryRun(&tail, 11, 8, "summary_pass")
		for i := range run.Executions {
			name := tc.runner
			run.Executions[i].RunnerName = &name
		}
		got := BuildMaskedRuns(run, AccountFromMessage("Done. All tests pass."))
		if got.Runs != tc.runs {
			t.Errorf("%s: runs %d, want %d", tc.runner, got.Runs, tc.runs)
		}
	}
}

// TestMO1_ABackgroundedMaskedRunIsNotASummaryRead: a backgrounded command's
// record holds the launch, not the run's output, and verify.readsOutput does
// not read it (r.Backgrounded), so readFromSummary must not either, whatever
// its caller has already filtered (BuildMaskedRuns' ended also drops it today;
// the two copies of the rule are meant to be one). Break: drop the
// Backgrounded check in readFromSummary.
func TestMO1_ABackgroundedMaskedRunIsNotASummaryRead(t *testing.T) {
	tail := "tail"
	for _, bg := range []bool{false, true} {
		run := summaryRun(&tail, 11, 8, "summary_pass")
		e := &run.Executions[0]
		e.Backgrounded = bg
		if got := readFromSummary(e); got != !bg {
			t.Errorf("backgrounded=%v: readFromSummary %v, want %v", bg, got, !bg)
		}
	}
}

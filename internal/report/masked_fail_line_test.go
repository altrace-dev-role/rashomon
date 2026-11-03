package report

import "testing"

// MASKED-FAIL-LINE1 (#149 F1): a masked run whose pipe kept a summary that
// says it failed has an outcome on the record, read from its output, as verify
// reads it (verify.readsOutput takes summary_fail for every runner, a
// delegate's included). The masked-runs sentence says the exit status "was not
// recorded", which is not so for it, and verify's line already names the
// runner and says it failed, read from its output.

// TestMFL1_AFailureReadFromItsOutputIsNotAMaskedRun: under a pass claim, a
// summary_fail kept by tail at rules 11, call schema 8, is not counted,
// whatever the runner; a delegate's pass, a fail line, a stage that drops the
// summary and an older record still are. Break: read only summary_pass in
// readFromSummary, or keep the delegate check on a failure.
func TestMFL1_AFailureReadFromItsOutputIsNotAMaskedRun(t *testing.T) {
	tail, head := "tail", "head"
	for _, tc := range []struct {
		name, runner string
		by           *string
		rules        int
		kind         string
		runs         int
	}{
		{"pytest's failure under tail", "python3 -m pytest", &tail, 11, "summary_fail", 0},
		{"make's failure under tail", "make test", &tail, 11, "summary_fail", 0},
		{"npm's failure under tail", "npm run test", &tail, 11, "summary_fail", 0},
		{"make's pass under tail", "make test", &tail, 11, "summary_pass", 1},
		{"a fail line under tail", "go test", &tail, 11, "fail_line", 1},
		{"pytest's failure under head", "python3 -m pytest", &head, 11, "summary_fail", 1},
		{"pytest's failure at rules 10", "python3 -m pytest", &tail, 10, "summary_fail", 1},
	} {
		run := summaryRun(tc.by, tc.rules, 8, tc.kind)
		for i := range run.Executions {
			name := tc.runner
			run.Executions[i].RunnerName = &name
		}
		got := BuildMaskedRuns(run, AccountFromMessage("Done. All tests pass."))
		if got.Runs != tc.runs || got.Fires != (tc.runs > 0) {
			t.Errorf("%s: runs %d fires %v, want %d runs", tc.name, got.Runs, got.Fires, tc.runs)
		}
	}
}

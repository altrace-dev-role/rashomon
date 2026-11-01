package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestGZ1b_AGreenDelegatedRunIsNotReadRed (GATEOUT-ZERO1, R29): live through
// the post and the evaluator, a `make test` or `npm test` that exited 0 is
// green when its output holds another tool's success line with a count of
// zero. Before the card pytest's bare fail form matched "0 failed" and
// "0 errors" in a delegated run's output, gate_output read summary_fail, and
// the gate was red over a run that passed: the Stop line said it failed and
// the agent was told so. Break: unanchor pytest's fail form.
func TestGZ1b_AGreenDelegatedRunIsNotReadRed(t *testing.T) {
	for _, tc := range []struct{ name, cmd, out, kind, parser string }{
		{"make over cargo", "make test",
			"cargo test\nrunning 5 tests\ntest result: ok. 5 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s\n",
			"summary_pass", "cargo"},
		{"npm over a checker", "npm test", "svelte-check found 0 errors and 0 warnings\n", "no_summary", "npm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, x := go1gGate(t, tc.cmd, tc.out)
			wantGateOutput(t, x, tc.kind, tc.parser, "")
			if g.State != verify.Green {
				t.Errorf("gate %s %s, want green: the run exited 0 and no runner said it failed", g.State, g.Reason)
			}
		})
	}
}

package verify

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// tailedPytest is the call the card is about: `python3 -m pytest -q 2>&1 |
// tail -5`, an ok run whose status the pipe hid, recorded at schema 8 under
// rules 11 with the runner's words, and the post's reading of its output as
// kind. The stage that hid the status is by.
func tailedPytest(seq int64, by, kind string) call {
	c := gateCall(seq, "pytest", store.ExecOK)
	c.prog, c.class, c.masked = "python3", shape.VerbExecute, shape.MaskedTest
	c.runnerName, c.maskedBy, c.rules = "python3 -m pytest", by, shape.RulesVersion
	if kind != "" {
		c.gateOutput = &store.GateOutput{Kind: kind, Parser: "pytest"}
	}
	return c
}

// TestMO1_AMaskedRunTakesItsOutcomeFromItsOutput: a masked run whose masking
// stage keeps the summary reads its gate's outcome from the classified
// output: summary_pass is green, summary_fail is red by output, empty_run is
// absent, and any other reading stays masked. The provenance travels on the
// run's reference. Break: drop readsOutput, and every row stays
// masked_inconclusive.
func TestMO1_AMaskedRunTakesItsOutcomeFromItsOutput(t *testing.T) {
	for _, tc := range []struct {
		kind  string
		state State
		why   Reason
		from  string
	}{
		{"summary_pass", Green, OK, OutcomeFromOutput},
		{"summary_fail", Red, FailedOutput, OutcomeFromOutput},
		{"empty_run", Absent, EmptyRun, OutcomeFromOutput},
		{"fail_line", MaskedInconclusive, MaskedUnattributable, ""},
		{"no_summary", MaskedInconclusive, MaskedNoSuccessLine, ""},
		{"unattributable", MaskedInconclusive, MaskedUnattributable, ""},
		{"", MaskedInconclusive, MaskedNoSuccessLine, ""},
	} {
		g := one(t, statesAt(t, 2, tailedPytest(1, "tail", tc.kind)))
		if g.State != tc.state || g.Reason != tc.why {
			t.Errorf("output %q: %s %s, want %s %s", tc.kind, g.State, g.Reason, tc.state, tc.why)
			continue
		}
		if g.LastRun == nil {
			t.Fatalf("output %q: no last run", tc.kind)
		}
		if g.LastRun.OutcomeFrom.String() != tc.from {
			t.Errorf("output %q: outcome_from %q, want %q", tc.kind, g.LastRun.OutcomeFrom, tc.from)
		}
		if tc.from != "" && tc.state != Absent {
			if g.LastRun.ExitCode != nil || g.LastRun.RunnerName.String() != "python3 -m pytest" || g.LastRun.MaskedBy.String() != "tail" {
				t.Errorf("output %q: run ref %+v, want no exit code, runner python3 -m pytest and masked_by tail", tc.kind, *g.LastRun)
			}
		}
	}
}

// TestMO1_OnlyAKeepingStageOnRulesElevenReadsOutput: the pipe has to have
// kept the summary line (tail, tee, cat), the run has to be written under the
// rules that name its stage, and the stage has to be recorded. A head, grep,
// wc, sort, cut or other stage, a run before rules 11 and a run with no stage
// word are masked still, whatever their output says. Break: read the output
// for any masked run, and head reads green.
func TestMO1_OnlyAKeepingStageOnRulesElevenReadsOutput(t *testing.T) {
	for _, by := range []string{"tail", "tee", "cat"} {
		g := one(t, statesAt(t, 2, tailedPytest(1, by, "summary_pass")))
		wantState(t, g, Green, OK)
	}
	for _, by := range []string{"head", "grep", "wc", "sort", "cut", "other", ""} {
		g := one(t, statesAt(t, 2, tailedPytest(1, by, "summary_pass")))
		wantState(t, g, MaskedInconclusive, MaskedWithSuccessLine)
		if g.LastRun.OutcomeFrom != SourceExit {
			t.Errorf("masked_by %q: outcome_from %q, want none", by, g.LastRun.OutcomeFrom)
		}
	}
	old := tailedPytest(1, "tail", "summary_pass")
	old.rules = shape.RulesVersion - 1
	wantState(t, one(t, statesAt(t, 2, old)), MaskedInconclusive, MaskedWithSuccessLine)
}

// TestMO1_AFailedOrBackgroundedRunIsNotRead: the output is read only for an
// ok, foreground run. A masked run the harness reported failed stays masked
// (the pipe's own status, not the runner's), whatever a summary says, and
// carries no provenance.
func TestMO1_AFailedOrBackgroundedRunIsNotRead(t *testing.T) {
	bg := tailedPytest(1, "tail", "summary_pass")
	bg.bg = true
	gb := one(t, statesAt(t, 2, bg))
	wantState(t, gb, Backgrounded, BackgroundLaunch)
	if gb.LastRun.OutcomeFrom != SourceExit {
		t.Errorf("a backgrounded run: outcome_from %q, want none", gb.LastRun.OutcomeFrom)
	}
	failed := tailedPytest(1, "tail", "summary_pass")
	failed.outcome, failed.exit = store.ExecFailed, 1
	g := one(t, statesAt(t, 2, failed))
	wantState(t, g, MaskedInconclusive, MaskedWithSuccessLine)
	if g.LastRun.OutcomeFrom != SourceExit {
		t.Errorf("a failed run: outcome_from %q, want none", g.LastRun.OutcomeFrom)
	}
}

// TestMO1_AnExitStatusGreenCarriesNoProvenance: a run whose exit status
// decided its outcome has no outcome_from, so an unpiped green reads as it
// did before.
func TestMO1_AnExitStatusGreenCarriesNoProvenance(t *testing.T) {
	g := one(t, statesAt(t, 2, gateCall(1, "go-test", store.ExecOK)))
	wantState(t, g, Green, OK)
	if g.LastRun.OutcomeFrom != SourceExit || g.LastRun.MaskedBy != 0 || g.LastRun.RunnerName != 0 {
		t.Errorf("run ref %+v, want no outcome_from, masked_by or runner_name", *g.LastRun)
	}
}

// TestMO1_ADerivedGreenGoesStale: an edit after a green read from output
// leaves the gate stale, as one after an exit-status green does, and the
// stale gate still cites the green run with its provenance. Break: leave the
// derived green out of the stale row, and the gate stays masked.
func TestMO1_ADerivedGreenGoesStale(t *testing.T) {
	g := one(t, statesAt(t, 4, tailedPytest(1, "tail", "summary_pass"), edit(3, "internal/a.py")))
	wantState(t, g, Stale, WritesAfterGreen)
	if g.LastGreen == nil || g.LastGreen.Seq != 1 || g.LastGreen.OutcomeFrom != SourceOutput {
		t.Errorf("last green %+v, want the tailed run with outcome_from %s", g.LastGreen, OutcomeFromOutput)
	}
}

// TestMO1_TheRegressionNoteRestsOnTheOutputSummary: a green, an edit, then a
// tailed run whose summary says it failed raises regression-location, and the
// gate fact and the note carry the provenance word. A tailed run whose output
// is only a fail line, or whose reading is anything else, raises none.
func TestMO1_TheRegressionNoteRestsOnTheOutputSummary(t *testing.T) {
	green := tailedPytest(2, "tail", "summary_pass")
	red := tailedPytest(8, "tail", "summary_fail")
	r := runOf(t, edit(1, "internal/before.py"), green, edit(3, "internal/a.py"), red)
	minuteClock(t, r)
	for i := range r.Declarations {
		r.Declarations[i].Project = strp("/repo")
	}
	pt := callPoint(t, r, 8)
	got := facts(t, r, 8, pt)
	if len(got) != 1 || got[0].Gate == nil {
		t.Fatalf("regressionLocations = %+v, want one gate finding", got)
	}
	g := got[0].Gate
	if g.State != Red || g.Reason != FailedOutput || g.OutcomeFrom != OutcomeFromOutput || g.ExitCode != nil {
		t.Errorf("gate fact = %+v, want red failed_output from output_summary and no exit code", *g)
	}
	if g.Program != "pytest" {
		t.Errorf("gate fact program %q, want the table word pytest, not python3", g.Program)
	}
	if f := viewAt(t, r, 8).Findings; len(f) != 1 || f[0].OutcomeFrom != OutcomeFromOutput {
		t.Errorf("steer findings %+v, want one carrying outcome_from %s", f, OutcomeFromOutput)
	}

	// A fail line alone is no regression: pytest has a summary form.
	for _, kind := range []string{"fail_line", "no_summary", ""} {
		r := runOf(t, edit(1, "internal/before.py"), green, edit(3, "internal/a.py"), tailedPytest(8, "tail", kind))
		minuteClock(t, r)
		if got := facts(t, r, 8, callPoint(t, r, 8)); len(got) != 0 {
			t.Errorf("output %q: %d regression findings, want none", kind, len(got))
		}
	}
	// A head-piped red is masked, not red, so it raises none either.
	r = runOf(t, edit(1, "internal/before.py"), green, edit(3, "internal/a.py"), tailedPytest(8, "head", "summary_fail"))
	minuteClock(t, r)
	if got := facts(t, r, 8, callPoint(t, r, 8)); len(got) != 0 {
		t.Errorf("head: %d regression findings, want none", len(got))
	}
}

// TestMO1_TheStaleNoteCarriesTheProvenance: stale-evidence over a green read
// from output names the word in the fact and the finding; the note's text is
// the render package's (TestMO1_ in internal/steer/render).
func TestMO1_TheStaleNoteCarriesTheProvenance(t *testing.T) {
	r := snRun(t, tailedPytest(2, "tail", "summary_pass"), edit(3, "internal/a.py"))
	idx := snIndex(2, "r:pytest@d:repo")
	got := staleFindings(staleAt(t, r, 3, idx))
	if len(got) != 1 {
		t.Fatalf("%d stale-evidence findings, want 1", len(got))
	}
	if f := got[0]; f.OutcomeFrom != OutcomeFromOutput {
		t.Errorf("finding outcome_from %q, want %s", f.OutcomeFrom, OutcomeFromOutput)
	}
}

// tailedMake is `make check 2>&1 | tail -10`: a delegated run, ok, whose
// status the pipe hid, and the post's reading of its output as kind.
func tailedMake(seq int64, prog, runnerName, kind string) call {
	c := tailedPytest(seq, "tail", kind)
	c.prog, c.runnerName = prog, runnerName
	return c
}

// TestMO1_ADelegatedRunIsReadOnlyWhereItIsRed: a delegate's output holds
// every step's lines, so a pass line in it is one step's. A delegated run
// takes only summary_fail and empty_run from its output; a summary_pass leaves
// it masked, the answer that may understate and never overstates. Break: let
// summary_pass read for any program, and make check reads green after a lint
// step failed behind a passing pytest.
func TestMO1_ADelegatedRunIsReadOnlyWhereItIsRed(t *testing.T) {
	for _, tc := range []struct {
		kind  string
		state State
		why   Reason
		from  string
	}{
		{"summary_pass", MaskedInconclusive, MaskedWithSuccessLine, ""},
		{"summary_fail", Red, FailedOutput, OutcomeFromOutput},
		{"empty_run", Absent, EmptyRun, OutcomeFromOutput},
		{"no_summary", MaskedInconclusive, MaskedNoSuccessLine, ""},
	} {
		for _, d := range [][2]string{{"make", "make test"}, {"npm", "npm run test"}, {"yarn", "yarn test"}, {"pnpm", "pnpm test"}} {
			g := one(t, statesAt(t, 2, tailedMake(1, d[0], d[1], tc.kind)))
			if g.State != tc.state || g.Reason != tc.why {
				t.Errorf("%s output %q: %s %s, want %s %s", d[0], tc.kind, g.State, g.Reason, tc.state, tc.why)
				continue
			}
			if g.LastRun.OutcomeFrom.String() != tc.from {
				t.Errorf("%s output %q: outcome_from %q, want %q", d[0], tc.kind, g.LastRun.OutcomeFrom, tc.from)
			}
		}
	}
}

// TestSTR43k_AFailLineRedIsNoCandidate: a red by a fail line in the output of a
// run whose exit status was its own (D61), where no summary was read in place of
// the status, is no regression. Only a red whose outcome was read from the
// output's summary (D229) joins the exit-status red as a candidate. Break: let
// every failed_output red through regressedHere.
func TestSTR43k_AFailLineRedIsNoCandidate(t *testing.T) {
	fail := gateCall(3, "go", store.ExecOK)
	fail.gateOutput = &store.GateOutput{Kind: "fail_line", Parser: "go"}
	r := runOf(t, gateCall(1, "go", store.ExecOK), edit(2, "internal/a.go"), fail)
	minuteClock(t, r)
	pt := callPoint(t, r, 3)
	if s := Evaluate(r, nil, nil, pt, time.Time{}); len(s.Gates) != 1 || s.Gates[0].State != Red || s.Gates[0].Reason != FailedOutput {
		t.Fatalf("premise: gates %+v, want one red by failed_output", s.Gates)
	}
	if got := facts(t, r, 3, pt); len(got) != 0 {
		t.Errorf("%d regression findings for a fail line the exit status did not back, want none", len(got))
	}
}

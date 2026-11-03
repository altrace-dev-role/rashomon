package recap

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// fromOutputRun is gate g with its last run (and its last green, if it is
// green) read from its output: the pipe hid the exit status and the summary
// said (D229).
func fromOutputRun(g verify.GateState) verify.GateState {
	run := *g.LastRun
	run.OutcomeFrom, run.MaskedBy, run.RunnerName = verify.SourceOutput, verify.StageWord(shape.MaskedByID("tail")), verify.RunnerWord(shape.RunnerNameID("python3 -m pytest"))
	g.LastRun = &run
	if g.LastGreen != nil {
		green := *g.LastGreen
		green.OutcomeFrom = verify.SourceOutput
		g.LastGreen = &green
	}
	return g
}

// maskedBy is a masked gate whose last run's pipe stage is by.
func maskedBy(by string) verify.GateState {
	g := gateAt("pytest:test", "test", verify.MaskedInconclusive, verify.MaskedNoSuccessLine, 7, nil)
	g.LastRun.MaskedBy, g.LastRun.RunnerName = verify.StageWord(shape.MaskedByID(by)), verify.RunnerWord(shape.RunnerNameID("python3 -m pytest"))
	return g
}

// TestMO1_TheValueLineNamesTheRunnerAndSaysWhereTheOutcomeCameFrom: a run
// whose outcome was read from its output says so, in words, beside the
// gate's own sentence, and a masked run says which pipe hid its result when
// its stage dropped the summary. The gate is named by the view's Programs,
// the runner's table word, never python3. Break: leave the suffix off, and a
// green read from a tail reads as an exit status the record holds.
func TestMO1_TheValueLineNamesTheRunnerAndSaysWhereTheOutcomeCameFrom(t *testing.T) {
	green := fromOutputRun(gateAt("pytest:test", "test", verify.Green, verify.OK, 3, nil))
	red := fromOutputRun(gateAt("pytest:test", "test", verify.Red, verify.FailedOutput, 7, nil))
	unnamed := wrote
	unnamed.UnnamedWriteSeq = 9
	const from = " (read from its output; the pipe hid the exit status)"
	for _, c := range []struct {
		name string
		v    verify.StopView
		want string
	}{
		{"verified", viewOf(verify.RowVerified, wrote, green), "verified - pytest passed after the last edit" + from},
		{"ran only", viewOf(verify.RowRanOnly, noWrite, green), "no edits; pytest passed (call 3)" + from},
		{"unnamed write", viewOf(verify.RowUnnamedWrite, unnamed, green),
			"2 files edited; pytest passed (call 3)" + from + "; call 9 may have written files after that"},
		{"red", viewOf(verify.RowRed, wrote, red), "2 files edited; pytest failed after the last edit (call 7)" + from},
		{"red, no write", viewOf(verify.RowRed, noWrite, red), "pytest failed in this turn (call 7)" + from},
		{"masked by head", viewOf(verify.RowMasked, wrote, maskedBy("head")),
			"pytest ran after the last edit, but the pipe to head hid its result (call 7)"},
		{"masked by grep, no write", viewOf(verify.RowMasked, noWrite, maskedBy("grep")),
			"pytest ran in this turn, but the pipe to grep hid its result (call 7)"},
		{"masked by another stage", viewOf(verify.RowMasked, wrote, maskedBy("other")),
			"pytest ran after the last edit, but a pipe hid its result (call 7)"},
		// A stage that keeps the summary whose output did not say, and a
		// record before schema 8, keep the sentence they had.
		{"masked by tail, output silent", viewOf(verify.RowMasked, wrote, maskedBy("tail")),
			"pytest ran after the last edit; its exit status was hidden by its line (call 7)"},
		{"masked, no stage", viewOf(verify.RowMasked, wrote, maskedBy("")),
			"pytest ran after the last edit; its exit status was hidden by its line (call 7)"},
		// An exit-status green says nothing of output.
		{"verified by exit status", viewOf(verify.RowVerified, wrote, gateAt("pytest:test", "test", verify.Green, verify.OK, 3, nil)),
			"verified - pytest passed after the last edit"},
	} {
		if got, want := ValueLine(c.v, Impact{}, "sess-1", false), lineOf(c.want, ""); got != want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}
}

// TestMO1_TheProvenanceIsAttachedToTheGatesReadFromOutput: the sentence lists
// every green program, so a clause that follows it reads as true of all of
// them. With every green read from output the clause stands alone; with some,
// it names them, and with none there is no clause. Break: make fromOutput
// answer for any one green, and go reads as read from its output.
func TestMO1_TheProvenanceIsAttachedToTheGatesReadFromOutput(t *testing.T) {
	byExit := gateAt("go:test", "test", verify.Green, verify.OK, 2, nil)
	out := fromOutputRun(gateAt("pytest:test", "test", verify.Green, verify.OK, 3, nil))
	out2 := fromOutputRun(gateAt("jest:test", "test", verify.Green, verify.OK, 4, nil))
	const rest = " read from its output; the pipe hid the exit status)"
	for _, c := range []struct {
		name  string
		gates []verify.GateState
		want  string
	}{
		{"one of two", []verify.GateState{byExit, out}, "verified - go and pytest passed after the last edit (pytest" + rest},
		{"the first of two", []verify.GateState{out, byExit}, "verified - pytest and go passed after the last edit (pytest" + rest},
		{"two of three", []verify.GateState{byExit, out, out2}, "verified - go, pytest and jest passed after the last edit (pytest and jest" + rest},
		{"all of two", []verify.GateState{out, out2}, "verified - pytest and jest passed after the last edit (read from its output; the pipe hid the exit status)"},
		{"none", []verify.GateState{byExit}, "verified - go passed after the last edit"},
	} {
		// The sentence unfitted: the 80-column fit names fewer programs
		// (and keeps the clause, which names the gates read from output).
		if got := sentence(viewOf(verify.RowVerified, wrote, c.gates...), maxGateNames, false); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

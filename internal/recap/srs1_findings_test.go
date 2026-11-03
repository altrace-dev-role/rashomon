package recap

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestSRS1n_EveryRowThatPrintsCountsItsFindings (card SPEC-RULES-STOP1,
// D-f): a row other than findings ends with the record findings' count
// clause when its Summary carries k > 0, after "advisory only" where that is
// said; the findings row says it once; k = 0 adds nothing; and the row with
// no line still prints nothing.
func TestSRS1n_EveryRowThatPrintsCountsItsFindings(t *testing.T) {
	goGreen := gateAt("go:test", "test", verify.Green, verify.OK, 3, nil)
	goRed := gateAt("go:test", "test", verify.Red, verify.FailedExit, 7, exitCode(1))
	advisoryGreen := goGreen
	advisoryGreen.Required = false
	counted := func(v verify.StopView, k int, sentence string) verify.StopView {
		v.Summary.Findings = k
		v.Summary.Sentence = sentence
		return v
	}
	oneFile := verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 5}
	for _, c := range []struct {
		name string
		v    verify.StopView
		want string
	}{
		{"no gate", counted(viewOf(verify.RowNoGate, oneFile), 1, ""),
			"1 file edited; no test or build is known for this project yet; 1 record finding(s) in this turn"},
		{"red", counted(viewOf(verify.RowRed, wrote, goRed), 2, ""),
			"2 files edited; go exited 1 after the last edit (call 7); 2 record finding(s) in this turn"},
		{"advisory", counted(viewOf(verify.RowUnverified, wrote, advisoryGreen), 1, "advisory only"),
			"2 files edited; go passed after the last edit; advisory only; 1 record finding(s) in this turn"},
		{"findings, said once", counted(viewOf(verify.RowFindings, wrote, goGreen), 1, "1 record finding(s) in this turn"),
			"2 files edited; go passed after the last edit; 1 record finding(s) in this turn"},
		{"no gate, no finding", counted(viewOf(verify.RowNoGate, oneFile), 0, ""),
			"1 file edited; no test or build is known for this project yet"},
	} {
		if got, want := ValueLine(c.v, Impact{}, "sess-1", false), lineOf(c.want, ""); got != want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}
	if got := ValueLine(counted(viewOf(verify.RowNone, verify.TurnCounts{}), 1, ""), Impact{}, "sess-1", false); got != "" {
		t.Errorf("RowNone with a finding printed %q, want nothing", got)
	}
}

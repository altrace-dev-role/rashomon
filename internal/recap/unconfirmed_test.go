package recap

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestWEND5_TheUnconfirmedRowSaysWhatWasMeasured: the unconfirmed_write row
// names the green run, then the call that did not end ok and how the record
// holds its end, in closed words: an edit that failed, a call that failed
// and may have written, one interrupted that may have written, a call with
// no recorded ending, placed nowhere. The others are placed by the green the
// guard read them against (Summary.GreenSeq): after the latest green the
// line names, before the earliest, or, between two gates' greens, after the
// earlier by name, never "after that" for a call before the run "(call N)"
// names. A turn that ran no gate names the call and the gates not run after
// it. It never reads "verified", and with no edit never "no edits". Break:
// render the row as verified's or ran_only's sentence, name the ending in
// other words, place the call by the latest green, or drop the turn that
// ran no gate.
func TestWEND5_TheUnconfirmedRowSaysWhatWasMeasured(t *testing.T) {
	goGreen := gateAt("go:test", "test", verify.Green, verify.OK, 4, nil)
	npmGreen := gateAt("npm:test", "test", verify.Green, verify.OK, 10, nil)
	goAbsent := verify.GateState{Gate: "go:test", Kind: "test", Source: "learned", Required: true, State: verify.Absent}
	one, both, absent := []verify.GateState{goGreen}, []verify.GateState{goGreen, npmGreen}, []verify.GateState{goAbsent}
	oneFile := verify.TurnCounts{Writes: 1, PathsWritten: 1, GateRuns: 1, LastWriteSeq: 2}
	ranOnly := verify.TurnCounts{GateRuns: 1}
	quiet := verify.TurnCounts{}
	at := func(turn verify.TurnCounts, call int64, end verify.WriteEnd, green int64, gates []verify.GateState) verify.StopView {
		v := viewOf(verify.RowUnconfirmedWrite, turn, gates...)
		v.Summary.Unconfirmed, v.Summary.Ending, v.Summary.GreenSeq = call, end, green
		return v
	}
	for _, c := range []struct {
		name string
		v    verify.StopView
		want string
	}{
		{"a failed edit", at(oneFile, 7, verify.EndFailedEdit, 4, one), "1 file edited; go passed (call 4); call 7, an edit, failed after that"},
		{"a failed call", at(oneFile, 7, verify.EndFailedCall, 4, one), "1 file edited; go passed (call 4); call 7 failed and may have written files after that"},
		{"an interrupted call", at(oneFile, 7, verify.EndInterrupted, 4, one), "1 file edited; go passed (call 4); call 7 was interrupted and may have written files after that"},
		{"no recorded ending", at(oneFile, 3, verify.EndNone, 4, one), "1 file edited; go passed (call 4); call 3 has no recorded ending"},
		{"no edit, a failed edit", at(ranOnly, 7, verify.EndFailedEdit, 4, one), "go passed (call 4); call 7, an edit, failed after that"},
		{"no edit, no recorded ending", at(ranOnly, 7, verify.EndNone, 4, one), "go passed (call 4); call 7 has no recorded ending"},
		{"no edit, a failed call before the green", at(ranOnly, 1, verify.EndFailedCall, 4, one), "go passed (call 4); call 1 failed and may have written files before that"},
		{"two gates, a failed edit between their greens", at(oneFile, 7, verify.EndFailedEdit, 4, both),
			"1 file edited; go and 1 more passed (call 10); call 7, an edit, failed after go passed (call 4)"},
		{"two gates, a failed call after both", at(oneFile, 12, verify.EndFailedCall, 4, both),
			"1 file edited; go and 1 more passed (call 10); call 12 failed and may have written files after that"},
		{"no gate run, a gate known", at(quiet, 3, verify.EndFailedCall, 0, absent), "call 3 failed and may have written files; go not run after it"},
		{"no gate run, an interrupted call", at(quiet, 3, verify.EndInterrupted, 0, absent), "call 3 was interrupted and may have written files; go not run after it"},
		{"no gate run, none known", at(quiet, 3, verify.EndFailedCall, 0, nil), "call 3 failed and may have written files; no test or build run after it"},
	} {
		got := ValueLine(c.v, Impact{}, "sess-1", false)
		if want := lineOf(c.want, ""); got != want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
		if strings.Contains(got, "verified") || strings.Contains(got, "no edits") {
			t.Errorf("%s: %q reads verified or no edits", c.name, got)
		}
	}
}

// TestWEND11_ARunAfterTheLastEditNamesACallThatMayHaveWrittenAfterIt: a row
// that places a gate's run after the last edit (red, unknown, masked,
// backgrounded) or its greens there (unverified) names a call that failed or
// was interrupted and may have written after that run, or one with no
// recorded ending anywhere in the turn, placed as the unconfirmed_write row
// places it: "after the last edit" counts edits alone, and the call may
// have changed the code the run saw. A failed edit applied nothing and is
// not named; a call before the run is one the run saw; a run placed before
// the last edit already says it has not run since, and one in a turn with
// no edit says nothing of an edit. Break: drop the clause from any row,
// name a failed edit, or name the call wherever the run was placed.
func TestWEND11_ARunAfterTheLastEditNamesACallThatMayHaveWrittenAfterIt(t *testing.T) {
	goRed := gateAt("go:test", "test", verify.Red, verify.FailedExit, 4, exitCode(1))
	goUnknown := gateAt("go:test", "test", verify.Unknown, verify.OutcomeUnknown, 4, nil)
	goMasked := gateAt("go:test", "test", verify.MaskedInconclusive, verify.MaskedNoSuccessLine, 4, nil)
	goPiped := goMasked
	run := *goMasked.LastRun
	run.MaskedBy = verify.StageWord(shape.MaskedByID("head"))
	goPiped.LastRun = &run
	goBack := gateAt("go:test", "test", verify.Backgrounded, verify.BackgroundLaunch, 4, nil)
	goGreen := gateAt("go:test", "test", verify.Green, verify.OK, 4, nil)
	npmGreen := gateAt("npm:test", "test", verify.Green, verify.OK, 10, nil)
	makeAbsent := verify.GateState{Gate: "make:test", Kind: "test", Source: "learned", Required: true, State: verify.Absent}
	edited := func(c verify.TurnCounts) verify.TurnCounts {
		c.Writes, c.PathsWritten, c.GateRuns, c.LastWriteSeq = 1, 1, 1, 2
		return c
	}
	sed := edited(verify.TurnCounts{FailedWriteSeq: 7, FailedWriteEnd: verify.EndFailedCall})
	for _, c := range []struct {
		name string
		v    verify.StopView
		want string
	}{
		{"red, a failed call after the run", viewOf(verify.RowRed, sed, goRed),
			"1 file edited; go exited 1 after the last edit (call 4); call 7 failed and may have written files after that"},
		{"red, an interrupted call after the run", viewOf(verify.RowRed, edited(verify.TurnCounts{FailedWriteSeq: 7, FailedWriteEnd: verify.EndInterrupted}), goRed),
			"1 file edited; go exited 1 after the last edit (call 4); call 7 was interrupted and may have written files after that"},
		{"red, a call with no ending before the run", viewOf(verify.RowRed, edited(verify.TurnCounts{UnendedWriteSeq: 3}), goRed),
			"1 file edited; go exited 1 after the last edit (call 4); call 3 has no recorded ending"},
		{"red, a failed edit after the run", viewOf(verify.RowRed, edited(verify.TurnCounts{FailedEditSeq: 7}), goRed),
			"1 file edited; go exited 1 after the last edit (call 4)"},
		{"red, a failed call before the run", viewOf(verify.RowRed, edited(verify.TurnCounts{FailedWriteSeq: 3, FailedWriteEnd: verify.EndFailedCall}), goRed),
			"1 file edited; go exited 1 after the last edit (call 4)"},
		{"red before the last edit", viewOf(verify.RowRed, verify.TurnCounts{Writes: 1, PathsWritten: 1, GateRuns: 1, LastWriteSeq: 9, FailedWriteSeq: 7, FailedWriteEnd: verify.EndFailedCall}, goRed),
			"1 file edited; go exited 1 before the last edit (call 4) and has not run since"},
		{"red with no edit", viewOf(verify.RowRed, verify.TurnCounts{GateRuns: 1, FailedWriteSeq: 7, FailedWriteEnd: verify.EndFailedCall}, goRed),
			"go exited 1 in this turn (call 4)"},
		{"unknown", viewOf(verify.RowUnknown, sed, goUnknown),
			"go ran after the last edit (call 4); its result is not in the record; call 7 failed and may have written files after that run"},
		{"masked", viewOf(verify.RowMasked, sed, goMasked),
			"go ran after the last edit; its exit status was hidden by its line (call 4); call 7 failed and may have written files after that"},
		{"masked by a pipe", viewOf(verify.RowMasked, sed, goPiped),
			"go ran after the last edit, but the pipe to head hid its result (call 4); call 7 failed and may have written files after that"},
		{"backgrounded", viewOf(verify.RowBackgrounded, sed, goBack),
			"go moved to the background after the last edit; its result is not in the record; call 7 failed and may have written files after that run"},
		{"unverified, a green beside a gate not run", viewOf(verify.RowUnverified, sed, goGreen, makeAbsent),
			"1 file edited; go passed after the last edit, make not run after it; call 7 failed and may have written files after go passed (call 4)"},
		{"unverified, the call between two greens", viewOf(verify.RowUnverified, sed, goGreen, npmGreen, makeAbsent),
			"1 file edited; go and 1 more passed after the last edit, make not run after it; call 7 failed and may have written files after go passed (call 4)"},
		{"unverified, the call before the greens", viewOf(verify.RowUnverified, edited(verify.TurnCounts{FailedWriteSeq: 3, FailedWriteEnd: verify.EndFailedCall}), goGreen, makeAbsent),
			"1 file edited; go passed after the last edit, make not run after it"},
	} {
		if got, want := ValueLine(c.v, Impact{}, "sess-1", false), lineOf(c.want, ""); got != want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}
}

// TestWEND9_TheUnnamedRowPlacesItsCallByTheGreenItRead: the unnamed_write
// row is read where a write the record cannot name came after the earliest
// last green of the turn's gates, and says where by that green: between two
// gates' greens, after the earlier one by name, not "after that", which
// would place it after the latest run the line names. One gate reads as it
// always did. Break: place the call by the latest green.
func TestWEND9_TheUnnamedRowPlacesItsCallByTheGreenItRead(t *testing.T) {
	goGreen := gateAt("go:test", "test", verify.Green, verify.OK, 4, nil)
	npmGreen := gateAt("npm:test", "test", verify.Green, verify.OK, 10, nil)
	turn := verify.TurnCounts{Writes: 1, PathsWritten: 1, GateRuns: 2, LastWriteSeq: 2, UnnamedWriteSeq: 7}
	for _, c := range []struct {
		name  string
		gates []verify.GateState
		want  string
	}{
		{"one gate", []verify.GateState{goGreen}, "1 file edited; go passed (call 4); call 7 may have written files after that"},
		{"between two gates' greens", []verify.GateState{goGreen, npmGreen},
			"1 file edited; go and 1 more passed (call 10); call 7 may have written files after go passed (call 4)"},
	} {
		v := viewOf(verify.RowUnnamedWrite, turn, c.gates...)
		v.Summary.GreenSeq = 4
		if got, want := ValueLine(v, Impact{}, "sess-1", false), lineOf(c.want, ""); got != want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}
}

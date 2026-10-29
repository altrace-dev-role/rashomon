package recap

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

var doneKey = []byte("install-key-for-the-done-flag")

// scanOf is the comparator's reading of a final message against gates, as
// EvaluateStop takes it: head and tail windows, a relevant write.
func scanOf(head, tail string, gates ...verify.GateState) verify.ClaimScan {
	var tl []byte
	if tail != "" {
		tl = []byte(tail)
	}
	return verify.Claims([]byte(head), tl, doneKey, gates, true, verify.SourceFinalMessage, nil)
}

// TestPG9_UnbackedDoneFlagFiresOnAClaimWithoutAGreen: the done flag fires on
// a strict claim whose verdict is a state other than green, while the row is
// not verified; it names the value sentence of the row. A green verdict, on
// a verified row or on one that is not (ran-only, B153), a failure report, a
// loose claim and a claim no gate speaks to do not fire it (card 2.4 R7,
// PF-PG9, D79).
func TestPG9_UnbackedDoneFlagFiresOnAClaimWithoutAGreen(t *testing.T) {
	red := gateAt("go:test", "test", verify.Red, verify.FailedExit, 7, exitCode(1))
	stale := gateAt("go:test", "test", verify.Stale, verify.WritesAfterGreen, 3, nil)
	green := gateAt("go:test", "test", verify.Green, verify.OK, 3, nil)
	const pass = "All 42 tests pass."

	fires := []struct {
		name     string
		v        verify.StopView
		sentence string
	}{
		{"red", viewOf(verify.RowRed, wrote, red), "2 files edited; go exited 1 after the last edit (call 7)"},
		{"stale", viewOf(verify.RowUnverified, wrote, stale), "2 files edited, no test or build run after the last edit"},
		{"no gate row", viewOf(verify.RowNoGate, wrote), "no test or build run is recorded for this turn"},
	}
	fires[0].v.Scan = scanOf(pass, "", red)
	fires[1].v.Scan = scanOf(pass, "", stale)
	fires[2].v.Scan = verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(verify.Absent)}}}
	for _, c := range fires {
		want := prefix + "done was claimed; the record shows " + c.sentence + ".\n" + indent + "→ rashomon report --session sess-1"
		got, ok := DoneFlag(c.v, "sess-1", false)
		if !ok || got != want {
			t.Errorf("%s: DoneFlag = %q, %v\nwant %q, true", c.name, got, ok, want)
		}
	}
	plugin := fires[0].v
	if got, _ := DoneFlag(plugin, "sess-1", true); got != prefix+"done was claimed; the record shows "+fires[0].sentence+".\n"+indent+"→ /rashomon:report --session sess-1" {
		t.Errorf("plugin pointer: DoneFlag = %q", got)
	}

	silent := []struct {
		name string
		v    verify.StopView
	}{
		{"green", viewOf(verify.RowVerified, wrote, green)},
		{"green, ran-only", viewOf(verify.RowRanOnly, noWrite, green)},
		{"failure report", viewOf(verify.RowRed, wrote, red)},
		{"loose claim", viewOf(verify.RowRed, wrote, red)},
		{"no gate known", viewOf(verify.RowRed, wrote, red)},
		{"verified row", viewOf(verify.RowVerified, wrote, green)},
	}
	silent[0].v.Scan = scanOf(pass, "", green)
	silent[1].v.Scan = scanOf(pass, "", green)
	silent[2].v.Scan = scanOf("All 42 tests pass, one test failed.", "", red)
	silent[3].v.Scan = scanOf(pass, "and more after the cut", red)
	silent[4].v.Scan = scanOf("Lint is clean.", "", red)
	silent[5].v.Scan = verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimLintClean, Strict: true, Verdict: verify.Verdict(verify.Red)}}}
	for _, c := range silent[:5] {
		if len(c.v.Scan.Claims) == 0 {
			t.Fatalf("%s: premise: the message holds no claim", c.name)
		}
	}
	for _, c := range silent {
		if got, ok := DoneFlag(c.v, "sess-1", false); ok || got != "" {
			t.Errorf("%s: DoneFlag = %q, %v, want nothing (scan %+v)", c.name, got, ok, c.v.Scan.Claims)
		}
	}
}

// TestDONE_RedBeforeTheLastEditSaysSo: p1's go test exits 1 at call 3; p2
// edits at call 5, its first, and claims the tests pass. The red ran before
// the edit and not since, and the done flag says so (card 2.4r R1a).
func TestDONE_RedBeforeTheLastEditSaysSo(t *testing.T) {
	red := gateAt("go:test", "test", verify.Red, verify.FailedExit, 3, exitCode(1))
	v := viewOf(verify.RowRed, verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 5, FirstSeq: 5}, red)
	v.Scan = scanOf("All 42 tests pass.", "", red)
	want := prefix + "done was claimed; the record shows 1 file edited; go exited 1 before the last edit (call 3) and has not run since.\n" +
		indent + "→ rashomon report --session sess-1"
	if got, ok := DoneFlag(v, "sess-1", false); !ok || got != want {
		t.Errorf("DoneFlag = %q, %v\nwant %q, true", got, ok, want)
	}
}

// TestDONE_RedFromAnEarlierTurnSaysSo: p1's go test exits 1 at call 2; p2
// writes nothing, runs npm test green at call 5, its first, and claims the
// tests pass. The npm run is what lets the turn speak (TurnSummary's RowNone
// return); the red is named as an earlier turn's (card 2.4r R1b).
func TestDONE_RedFromAnEarlierTurnSaysSo(t *testing.T) {
	red := gateAt("go:test", "test", verify.Red, verify.FailedExit, 2, exitCode(1))
	green := gateAt("npm:test", "test", verify.Green, verify.OK, 5, nil)
	v := viewOf(verify.RowRed, verify.TurnCounts{GateRuns: 1, FirstSeq: 5}, green, red)
	v.Scan = scanOf("All 42 tests pass.", "", green, red)
	want := prefix + "done was claimed; the record shows go exited 1 in an earlier turn (call 2) and has not run since.\n" +
		indent + "→ rashomon report --session sess-1"
	if got, ok := DoneFlag(v, "sess-1", false); !ok || got != want {
		t.Errorf("DoneFlag = %q, %v\nwant %q, true", got, ok, want)
	}
}

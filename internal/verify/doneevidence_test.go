package verify

import (
	"regexp"
	"testing"
)

// doneView is a Stop view the done flag fires on: a red row, one write at
// seq 9, one strict done claim that reads red, over gates. Each call builds
// a fresh view, since a struct copy would share the gates slice.
func doneView(gates ...GateState) StopView {
	return StopView{
		Snap:    Snapshot{Point: Point{Seq: 20}, Gates: gates},
		Summary: Summary{Row: RowRed},
		Turn:    TurnCounts{Writes: 1, LastWriteSeq: 9},
		Scan:    ClaimScan{Claims: []ClaimVerdict{{Class: ClaimDone, Strict: true, Verdict: Verdict(Red)}}},
	}
}

// doneGate is a required gate in state st whose last run is at seq, none for 0.
func doneGate(id string, seq int64, st State) GateState {
	g := GateState{Gate: id, Required: true, State: st}
	if seq > 0 {
		g.LastRun = &RunRef{Seq: seq}
	}
	return g
}

var hex16 = regexp.MustCompile(`^[0-9a-f]{16}$`)

// TestDONE1k_DigestSortsGatesAtZeroOneTwo: the digest is 16 hex digits at 0,
// 1 and 2 gates, differs between the three, does not depend on the order
// the gates come in, and passes over a gate off the turn or not required.
func TestDONE1k_DigestSortsGatesAtZeroOneTwo(t *testing.T) {
	a, b := doneGate("learned:go-test", 4, Red), doneGate("learned:go-build", 6, Green)
	zero, one, two := DoneEvidence(doneView()), DoneEvidence(doneView(a)), DoneEvidence(doneView(a, b))
	for i, d := range []string{zero, one, two} {
		if !hex16.MatchString(d) {
			t.Errorf("digest at %d gates = %q, want 16 hex digits", i, d)
		}
	}
	if zero == one || one == two || zero == two {
		t.Errorf("digests at 0, 1 and 2 gates = %q, %q, %q, want three distinct", zero, one, two)
	}
	if ba := DoneEvidence(doneView(b, a)); ba != two {
		t.Errorf("digest of (b, a) = %q, want %q as for (a, b)", ba, two)
	}
	off := doneGate("spec:lint", 7, Red)
	off.offTurn = true
	loose := doneGate("ci:0", 8, Red)
	loose.Required = false
	if got := DoneEvidence(doneView(a, b, off, loose)); got != two {
		t.Errorf("digest with a gate off the turn and one not required = %q, want %q", got, two)
	}
}

// TestDONE1r_NewUnmetKindOverUnchangedGatesDelivers: every input the done
// flag decides on moves the digest, a new unmet expected kind over unchanged
// gates included; the point's seq, a loose claim and a repeated strict claim
// do not.
func TestDONE1r_NewUnmetKindOverUnchangedGatesDelivers(t *testing.T) {
	base := func() StopView { return doneView(doneGate("learned:go-test", 4, Red)) }
	want := DoneEvidence(base())
	moves := []struct {
		name string
		edit func(*StopView)
	}{
		{"row", func(v *StopView) { v.Summary.Row = RowNoGate }},
		{"last write", func(v *StopView) { v.Turn.LastWriteSeq = 11 }},
		{"gate run", func(v *StopView) { v.Snap.Gates[0].LastRun.Seq = 5 }},
		{"gate state", func(v *StopView) { v.Snap.Gates[0].State = Stale }},
		{"strict claim", func(v *StopView) {
			v.Scan.Claims = append(v.Scan.Claims, ClaimVerdict{Class: ClaimTestsPass, Strict: true, Verdict: Verdict(Red)})
		}},
		{"unmet kind", func(v *StopView) {
			v.Expected = []Expected{{Kind: "run", Reason: "no gate declares it"}}
			v.Obligations = []Obligation{{Kind: ExpectedEvidence, State: ObligationUnmet, ID: ObligationID("", ExpectedEvidence, "run")}}
		}},
		{"done window", func(v *StopView) {
			v.Done = &DoneWindow{Turn: TurnCounts{Writes: 1, LastWriteSeq: 3}, Summary: v.Summary}
		}},
	}
	for _, m := range moves {
		v := base()
		m.edit(&v)
		if got := DoneEvidence(v); got == want {
			t.Errorf("%s: digest %q unchanged, want it moved", m.name, got)
		}
	}
	stays := []struct {
		name string
		edit func(*StopView)
	}{
		{"point seq", func(v *StopView) { v.Snap.Point.Seq = 21 }},
		{"loose claim", func(v *StopView) {
			v.Scan.Claims = append(v.Scan.Claims, ClaimVerdict{Class: ClaimTestsPass, Strict: false, Verdict: Verdict(Red)})
		}},
		{"repeated strict claim", func(v *StopView) { v.Scan.Claims = append(v.Scan.Claims, v.Scan.Claims[0]) }},
	}
	for _, s := range stays {
		v := base()
		s.edit(&v)
		if got := DoneEvidence(v); got != want {
			t.Errorf("%s: digest %q, want %q unchanged", s.name, got, want)
		}
	}
}

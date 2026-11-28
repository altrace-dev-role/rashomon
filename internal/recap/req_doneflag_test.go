package recap

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// sessionStores holds the recorded sessions the session fold's tests read.
// This package reads them and never writes there: that set fails on any file
// its SHA256SUMS.txt does not list, so each store is copied first, and the
// copy is opened with store.Open, which writes the install files a store
// needs (the set holds only runs/<session>/*.ndjson) into the copy alone.
const sessionStores = "../graph/session/testdata/sessions/stores"

// copiedStore copies the store under src into a new temporary directory and
// returns its root.
func copiedStore(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o700)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// requirementRows counts the rows of obs whose Source is requirement.
func requirementRows(obs []verify.Obligation) int {
	n := 0
	for _, o := range obs {
		if o.Source == verify.SourceRequirement {
			n++
		}
	}
	return n
}

// withReqRows is v with one requirement row of each state, in the shape a
// Stop writes them: an expected_evidence row keyed by ReqFamily, its source
// requirement, its id in Req; after v's rows, or before them when first, so
// no reader that walks the ledger by position is spared.
func withReqRows(v verify.StopView, first bool) verify.StopView {
	rows := make([]verify.Obligation, 0, 4)
	for _, r := range []struct {
		id    string
		state verify.ObligationState
	}{
		{verify.PredFileExists + ":pkg/removed.go", verify.ObligationUnmet},
		{verify.PredGateGreen + ":spec:unit", verify.ObligationMet},
		{verify.PredGateGreen + ":spec:lint", verify.ObligationOpen},
		{verify.PredTestNamed + ":TestNone", verify.ObligationUnknown},
	} {
		rows = append(rows, verify.Obligation{ID: verify.ObligationID(verify.ScopeSession, verify.ExpectedEvidence, verify.ReqFamily(r.id)),
			Scope: verify.ScopeSession, Kind: verify.ExpectedEvidence, Source: verify.SourceRequirement, State: r.state,
			Evidence: []int64{}, Req: r.id})
	}
	if first {
		v.Obligations = append(rows, v.Obligations...)
	} else {
		v.Obligations = append(slices.Clone(v.Obligations), rows...)
	}
	return v
}

// TestReq_DoneFlagIgnoresReqs: no requirement row sets or moves the done flag,
// in two parts.
//
// Views: each Stop view the done flag fires on, by a red, a stale, a no-gate
// row and an unmet expected kind on a verified row, and one strict claim it
// backs, are read with requirement rows of every state after and before the
// ledger's own: DoneFlag's text and firing and DoneBacked read the same. Each
// view's premise, that it fires or is backed, is checked first, so the pin
// never compares two silences.
//
// Recorded stores: every recorded session's Stop, under no spec and under a
// task expecting judgment_disclosed, with Reqs nil and with three witness
// rows (a handle removed since its prompt, file_exists, known, not holding;
// a gate no spec declares that the caller says holds; a check that could not
// be made). The filled Stop must hold all three rows, so the pin cannot pass
// on a Stop that dropped them, and DoneFlag and DoneBacked read the same.
// The flag fires on none of them, which the log states: the recorded runs
// hold no coverage record, so each reads incomplete (coverage_absent) and
// every expected row unknown, never unmet (INV-2), and none holds a relevant
// write by verify's reading, so no done claim is read (recap-pin r1: 0
// complete, 0 claims read, fired 0, backed 42 of 42 per pass). The views
// part is the one that fires; this part runs the rows through EvaluateStop
// over real sessions' shapes.
func TestReq_DoneFlagIgnoresReqs(t *testing.T) {
	t.Run("views", func(t *testing.T) {
		red := gateAt("go:test", "test", verify.Red, verify.FailedExit, 7, exitCode(1))
		stale := gateAt("go:test", "test", verify.Stale, verify.WritesAfterGreen, 3, nil)
		green := gateAt("go:test", "test", verify.Green, verify.OK, 3, nil)
		const pass = "All 42 tests pass."
		redView := viewOf(verify.RowRed, wrote, red)
		redView.Scan = scanOf(pass, "", red)
		staleView := viewOf(verify.RowUnverified, wrote, stale)
		staleView.Scan = scanOf(pass, "", stale)
		noGate := viewOf(verify.RowNoGate, wrote)
		noGate.Scan = doneClaim(verify.Absent)
		unmet := viewOf(verify.RowVerified, wrote, green)
		unmet.Scan = doneClaim(verify.Green)
		unmet.Expected = []verify.Expected{{Kind: "real_data_run", State: verify.ObligationUnmet, Reason: "no gate declares it"}}
		backed := viewOf(verify.RowVerified, wrote, green)
		backed.Scan = doneClaim(verify.Green)
		views := []struct {
			name  string
			v     verify.StopView
			fires bool
		}{
			{"red", redView, true},
			{"stale", staleView, true},
			{"no gate row", noGate, true},
			{"unmet expected kind", withRows(unmet), true},
			{"backed", withRows(backed), false},
		}
		for _, c := range views {
			flag, on := DoneFlag(c.v, "sess-1", false)
			if on != c.fires || !c.fires && !DoneBacked(c.v) {
				t.Fatalf("%s: premise: DoneFlag %v %q, DoneBacked %v; want the flag to fire %v, or the claim backed",
					c.name, on, flag, DoneBacked(c.v), c.fires)
			}
			for _, first := range []bool{false, true} {
				w := withReqRows(c.v, first)
				if n := requirementRows(w.Obligations); n != 4 {
					t.Fatalf("%s: premise: %d requirement rows, want 4", c.name, n)
				}
				if got, ok := DoneFlag(w, "sess-1", false); got != flag || ok != on {
					t.Errorf("%s (rows first %v): DoneFlag moved with requirement rows:\nnil    %v %q\nfilled %v %q", c.name, first, on, flag, ok, got)
				}
				if b0, b1 := DoneBacked(c.v), DoneBacked(w); b0 != b1 {
					t.Errorf("%s (rows first %v): DoneBacked %v with no requirement row, %v with them", c.name, first, b0, b1)
				}
			}
		}
	})

	t.Run("recorded stores", func(t *testing.T) {
		ents, err := os.ReadDir(sessionStores)
		if err != nil {
			t.Fatal(err)
		}
		witness := []verify.ReqRow{
			{ID: verify.PredFileExists + ":pkg/removed.go", Pred: verify.PredFileExists, Arg: "pkg/removed.go", Known: true},
			{ID: verify.PredGateGreen + ":spec:none", Pred: verify.PredGateGreen, Arg: "spec:none", Holds: true, Known: true},
			{ID: verify.PredTestNamed + ":TestNone", Pred: verify.PredTestNamed, Arg: "TestNone"},
		}
		specs := []struct {
			name string
			sp   *spec.Compiled
		}{
			{"no spec", nil},
			{"task", &spec.Compiled{Task: &spec.Task{ID: "t1", Goal: "g", Decision: "d", ExpectedEvidence: []string{"judgment_disclosed"}}}},
		}
		now := time.UnixMilli(1_790_000_000_000)
		stops, fired, backed, complete := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			st, err := store.Open(copiedStore(t, filepath.Join(sessionStores, e.Name())))
			if err != nil {
				t.Fatalf("%s: %v", e.Name(), err)
			}
			ids, err := st.Runs()
			if err != nil {
				t.Fatalf("%s: %v", e.Name(), err)
			}
			for _, id := range ids {
				r, err := st.ReadRun(id)
				if err != nil {
					t.Fatalf("%s %s: %v", e.Name(), id, err)
				}
				pt := verify.Point{Kind: verify.PointStop, Seq: r.LastSeq(), SessionID: id}
				for _, s := range specs {
					in := verify.StopInput{Key: doneKey, Head: []byte("All 42 tests pass. The fix is complete.")}
					bare := verify.EvaluateStop(r, nil, s.sp, pt, now, in)
					in.Reqs = witness
					filled := verify.EvaluateStop(r, nil, s.sp, pt, now, in)
					if n := requirementRows(filled.Obligations); n != len(witness) {
						t.Errorf("%s, %s: %d requirement rows with Reqs filled, want %d", e.Name(), s.name, n, len(witness))
					}
					flagBare, onBare := DoneFlag(bare, id, false)
					flagFilled, onFilled := DoneFlag(filled, id, false)
					if flagBare != flagFilled || onBare != onFilled {
						t.Errorf("%s, %s: DoneFlag moved with requirement rows:\nnil    %v %q\nfilled %v %q", e.Name(), s.name, onBare, flagBare, onFilled, flagFilled)
					}
					b0, b1 := DoneBacked(bare), DoneBacked(filled)
					if b0 != b1 {
						t.Errorf("%s, %s: DoneBacked %v with Reqs nil, %v filled", e.Name(), s.name, b0, b1)
					}
					stops[s.name]++
					if bare.Snap.Recording == verify.Complete {
						complete[s.name]++
					}
					if onBare {
						fired[s.name]++
					}
					if b0 {
						backed[s.name]++
					}
				}
			}
		}
		if stops["no spec"] == 0 {
			t.Fatal("no recorded Stop was read")
		}
		for _, s := range specs {
			t.Logf("%s: %d recorded Stops, %d complete; the done flag fired on %d, backed on %d",
				s.name, stops[s.name], complete[s.name], fired[s.name], backed[s.name])
		}
	})
}

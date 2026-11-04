package verify

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Card F2: a gate answers for a turn only where a call of the turn ran in
// its project. eng-4's session 02bc13cc ran `go vet ./internal/gateinfer
// 2>&1 | head -3` in rv-se4-gi1w at 09:24 (call 3060); the pipe hid vet's
// status, a true catch at the act. The R67 replay then repeats "go vet ran
// before the last edit, but the pipe to head hid its result (call 3060)" on
// 13 later Stops and one prompt over seven hours, ten of them in turns whose
// every call ran in another worktree or under /tmp. The final messages are
// those turns' word for word; the paths are theirs under /wt.

const (
	f2Vetted  = "/wt/rv-se4-gi1w"
	f2Other   = "/wt/rv-se4-gi1w5-red"
	f2Said    = "GATE-INFER1 is finished and verified: `fix/gate-infer1` now points at `2bfa9ee0`, and I sent FIX BRANCH READY."
	f2SaidRPS = "b2 so far: generate, gofmt, vet under both tags, `--check` and all nine staticcheck U1000 passes are green."
)

// f2MaskedVet is call 3060's shape: go vet, its status hidden by the pipe.
func f2MaskedVet(seq int64) call {
	c := gateCall(seq, "go-vet", store.ExecOK)
	c.class, c.masked = shape.VerbPackage, shape.MaskedTest
	return c
}

// f2Unkeyed makes call seq's declaration one whose project the record does
// not say: no key and no cwd (projectOf nil).
func f2Unkeyed(t *testing.T, r *store.Run, seq int64) {
	t.Helper()
	for i := range r.Declarations {
		if d := &r.Declarations[i]; d.ToolUseID == callID(seq) {
			d.Project, d.CWD = nil, nil
			return
		}
	}
	t.Fatalf("f2Unkeyed: no declaration t%d", seq)
}

// f2Vet is the view's go vet row, which must be the masked one, or the test
// proves nothing. A learned row is found by its family's runner: its id ends
// in the family's location ("learned:test:r:go-vet@d:repo").
func f2Vet(t *testing.T, v StopView) GateState {
	t.Helper()
	for _, g := range v.Snap.Gates {
		if strings.HasPrefix(g.Family, "r:go-vet@") {
			if g.State != MaskedInconclusive {
				t.Fatalf("premise: go vet is %s %s, want masked", g.State, g.Reason)
			}
			return g
		}
	}
	t.Fatalf("premise: no go vet row in %+v", v.Snap.Gates)
	return GateState{}
}

// TestF2a_AMaskedRunInAnotherWorktreeIsNotThisTurns (8453, 12:38): turn 1
// edited rv-se4-gi1w and ran vet there behind a pipe; turn 2 ran in
// rv-se4-gi1w5-red, wrote one commit message under /tmp and said 8449's
// first sentence. No call of turn 2 is in rv-se4-gi1w, so its done claim does
// not answer to vet there, and the Stop names no call of turn 1. Controls,
// each still flagged: turn 2 ran in the vetted worktree; no call of turn 2
// is in a project (the session's gates, as before the card); a call of turn
// 2 whose project the record does not say; the gate's own project not known;
// a call in the directory that holds the worktree; a call in a repository
// inside it. Break: read a gate in a turn none of whose calls is in its
// project, or drop any of the fail-closed arms.
func TestF2a_AMaskedRunInAnotherWorktreeIsNotThisTurns(t *testing.T) {
	for _, c := range []struct {
		name, project, cwd string
		unkeyedVet         bool
		unkeyedCall        bool
		flagged            bool
	}{
		{"turn 2 in another worktree", f2Other, f2Other, false, false, false},
		{"control: turn 2 in the vetted worktree", f2Vetted, f2Vetted, false, false, true},
		{"control: no call of turn 2 in a project", "", "/home/u", false, false, true},
		{"control: a call of turn 2 not keyed", f2Other, f2Other, false, true, true},
		{"control: the vet run's project not known", f2Other, f2Other, true, false, true},
		{"control: a call in the directory holding the worktree", "/wt", "/wt", false, false, true},
		{"control: a call in a repository inside the worktree", f2Vetted + "/third_party/x", f2Vetted + "/third_party/x", false, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, scWrite(1, f2Vetted+"/internal/gateinfer/gateinfer.go"), f2MaskedVet(2),
				bashWrite(3, "/tmp/rk-se4-w5/msg-w5.txt"), scRead(4))
			scAt(t, r, 1, f2Vetted, f2Vetted)
			scAt(t, r, 2, f2Vetted, f2Vetted)
			scAt(t, r, 3, c.project, c.cwd)
			scAt(t, r, 4, c.project, c.cwd)
			if c.unkeyedVet {
				f2Unkeyed(t, r, 2)
			}
			if c.unkeyedCall {
				f2Unkeyed(t, r, 4)
			}
			scPrompt(r, "p1", 1, 2)
			scPrompt(r, "p2", 3, 4)
			v := scStop(r, "p2", f2Said, f2Other, nil)
			f2Vet(t, v)
			if got := scFlagged(t, v); got != c.flagged {
				t.Errorf("flagged %t (row %s, claims %+v); want %t", got, v.Summary.Row, v.Scan.Claims, c.flagged)
			}
		})
	}
}

// TestF2b_EditsInAnotherWorktreeReadNoMaskedRow (10171, 17:01): turn 2
// edited rv-se4-rps alone, and the row was masked by vet in rv-se4-gi1w:
// "go vet ran before the last edit, but the pipe to head hid its result
// (call 3060)". The edit is not vet's, nor is the turn, so the row is not
// masked. Control: the same edit in rv-se4-gi1w keeps the masked row.
// Break: choose the row from a gate whose project no call of the turn is in.
func TestF2b_EditsInAnotherWorktreeReadNoMaskedRow(t *testing.T) {
	for _, c := range []struct {
		name, root string
		row        Row
	}{
		{"an edit in rv-se4-rps", "/wt/rv-se4-rps", RowNoGate},
		{"control: an edit in rv-se4-gi1w", f2Vetted, RowMasked},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, scWrite(1, f2Vetted+"/internal/gateinfer/gateinfer.go"), f2MaskedVet(2),
				scWrite(3, c.root+"/scripts/pluginsum/main.go"))
			scAt(t, r, 1, f2Vetted, f2Vetted)
			scAt(t, r, 2, f2Vetted, f2Vetted)
			scAt(t, r, 3, c.root, "/wt/rv-se4-rps")
			scPrompt(r, "p1", 1, 2)
			scPrompt(r, "p2", 3)
			v := scStop(r, "p2", f2SaidRPS, "/wt/rv-se4-rps", nil)
			f2Vet(t, v)
			if v.Summary.Row != c.row || v.Turn.Writes != 1 {
				t.Errorf("row %s, turn writes %d; want %s, 1", v.Summary.Row, v.Turn.Writes, c.row)
			}
		})
	}
}

// TestF2c_ATurnsOwnGreenVerifiesBesideAnotherWorktreesMask: turn 2 edited
// rv-se4-gi1w5-red and ran go test green there after the edit; vet stays
// masked in rv-se4-gi1w, which turn 2 never ran in. The turn is verified by
// its own gate, not a findings row with nothing found. Control: the same
// turn in rv-se4-gi1w reads vet's mask. Break: read the record-green point
// over every gate while the row reads the turn's.
func TestF2c_ATurnsOwnGreenVerifiesBesideAnotherWorktreesMask(t *testing.T) {
	for _, c := range []struct {
		name, root string
		row        Row
	}{
		{"turn 2 in another worktree", f2Other, RowVerified},
		{"control: turn 2 in the vetted worktree", f2Vetted, RowMasked},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, scWrite(1, f2Vetted+"/internal/gateinfer/gateinfer.go"), f2MaskedVet(2),
				scWrite(3, c.root+"/internal/verify/gi1_inferred_test.go"), gateCall(4, "go-test", store.ExecOK))
			scAt(t, r, 1, f2Vetted, f2Vetted)
			scAt(t, r, 2, f2Vetted, f2Vetted)
			scAt(t, r, 3, c.root, c.root)
			scAt(t, r, 4, c.root, c.root)
			scPrompt(r, "p1", 1, 2)
			scPrompt(r, "p2", 3, 4)
			v := scStop(r, "p2", f2Said, c.root, nil)
			f2Vet(t, v)
			if v.Summary.Row != c.row {
				t.Errorf("row %s (gates %+v); want %s", v.Summary.Row, v.Snap.Gates, c.row)
			}
		})
	}
}

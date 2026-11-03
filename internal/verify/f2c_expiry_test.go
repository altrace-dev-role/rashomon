package verify

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Card F2c, after F2 (#255): the six F2 lines F2 leaves in the R67 replay of
// eng-4's session 02bc13cc, all "go vet ran before the last edit, but the pipe
// to head hid its result (call 3076)". Five are in turns that edited
// rv-se4-gi1w after that masked vet (8164's turn wrote gi1w at 7930 to 7976),
// so the line answers a done claim with a run the turn's own edits came
// after. One (9376) is in a turn whose one call in a directory holding gi1w
// read two files and printed a separator and the date (call 9325: cat, echo,
// cat, date), every other call in rv-se4-gi1w5-red. The manager's rulings of
// 11-03: a masked last run with a covered write after it reads Stale, cites
// no call, and never drops the gate; the classic catch (edit, go test | head,
// done) keeps its masked line; a call acts on the projects where its writing
// or executing segments run, and a call that only reads acts on nothing.

// f2cExpired is the reason a masked last run reads beside a covered write
// after it: a word of the closed list (state.go), frozen here as the record
// carries it.
const f2cExpired Reason = "masked_then_writes"

// f2cVetRow is the view's go vet row, in whatever state it reads.
func f2cVetRow(t *testing.T, v StopView) GateState {
	t.Helper()
	for _, g := range v.Snap.Gates {
		if strings.HasPrefix(g.Family, "r:go-vet@") {
			return g
		}
	}
	t.Fatalf("premise: no go vet row in %+v", v.Snap.Gates)
	return GateState{}
}

// f2cGreenVet is go vet with its status read: call 3076's line with no pipe.
func f2cGreenVet(seq int64) call {
	c := f2MaskedVet(seq)
	c.masked = shape.MaskedNone
	return c
}

// TestF2d_AMaskedRunWithAWriteAfterItReadsStale: a masked last run followed
// by a write its gate covers is stale (masked_then_writes), as a red run with
// writes after it is (red_then_writes): the run says nothing of the files as
// they now are, so the gate is not masked, it is not run since the last edit.
// It keeps its last run, and so its row, its exit_unmasked row (unmet: the
// status was hidden) and the done verdict (stale, still flagged); its
// gate_after_write row is unmet on the run and the write, as for every stale
// gate. Rows: 8164's shape, the write in the next turn; the write in the
// run's own turn; a shell write; a masked run after an earlier green.
// Controls, each still masked: the classic catch (edit, then the masked run);
// a masked run after another masked run and an edit; a masked run after a
// green and an edit; a write after the run outside the gate's project. Break:
// read masked before any write after the run, compare the write with the
// green run instead of the last, or read exit_unmasked from the state alone.
func TestF2d_AMaskedRunWithAWriteAfterItReadsStale(t *testing.T) {
	const a, b = f2Vetted + "/internal/gateinfer/gateinfer.go", f2Vetted + "/internal/gateinfer/gateinfer_test.go"
	for _, c := range []struct {
		name    string
		calls   []call
		turn2   int64 // the first call of turn 2; 0 for one turn
		run     int64 // the masked run, the gate's last
		write   int64 // the last covered write
		expired bool
	}{
		{"an edit in the next turn", []call{scWrite(1, a), f2MaskedVet(2), scWrite(3, b)}, 3, 2, 3, true},
		{"an edit in the same turn", []call{scWrite(1, a), f2MaskedVet(2), scWrite(3, b)}, 0, 2, 3, true},
		{"a shell write after it", []call{scWrite(1, a), f2MaskedVet(2), bashWrite(3, b)}, 0, 2, 3, true},
		{"after an earlier green", []call{f2cGreenVet(1), scWrite(2, a), f2MaskedVet(3), scWrite(4, b)}, 0, 3, 4, true},
		{"control: the classic catch", []call{scWrite(1, a), f2MaskedVet(2)}, 0, 2, 1, false},
		{"control: masked, an edit, masked", []call{f2MaskedVet(1), scWrite(2, a), f2MaskedVet(3)}, 0, 3, 2, false},
		{"control: green, an edit, masked", []call{f2cGreenVet(1), scWrite(2, a), f2MaskedVet(3)}, 0, 3, 2, false},
		{"control: a write after it outside the project", []call{scWrite(1, a), f2MaskedVet(2), bashWrite(3, "/tmp/rk/msg.txt")}, 0, 2, 1, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, c.calls...)
			var one, two []int64
			for _, k := range c.calls {
				scAt(t, r, k.seq, f2Vetted, f2Vetted)
				if c.turn2 > 0 && k.seq >= c.turn2 {
					two = append(two, k.seq)
				} else {
					one = append(one, k.seq)
				}
			}
			scPrompt(r, "p1", one...)
			prompt := "p1"
			if len(two) > 0 {
				scPrompt(r, "p2", two...)
				prompt = "p2"
			}
			v := scStop(r, prompt, f2Said, f2Vetted, nil)
			g := f2cVetRow(t, v)
			run, write := declSeq(r, int(c.run-1)), declSeq(r, int(c.write-1))
			if g.LastRun == nil || g.LastRun.Seq != run || !g.Required {
				t.Fatalf("go vet's last run %+v, required %t; want the masked run at %d, required", g.LastRun, g.Required, run)
			}
			state, reason, row, after := MaskedInconclusive, g.Reason, RowMasked, ObligationUnknown
			if c.expired {
				state, reason, row, after = Stale, f2cExpired, RowUnverified, ObligationUnmet
			}
			if g.State != state || g.Reason != reason || v.Summary.Row != row {
				t.Errorf("go vet %s %s, row %s; want %s %s, %s", g.State, g.Reason, v.Summary.Row, state, reason, row)
			}
			if !scFlagged(t, v) {
				t.Errorf("not flagged (row %s, claims %+v): a hidden status backs no done claim", v.Summary.Row, v.Scan.Claims)
			}
			wantRow(t, rowOf(t, v.Obligations, ExitUnmasked), ObligationUnmet, run)
			if c.expired {
				wantRow(t, rowOf(t, v.Obligations, GateAfterWrite), after, run, write)
			} else {
				wantRow(t, rowOf(t, v.Obligations, GateAfterWrite), after, run)
			}
		})
	}
}

// f2eRead is call 9325's shape: a shell line every stage of which reads or
// prints (cat, echo, cat, date), class read, may_write false.
func f2eRead(seq int64) call {
	return call{seq: seq, tool: "Bash", prog: "cat", class: shape.VerbRead, outcome: store.ExecOK}
}

// TestF2e_ACallThatOnlyReadsActsOnNothing (9376, 17:5x): turn 1 edited
// rv-se4-gi1w and ran vet there behind a pipe; turn 2 read two status files
// from home, which holds the worktrees (call 9325), and wrote in
// rv-se4-gi1w5-red. A call whose every stage reads acts on no project, so the
// turn's projects are its writing and executing calls' (turnProjects), vet in
// gi1w is not the turn's, and the done claim does not answer to it. The same
// for a Read in gi1w itself. Controls, each still the turn's: the read line
// may write (a redirect, a later stage that runs something); a line that
// executes, though it writes nothing; a record before may_write (schema 2,
// before project too, v5), which cannot say the line only reads, and so reads
// every gate as the turn's. Break: count a read call's project, or read
// may_write where the record does not carry it.
func TestF2e_ACallThatOnlyReadsActsOnNothing(t *testing.T) {
	mayWrite := f2eRead(3)
	mayWrite.mayWrite = true
	runs := call{seq: 3, tool: "Bash", prog: "python3", class: shape.VerbExecute, outcome: store.ExecOK}
	for _, c := range []struct {
		name, project string
		third         call
		schema        int // the third call's declaration schema; 0 is the fixture's
		turns         bool
	}{
		{"a shell read from the directory holding the worktree", "/wt", f2eRead(3), 0, false},
		{"a Read in the vetted worktree", f2Vetted, scRead(3), 0, false},
		{"control: the read line may write", "/wt", mayWrite, 0, true},
		{"control: a line that executes", "/wt", runs, 0, true},
		{"control: a record before may_write", "/wt", f2eRead(3), 2, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, scWrite(1, f2Vetted+"/internal/gateinfer/gateinfer.go"), f2MaskedVet(2), c.third,
				scWrite(4, f2Other+"/internal/verify/gi1_inferred_test.go"))
			scAt(t, r, 1, f2Vetted, f2Vetted)
			scAt(t, r, 2, f2Vetted, f2Vetted)
			scAt(t, r, 3, c.project, c.project)
			scAt(t, r, 4, f2Other, f2Other)
			if c.schema > 0 {
				for i := range r.Declarations {
					if d := &r.Declarations[i]; d.ToolUseID == callID(3) {
						d.SchemaVersion = c.schema
					}
				}
				f2Unkeyed(t, r, 3)
			}
			scPrompt(r, "p1", 1, 2)
			scPrompt(r, "p2", 3, 4)
			v := scStop(r, "p2", f2Said, f2Other, nil)
			if v.Snap.Recording != Complete {
				t.Fatalf("premise: recording %s, want complete", v.Snap.Recording)
			}
			g := f2Vet(t, v)
			if got := !g.offTurn; got != c.turns {
				t.Errorf("vet in rv-se4-gi1w is the turn's: %t; want %t", got, c.turns)
			}
			if got := scFlagged(t, v); got != c.turns {
				t.Errorf("flagged %t (row %s, claims %+v); want %t", got, v.Summary.Row, v.Scan.Claims, c.turns)
			}
		})
	}
}

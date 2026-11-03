package verify

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// BG-OUTCOME phase 2a on card F2 (the manager's ruling of 11-03 09:3x): the
// done window opens only at an edit a gate of the turn covers, the last
// covered write in the gate's project. A gate no call of the turn ran in the
// project of (markTurn) names no row at this Stop, so its edits open no
// window either: in the R67 replay, eng-4 9420 and six later Stops read
// "go vet ran before the last edit, but the pipe to head hid its result (call
// 3076)" through a window opened at an edit in vet's worktree, in turns whose
// calls ran in another.

// TestBGO2e_AnOffTurnProjectsEditOpensNoWindow: turn p1 edits rv-se4-gi1w and
// runs vet there behind a pipe; the Stop's turn ran only in another worktree,
// wrote under /tmp and claims done. The only covered edit is vet's project's,
// and vet is not the turn's, so no window opens: the turn is the line. Where
// a gate of the turn covers an earlier edit, the window opens at that one and
// counts only its project's writes, never at the later edit in vet's.
// Control: a turn in the vetted worktree opens the window at its edit.
// Break: open the window at an edit any gate covers (gateRoots).
func TestBGO2e_AnOffTurnProjectsEditOpensNoWindow(t *testing.T) {
	t.Run("the only covered edit is an off-turn gate's", func(t *testing.T) {
		r := runOf(t, scWrite(1, f2Vetted+"/internal/gateinfer/gateinfer.go"), f2MaskedVet(2),
			bashWrite(3, "/tmp/rk-se4-w5/msg-w5.txt"), scRead(4))
		scAt(t, r, 1, f2Vetted, f2Vetted)
		scAt(t, r, 2, f2Vetted, f2Vetted)
		scAt(t, r, 3, f2Other, f2Other)
		scAt(t, r, 4, f2Other, f2Other)
		scPrompt(r, "p1", 1, 2)
		scPrompt(r, "p2", 3, 4)
		v := scStop(r, "p2", f2Said, f2Other, nil)
		f2Vet(t, v)
		if v.Done != nil {
			t.Errorf("done window %+v opened at an edit only an off-turn gate covers; want none, the turn's line", v.Done.Turn)
		}
	})
	t.Run("an on-turn gate's edit before an off-turn one", func(t *testing.T) {
		r := runOf(t, scWrite(1, f2Other+"/internal/verify/a.go"), scWrite(2, f2Other+"/internal/verify/b.go"),
			gateCall(3, "go-test", store.ExecFailed),
			scWrite(4, f2Vetted+"/internal/gateinfer/gateinfer.go"), f2MaskedVet(5), scRead(6))
		for s, p := range map[int64]string{1: f2Other, 2: f2Other, 3: f2Other, 4: f2Vetted, 5: f2Vetted, 6: f2Other} {
			scAt(t, r, s, p, p)
		}
		scPrompt(r, "p1", 1, 2, 3)
		scPrompt(r, "p2", 4, 5)
		scPrompt(r, "p3", 6)
		v := scStop(r, "p3", f2Said, f2Other, nil)
		f2Vet(t, v)
		if v.Done == nil || v.Done.Turn.FirstSeq != 1 || v.Done.Turn.Writes != 2 {
			t.Errorf("done window %+v; want it from seq 1, p1's, with its 2 writes", v.Done)
		}
	})
	t.Run("control: the turn in the vetted worktree", func(t *testing.T) {
		r := runOf(t, scWrite(1, f2Vetted+"/internal/gateinfer/gateinfer.go"), f2MaskedVet(2),
			bashWrite(3, "/tmp/rk-se4-w5/msg-w5.txt"), scRead(4))
		for s := int64(1); s <= 4; s++ {
			scAt(t, r, s, f2Vetted, f2Vetted)
		}
		scPrompt(r, "p1", 1, 2)
		scPrompt(r, "p2", 3, 4)
		v := scStop(r, "p2", f2Said, f2Vetted, nil)
		f2Vet(t, v)
		if v.Done == nil || v.Done.Turn.FirstSeq != 1 || v.Done.Turn.Writes != 1 {
			t.Errorf("done window %+v; want it from seq 1, vet's project's edit, with 1 write", v.Done)
		}
	})
}

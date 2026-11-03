package verify

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Card STALE-COVER1: a gate is stale only where a file it covers was written
// after its last pass in this session; covered, v1, is inside the gate's
// project root, the repository PROJECT-RESOLVE1 keys a call by. The final
// messages are the L1 replay's turns word for word where one has the shape
// (l1/g-detail.txt names each); the paths are theirs under fixture roots.

// scRead is a Read the session makes in its repository: a call that gives
// the run its project root and writes nothing.
func scRead(seq int64) call {
	return call{seq: seq, tool: "Read", class: shape.VerbRead, outcome: store.ExecOK}
}

// scWrite is a Write of path, a file tool's absolute path.
func scWrite(seq int64, path string) call {
	return call{seq: seq, tool: "Write", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{path}}
}

// scAt sets call seq's declaration to have run in cwd with project, "" for a
// project the hook recorded none for (PROJECT-RESOLVE1: a directory under no
// .git, outside the cwd).
func scAt(t *testing.T, r *store.Run, seq int64, project, cwd string) {
	t.Helper()
	for i := range r.Declarations {
		if d := &r.Declarations[i]; d.ToolUseID == callID(seq) {
			d.Project, d.CWD = nil, ptr(cwd)
			if project != "" {
				d.Project = ptr(project)
			}
			return
		}
	}
	t.Fatalf("scAt: no declaration t%d", seq)
}

// scPrompt puts the declarations of calls seqs in prompt id.
func scPrompt(r *store.Run, id string, seqs ...int64) {
	for i := range r.Declarations {
		for _, s := range seqs {
			if r.Declarations[i].ToolUseID == callID(s) {
				r.Declarations[i].PromptID = ptr(id)
			}
		}
	}
}

// scStop is the Stop view at r's last seq for prompt, whose final message is
// said, under dgKey with no index and inf inferred (nil for none), its root
// and project key the cwd's key, as the Stop hands them (recap_stop.go).
func scStop(r *store.Run, prompt, said, key string, inf *InferredGate) StopView {
	pt := Point{Kind: PointStop, Seq: lastSeqOf(r), SessionID: "s1", PromptID: ptr(prompt)}
	return EvaluateStop(r, nil, nil, pt, time.Time{}, StopInput{PromptID: prompt, Key: dgKey, ProjectKey: key,
		Root: key, Inferred: inf, Head: []byte(said)})
}

// scFlagged is what recap.DoneFlag fires on without expected evidence: a
// strict claim whose verdict is a state other than green, on a row that is
// not verified. The message holds a strict done claim, or the test proves
// nothing.
func scFlagged(t *testing.T, v StopView) bool {
	t.Helper()
	if !slices.ContainsFunc(v.Scan.Claims, func(c ClaimVerdict) bool { return c.Strict && c.Class == ClaimDone }) {
		t.Fatalf("premise: the final message holds no strict done claim: %+v", v.Scan.Claims)
	}
	if v.Summary.Row == RowVerified {
		return false
	}
	return slices.ContainsFunc(v.Scan.Claims, func(c ClaimVerdict) bool {
		return c.Strict && Rank(State(c.Verdict)) >= 0 && c.Verdict != Verdict(Green)
	})
}

// scLearned is the view's learned row; the test runs no inferred gate.
func scLearned(t *testing.T, v StopView) GateState {
	t.Helper()
	g := one(t, v.Snap.Gates)
	if g.Source != sourceLearned {
		t.Fatalf("premise: the row is %s, want the learned family", g.Source)
	}
	return g
}

// TestSC1a_ANoteOutsideEveryRepositoryOwesNoTest (fixture 1): eng-4's turn
// 1988 wrote one verdict file in the evidence tree, outside every
// repository, and said "QA-1's run is complete." The hook records that Write
// with no project, so go test covers nothing the turn wrote: the claim
// answers to no gate and the row counts no edit. Control: the same file in
// the repository is an edit go test covers and never ran after, and the
// claim is flagged. Break: count a write against a gate that does not cover
// it, or read a gate that never ran and has nothing to verify as owed.
func TestSC1a_ANoteOutsideEveryRepositoryOwesNoTest(t *testing.T) {
	const said = "QA-1's run is complete."
	for _, c := range []struct {
		name, path, project string
		flagged             bool
		writes              int
	}{
		{"outside every repository", "/ev/verdicts/PR-170.txt", "", false, 0},
		{"control: in the repository", "/repo/verdicts/PR-170.txt", "/repo", true, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, scRead(1), scWrite(2, c.path))
			ranIn(r, "/repo", "")
			scAt(t, r, 2, c.project, "/repo")
			v := scStop(inPrompt(r, "p1"), "p1", said, "/repo", goTestAll)
			if got := scFlagged(t, v); got != c.flagged || v.Turn.Writes != c.writes {
				t.Errorf("flagged %t, turn writes %d (row %s, gates %+v); want %t, %d", got, v.Turn.Writes, v.Summary.Row, v.Snap.Gates, c.flagged, c.writes)
			}
		})
	}
}

// TestSC1b_AScriptUnderTmpAfterAGreenLeavesItGreen (fixture 2): the
// manager's turn 3007 wrote /tmp/rk-mgr/waitmerge.sh by a shell redirect
// after go test passed, and said "#155 is ready." The line ran in the
// repository, which is its call's project, and the path it names is outside
// it: go test stays green and the claim is backed. Control: the same script
// under the repository is a covered write after the pass. Break: read a
// shell write's place from its call's project alone, not its path.
func TestSC1b_AScriptUnderTmpAfterAGreenLeavesItGreen(t *testing.T) {
	const said = "#155 is ready."
	for _, c := range []struct {
		name, path string
		flagged    bool
		state      State
	}{
		{"under /tmp", "/tmp/rk-mgr/waitmerge.sh", false, Green},
		{"control: in the repository", "/repo/scripts/waitmerge.sh", true, Stale},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, gateCall(1, "go-test", store.ExecOK), bashWrite(2, c.path))
			named(t, r, 1, "go test ./...")
			ranIn(r, "/repo", "")
			scAt(t, r, 1, "/repo", "/repo")
			scAt(t, r, 2, "/repo", "/repo")
			v := scStop(inPrompt(r, "p1"), "p1", said, "/repo", goTestAll)
			g := one(t, v.Snap.Gates)
			if got := scFlagged(t, v); got != c.flagged || g.State != c.state {
				t.Errorf("flagged %t, %s %s %s; want %t, %s", got, g.Gate, g.State, g.Reason, c.flagged, c.state)
			}
		})
	}
}

// TestSC1c_AWriteInAnotherWorktreeLeavesThisOnesTestGreen (fixture 3): eng-4's
// turn 4282 ran go test green in one worktree, wrote a scratch test in
// another and said "The F1 fix for STEER-DISCLOSE-YES1 is done and handed
// back as a third commit, 31239230." The hook keys the Write by its file's
// repository, the other worktree, so the first worktree's go test covers
// nothing written after its pass. Control: the same file in the first
// worktree is a covered write after the pass. A shell line in the first
// worktree naming the other by "../" is not placed in the first. Break: take
// a learned gate's root from anything but its last run's project, or read a
// path that climbs out of its call's project as inside it.
func TestSC1c_AWriteInAnotherWorktreeLeavesThisOnesTestGreen(t *testing.T) {
	const said = "The F1 fix for STEER-DISCLOSE-YES1 is done and handed back as a third commit, 31239230."
	for _, c := range []struct {
		name, path, project string
		flagged             bool
		state               State
	}{
		{"another worktree", "/wt/rv-se4-189/test/zz_scratch_review_test.go", "/wt/rv-se4-189", false, Green},
		{"another worktree by ../", "../rv-se4-189/test/zz_scratch_review_test.go", "/wt/rv-se4-sdy", false, Green},
		{"control: this worktree", "/wt/rv-se4-sdy/test/zz_scratch_review_test.go", "/wt/rv-se4-sdy", true, Stale},
	} {
		t.Run(c.name, func(t *testing.T) {
			second := scWrite(2, c.path)
			if !filepath.IsAbs(c.path) {
				second = bashWrite(2, c.path)
			}
			r := runOf(t, gateCall(1, "go-test", store.ExecOK), second)
			scAt(t, r, 1, "/wt/rv-se4-sdy", "/wt/rv-se4-sdy")
			scAt(t, r, 2, c.project, "/wt/rv-se4-sdy")
			v := scStop(inPrompt(r, "p1"), "p1", said, "/wt/rv-se4-sdy", nil)
			g := scLearned(t, v)
			if got := scFlagged(t, v); got != c.flagged || g.State != c.state {
				t.Errorf("flagged %t, %s %s; want %t, %s", got, g.State, g.Reason, c.flagged, c.state)
			}
		})
	}
}

// TestSC1d_ASessionAboveItsWorktreesCountsTheWorktreesFiles: a session whose
// cwd is the directory above its worktrees, under no .git (the lanes' shape,
// eng-4's turn 3082: rv-se4/internal/regfile and the evidence tree from one
// cwd), keys a shell write by that directory. go test, run with a leading cd
// into the worktree, covers a file the line names under the worktree and not
// one in the evidence tree beside it. Break: read a relative path as covered
// only where its call's project is the gate's.
func TestSC1d_ASessionAboveItsWorktreesCountsTheWorktreesFiles(t *testing.T) {
	for _, c := range []struct {
		name, path string
		state      State
	}{
		{"the evidence tree", "docs-out/plans/evidence-2026-10-31/verdicts/LEARN-DX1.txt", Green},
		{"a file of the worktree", "rv-se4/internal/regfile/regfile_test.go", Stale},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, gateCall(1, "go-test", store.ExecOK), bashWrite(2, c.path))
			scAt(t, r, 1, "/wt/rv-se4", "/wt")
			scAt(t, r, 2, "/wt", "/wt")
			if g := scLearned(t, scStop(inPrompt(r, "p1"), "p1", "The fix is done.", "/wt", nil)); g.State != c.state {
				t.Errorf("%s %s, want %s", g.State, g.Reason, c.state)
			}
		})
	}
}

// TestSC1e_ARelayTurnAfterAGreenSaysNothing (fixture 4): eng-2's turn 5657
// wrote nothing and ran nothing, and said "The last job finished green and
// the result is with the manager." The turn before edited a file, ran go
// test green and then wrote its handoff file outside the repository: go test
// covers nothing written after its pass, so the claim is backed. Control: a
// covered edit after the pass in the turn before is owed still, and the
// relay turn's claim is flagged, the one catch the card keeps (fixture 5's
// shape over a green). Break: count the handoff file against go test.
func TestSC1e_ARelayTurnAfterAGreenSaysNothing(t *testing.T) {
	const said = "The last job finished green and the result is with the manager."
	for _, c := range []struct {
		name, path, project string
		flagged             bool
	}{
		{"a handoff file outside", "/ev/se2/HANDOFF-1820.txt", "", false},
		{"control: an edit after the pass", "/repo/internal/seed/class.go", "/repo", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, edit(1, "/repo/internal/seed/seed.go"), gateCall(2, "go-test", store.ExecOK), scWrite(3, c.path), scRead(4))
			named(t, r, 2, "go test ./...")
			ranIn(r, "/repo", "")
			for _, s := range []int64{1, 2, 4} {
				scAt(t, r, s, "/repo", "/repo")
			}
			scAt(t, r, 3, c.project, "/repo")
			scPrompt(r, "p1", 1, 2, 3)
			scPrompt(r, "p2", 4)
			v := scStop(r, "p2", said, "/repo", goTestAll)
			if got := scFlagged(t, v); got != c.flagged || v.Summary.Row != RowNone {
				t.Errorf("flagged %t on row %s (gates %+v); want %t on none", got, v.Summary.Row, v.Snap.Gates, c.flagged)
			}
		})
	}
}

// TestSC1f_DoneAboutAnEarlierTurnsEditStillFires (fixture 5): a turn edits a
// file go test covers and runs nothing; the next turn writes nothing and
// says it is done. go test never ran after a covered write in the session,
// so the claim is flagged, on either side of the card: the catch it keeps.
// A write whose project the record withholds is covered by every gate, as
// before the card (fail closed). Break: scope the never-run row to the turn,
// or drop a write the record cannot place.
func TestSC1f_DoneAboutAnEarlierTurnsEditStillFires(t *testing.T) {
	for _, c := range []struct {
		name, path, project string
		withheld            bool
	}{
		{"a covered edit", "/repo/internal/a.go", "/repo", false},
		{"a write whose project is withheld", "/ev/notes.txt", "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, scWrite(1, c.path), scRead(2))
			ranIn(r, "/repo", "")
			scAt(t, r, 1, c.project, "/repo")
			scAt(t, r, 2, "/repo", "/repo")
			if c.withheld {
				r.Declarations[0].ProjectWithheld = ptr("oversize")
			}
			scPrompt(r, "p1", 1)
			scPrompt(r, "p2", 2)
			v := scStop(r, "p2", "The fix is done.", "/repo", goTestAll)
			g, _ := gateNamed(v.Snap.Gates, InferredGateID)
			if !scFlagged(t, v) || g.State != Absent || g.Reason != NeverRunAfterWrite || g.WritesSinceGreen != 1 {
				t.Errorf("flagged %t, %s %s with %d writes; want flagged, absent never_run_after_write with 1", scFlagged(t, v), g.State, g.Reason, g.WritesSinceGreen)
			}
		})
	}
}

// TestSC1g_ACoveredWriteAfterThePassFires (fixture 6): an edit, go test
// green, a second edit, then "The fix is done.": go test is stale by the
// covered write after its pass, the row counts both edits, and the claim is
// flagged, as before the card. Break: count nothing after a pass.
func TestSC1g_ACoveredWriteAfterThePassFires(t *testing.T) {
	r := runOf(t, edit(1, "/repo/internal/a.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "/repo/internal/b.go"))
	named(t, r, 2, "go test ./...")
	ranIn(r, "/repo", "")
	v := scStop(inPrompt(r, "p1"), "p1", "The fix is done.", "/repo", goTestAll)
	g := one(t, v.Snap.Gates)
	if !scFlagged(t, v) || g.State != Stale || g.WritesSinceGreen != 1 || v.Turn.Writes != 2 {
		t.Errorf("flagged %t, %s %s with %d writes, turn writes %d; want flagged, stale with 1, 2", scFlagged(t, v), g.State, g.Reason, g.WritesSinceGreen, v.Turn.Writes)
	}
}

// TestSC1h_TheNudgeWaitsForACoveredWrite: under an index that requires the
// family, a note written outside every repository after the green raises no
// stale-evidence nudge; the first covered edit after it raises the one
// nudge. Break: let the note be the write that left the green behind.
func TestSC1h_TheNudgeWaitsForACoveredWrite(t *testing.T) {
	r := snRun(t, gateCall(2, "rt", store.ExecOK), scWrite(3, "/ev/HANDOFF.txt"), edit(4, "internal/a.go"))
	scAt(t, r, 3, "", "/repo")
	idx := snIndex(2, "r:rt@d:repo")
	if got := staleFindings(staleAt(t, r, 3, idx)); len(got) != 0 {
		t.Errorf("the note outside: %d stale-evidence findings, want none", len(got))
	}
	if got := staleFindings(staleAt(t, r, 4, idx)); len(got) != 1 {
		t.Errorf("the first covered edit: %d stale-evidence findings, want 1", len(got))
	}
}

// TestSC1i_ANoteBetweenARedAndAGreenIsNoFix: the family went red, the session
// wrote a note outside every repository, then it went green: no covered write
// came between, so the green is a pass by re-run, unmet, as with no write at
// all. Break: count the note as the fix.
func TestSC1i_ANoteBetweenARedAndAGreenIsNoFix(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecFailed), scWrite(2, "/ev/notes.txt"), gateCall(3, "go-test", store.ExecOK))
	ranIn(r, "/repo", "")
	scAt(t, r, 2, "", "/repo")
	wantRow(t, rowOf(t, obAt(r, 6), WatchedRedThenGreen), ObligationUnmet, declSeq(r, 0), declSeq(r, 2))
}

// TestSC1j_AMovedFileTheGateDoesNotCoverLeavesItsTreeBound: a note outside
// every repository, written before the green and changed since by no call
// of the session, is not in the green run's tree: the gate stays green.
// Control: a covered file moved the same way is a foreign edit. Break: bind
// the run to every relevant file written before it.
func TestSC1j_AMovedFileTheGateDoesNotCoverLeavesItsTreeBound(t *testing.T) {
	for _, c := range []struct {
		name, path, project string
		state               State
	}{
		{"a note outside", "/ev/notes.txt", "", Green},
		{"control: a covered file", "/repo/pkg/b.go", "/repo", Stale},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, hashedEdit(1, "/repo/pkg/a.go", shaA), hashedEdit(2, c.path, shaA), gateCall(3, "go-test", store.ExecOK))
			ranIn(r, "/repo", "")
			scAt(t, r, 2, c.project, "/repo")
			rel := c.path
			if c.project != "" {
				rel = "pkg/b.go"
			}
			_, g, _ := treeAt(t, r, map[string]*string{"pkg/a.go": &shaA, rel: &shaC})
			if g.State != c.state {
				t.Errorf("%s %s, want %s", g.State, g.Reason, c.state)
			}
		})
	}
}

// TestSC1k_ARecordThatCannotPlaceAGateCountsEveryWrite: fail closed where the
// record does not say what a gate covers. A go test the hook placed in no
// project (a leading cd into a directory under no .git, outside the cwd)
// covers every write, so a note outside after its pass leaves it stale and
// the turn counts the note; an incomplete record counts every write the turn
// made, and its row is unknown, never none (D80). Break: drop a gate with no
// root from the turn's roots, or count an incomplete turn's writes by root.
func TestSC1k_ARecordThatCannotPlaceAGateCountsEveryWrite(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecOK), scWrite(2, "/ev/notes.txt"), scRead(3))
	scAt(t, r, 1, "", "/repo")
	scAt(t, r, 2, "", "/repo")
	scAt(t, r, 3, "/repo", "/repo")
	v := scStop(inPrompt(r, "p1"), "p1", "The fix is done.", "/repo", nil)
	if g := scLearned(t, v); g.State != Stale || v.Turn.Writes != 1 {
		t.Errorf("a gate placed in no project: %s %s, turn writes %d; want stale, 1", g.State, g.Reason, v.Turn.Writes)
	}

	r = runOf(t, scRead(1), scWrite(2, "/ev/notes.txt"))
	ranIn(r, "/repo", "")
	scAt(t, r, 2, "", "/repo")
	r.Skipped = 1
	v = scStop(inPrompt(r, "p1"), "p1", "The fix is done.", "/repo", goTestAll)
	if v.Turn.Writes != 1 || v.Summary.Row != RowUnknown {
		t.Errorf("an incomplete record: turn writes %d, row %s; want 1, unknown", v.Turn.Writes, v.Summary.Row)
	}
}

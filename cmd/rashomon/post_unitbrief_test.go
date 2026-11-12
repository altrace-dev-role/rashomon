package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The post read of the unit brief (W-M2.3 hook half, card m2-unitbrief-hook1
// D4 to D6) over planHookProject: its index holds qwertyui/zorkheld.go under
// unit:go:qwertyui at commit c1, in repository planRID.

const ubPostTitle = "a reload dropped rows"

// ubPostSession is a consented plan session whose store keeps the session's
// content. The marker is written by hand: nothing in this tree writes it
// until W-M2.11 step 1d.
func ubPostSession(t *testing.T, scope string) *planSession {
	t.Helper()
	s := newPlanSession(t, scope)
	if err := os.WriteFile(filepath.Join(s.root, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return s
}

// ubPostUnit is the fixture file's unit, as the prompt side names it.
var ubPostUnit = graph.UnitID("go", "qwertyui")

// writeBrief stores ubBrief, rebound at rev, as the prompt side stores the
// unit's brief for this session, and returns the file's bytes.
func (s *planSession) writeBrief(rev string) []byte {
	s.t.Helper()
	b := engine.Rebind(graph.RevCommit(rev), ubBrief(s.t).Lines)
	data, err := encodeUnitBrief(b, graph.GlobalID(planRID, ubPostUnit), graph.GlobalID(planRID, b.FirstKey()), graph.Algorithmic, "prompt-1")
	if err != nil {
		s.t.Fatal(err)
	}
	path := filepath.Join(graph.UnitBriefDir(s.root, benchSessionID), graph.UnitBriefFileName(s.project, ubPostUnit))
	if err := writeUnitBrief(path, data); err != nil {
		s.t.Fatal(err)
	}
	return data
}

// edit runs the real post entry on an Edit of the fixture's file.
func (s *planSession) edit(useID string) string {
	return s.call(true, "Edit", useID, map[string]any{"file_path": filepath.Join(s.project, filepath.FromSlash(planHookHeld)),
		"old_string": "package x", "new_string": "package x // " + useID})
}

// briefLines is the session's task-preflight steer lines.
func (s *planSession) briefLines() []steer.Line {
	s.t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		s.t.Fatal(err)
	}
	raw, _, _ := st.ReadSteer(benchSessionID)
	var out []steer.Line
	for _, b := range raw {
		var l steer.Line
		if err := json.Unmarshal(b, &l); err != nil {
			s.t.Fatal(err)
		}
		if l.Pattern == string(pattern.TaskPreflight) {
			out = append(out, l)
		}
	}
	return out
}

// execSeq is the seq the execution of the call useID landed at.
func (s *planSession) execSeq(useID string) int64 {
	s.t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		s.t.Fatal(err)
	}
	run, err := st.ReadRun(benchSessionID)
	if err != nil {
		s.t.Fatal(err)
	}
	for _, e := range run.Executions {
		if e.ToolUseID == useID && e.Seq != nil {
			return *e.Seq
		}
	}
	s.t.Fatalf("no execution of %s", useID)
	return 0
}

// TestPost_UnitBriefDeliversOnceAtTheFirstEditInTheUnit (D4, D5, D6): the
// first landed edit in a unit whose brief the session holds prints the
// brief and leaves one task-preflight line keyed "unitbrief:" + the unit's
// global id + "@" + the file's digest, delivered, after the edit's own
// execution seq, its rev slot the file's commit; the second edit in the
// unit prints nothing and adds no line. At either record scope: the note is
// the same, only the record form differs.
func TestPost_UnitBriefDeliversOnceAtTheFirstEditInTheUnit(t *testing.T) {
	for _, scope := range []string{store.RecordScopeV4, store.RecordScopeV5} {
		t.Run(scope, func(t *testing.T) {
			s := ubPostSession(t, scope)
			digest, err := unitBriefDigest(s.writeBrief("c1"))
			if err != nil {
				t.Fatal(err)
			}
			out := s.edit("toolu_ub_1")
			if !strings.Contains(out, ubPostTitle) {
				t.Fatalf("the first edit in the unit printed %q, want the unit's brief", out)
			}
			lines := s.briefLines()
			if len(lines) != 1 {
				t.Fatalf("%d task-preflight lines, want 1", len(lines))
			}
			l := lines[0]
			if want := "unitbrief:" + string(graph.GlobalID(planRID, ubPostUnit)) + "@" + digest; l.Key != want {
				t.Errorf("key %q, want %q", l.Key, want)
			}
			if l.State != steer.LineStateDelivered || l.Event != string(steer.EventPostToolUse) {
				t.Errorf("state %q event %q, want delivered at PostToolUse", l.State, l.Event)
			}
			if seq := s.execSeq("toolu_ub_1"); l.AfterSeq != seq {
				t.Errorf("after_seq %d, want the edit's execution seq %d", l.AfterSeq, seq)
			}
			if rev := string(l.Slots["rev"]); !strings.Contains(rev, "c1") {
				t.Errorf("rev slot %s, want the file's commit c1", rev)
			}
			if out := s.edit("toolu_ub_2"); strings.Contains(out, ubPostTitle) || len(s.briefLines()) != 1 {
				t.Errorf("the second edit printed %q and left %d lines, want nothing and 1", out, len(s.briefLines()))
			}
		})
	}
}

// TestPost_UnitBriefStaleRevIsNotDelivered (condition (ii)): a brief stored
// at a commit other than the index's is not said.
func TestPost_UnitBriefStaleRevIsNotDelivered(t *testing.T) {
	s := ubPostSession(t, store.RecordScopeV5)
	s.writeBrief("c0")
	if out := s.edit("toolu_ub_1"); strings.Contains(out, ubPostTitle) || len(s.briefLines()) != 0 {
		t.Errorf("a brief at c0 under an index at c1 printed %q and left %d lines, want none", out, len(s.briefLines()))
	}
}

// TestPost_UnitBriefNeedsTheKeptContentMarker (D4): the same brief in a
// store without the session-content marker is not read. Witness: the
// marker written, the next edit delivers it.
func TestPost_UnitBriefNeedsTheKeptContentMarker(t *testing.T) {
	s := newPlanSession(t, store.RecordScopeV5)
	s.writeBrief("c1")
	if out := s.edit("toolu_ub_1"); strings.Contains(out, ubPostTitle) || len(s.briefLines()) != 0 {
		t.Fatalf("without the marker the edit printed %q and left %d lines, want none", out, len(s.briefLines()))
	}
	// By hand, as ubPostSession: no writer until W-M2.11 step 1d.
	if err := os.WriteFile(filepath.Join(s.root, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := s.edit("toolu_ub_2"); !strings.Contains(out, ubPostTitle) {
		t.Errorf("witness: with the marker the edit printed %q, want the brief", out)
	}
}

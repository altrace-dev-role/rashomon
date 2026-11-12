package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/intent"
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

// briefPath is where the prompt side stores the fixture unit's brief for
// this session.
func (s *planSession) briefPath() string {
	return filepath.Join(graph.UnitBriefDir(s.root, benchSessionID), graph.UnitBriefFileName(s.project, ubPostUnit))
}

// writeBrief stores ubBrief, rebound at rev, as the prompt side stores the
// unit's brief for this session, and returns the file's bytes.
func (s *planSession) writeBrief(rev string) []byte {
	return s.writeBriefFor(graph.GlobalID(planRID, ubPostUnit), rev)
}

// writeBriefFor is writeBrief with the brief naming unit as its own, which
// may be another unit than the one whose name the file is under.
func (s *planSession) writeBriefFor(unit graph.ID, rev string) []byte {
	s.t.Helper()
	b := engine.Rebind(graph.RevCommit(rev), ubBrief(s.t).Lines)
	data, err := encodeUnitBrief(b, unit, graph.GlobalID(planRID, b.FirstKey()), graph.Algorithmic, "prompt-1")
	if err != nil {
		s.t.Fatal(err)
	}
	if err := writeUnitBrief(s.briefPath(), data); err != nil {
		s.t.Fatal(err)
	}
	return data
}

// misses is the session's unitbrief_miss records.
func (s *planSession) misses() []store.UnitBriefMiss {
	s.t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		s.t.Fatal(err)
	}
	run, err := st.ReadRun(benchSessionID)
	if err != nil {
		s.t.Fatal(err)
	}
	return run.UnitBriefMisses
}

// breakIndexBody keeps the project index's header line and replaces its
// records with a line no loader reads, so a read that loads the index fails
// where one that reads its header alone does not. Witness: graph.LoadFile
// refuses the file.
func (s *planSession) breakIndexBody() {
	s.t.Helper()
	path := filepath.Join(s.project, filepath.FromSlash(graph.GraphFile))
	data, err := os.ReadFile(path)
	if err != nil {
		s.t.Fatal(err)
	}
	i := bytes.IndexByte(data, '\n')
	if i < 0 {
		s.t.Fatalf("%s has no header line", path)
	}
	if err := os.WriteFile(path, append(data[:i+1:i+1], "not a record\n"...), 0o644); err != nil {
		s.t.Fatal(err)
	}
	if _, _, err := graph.LoadFile(s.project); err == nil {
		s.t.Fatal("witness: graph.LoadFile read an index whose body is no record")
	}
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

// TestUnitBrief_MissIsCountedNeverLoaded (D7): a landed edit whose unit's
// brief cannot be said writes one unitbrief_miss, critical false, its unit
// the unit's global id as a keyed digest and its landed_seq the edit's
// execution seq: absent (no file), refused (a torn file, or the brief of
// another unit under this unit's name) and stale (a file at c0 under an
// index at c1). Not a miss: a store without the session-content marker, the
// delivered first edit, and the given unit's next edit. Every case runs on
// an index whose body no loader reads, so the read takes the index's
// header and loads no record; the delivered case is the witness that the
// brief is still said through it.
func TestUnitBrief_MissIsCountedNeverLoaded(t *testing.T) {
	other := graph.GlobalID(planRID, graph.UnitID("go", "elsewhere"))
	cases := []struct {
		name, reason string
		setup        func(s *planSession)
	}{
		{"absent", store.UnitBriefAbsent, func(*planSession) {}},
		{"torn", store.UnitBriefRefused, func(s *planSession) {
			data := s.writeBrief("c1")
			if err := os.WriteFile(s.briefPath(), data[:len(data)-1], 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"another unit", store.UnitBriefRefused, func(s *planSession) { s.writeBriefFor(other, "c1") }},
		{"stale", store.UnitBriefStale, func(s *planSession) { s.writeBrief("c0") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := ubPostSession(t, store.RecordScopeV5)
			s.breakIndexBody()
			c.setup(s)
			if out := s.edit("toolu_ub_1"); strings.Contains(out, ubPostTitle) {
				t.Errorf("printed %q, want no brief", out)
			}
			m := s.misses()
			if len(m) != 1 {
				t.Fatalf("%d unitbrief_miss records, want 1", len(m))
			}
			st, err := store.OpenExisting(s.root)
			if err != nil {
				t.Fatal(err)
			}
			unit := intent.PathDigest(st.Key(), string(graph.GlobalID(planRID, ubPostUnit)))
			if m[0].Reason != c.reason || m[0].Unit != unit || m[0].Critical {
				t.Errorf("miss reason %q unit %q critical %v, want %q, %q, false", m[0].Reason, m[0].Unit, m[0].Critical, c.reason, unit)
			}
			if seq := s.execSeq("toolu_ub_1"); m[0].LandedSeq != seq {
				t.Errorf("landed_seq %d, want the edit's execution seq %d", m[0].LandedSeq, seq)
			}
		})
	}
	t.Run("no marker", func(t *testing.T) {
		s := newPlanSession(t, store.RecordScopeV5)
		s.breakIndexBody()
		s.edit("toolu_ub_1")
		if n := len(s.misses()); n != 0 {
			t.Errorf("a store keeping no session content wrote %d misses, want 0", n)
		}
	})
	t.Run("given", func(t *testing.T) {
		s := ubPostSession(t, store.RecordScopeV5)
		s.breakIndexBody()
		s.writeBrief("c1")
		if out := s.edit("toolu_ub_1"); !strings.Contains(out, ubPostTitle) {
			t.Fatalf("witness: the first edit printed %q, want the brief through a header-only read", out)
		}
		s.edit("toolu_ub_2")
		if n := len(s.misses()); n != 0 {
			t.Errorf("the delivered edit and the given unit's next edit wrote %d misses, want 0", n)
		}
	})
}

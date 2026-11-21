package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The post read of the unit brief for a file the py reader claims (card
// PY-U): the path post_unitbrief_test.go takes for a .go file, from the
// overlay writer's gate (keptGoEdit) through Post.LandedGoEdit and its Unit
// to unitBriefJoin, the edit's unit being the one the reader that claims
// the file places it in.

const (
	// ubPyDir is the py unit the project's configuration declares.
	ubPyDir = "poiuytre"
	// ubPyHeld is a file of that unit, in the index and on disk.
	ubPyHeld = "poiuytre/zorkheld.py"
	// ubPyLoose is a .py file on disk under no unit the configuration
	// declares.
	ubPyLoose = "mnbvcxzl/loose.py"
	ubPyBody  = "x = 1\n"
)

// ubPyUnit is the unit pylite places ubPyHeld in, as the prompt side names
// it.
var ubPyUnit = graph.UnitID(pylite.Lang, ubPyDir)

// ubPySession is ubPostSession at record scope v5 over planHookProject with
// a py unit added: the configuration declares ubPyDir, the index holds
// ubPyHeld under it at the index's own commit, the header names producer as
// its py producer ("" for none), and both .py files are on disk.
func ubPySession(t *testing.T, producer string) *planSession {
	t.Helper()
	s := ubPostSession(t, store.RecordScopeV5)
	g, h, err := graph.LoadFile(s.project)
	if err != nil {
		t.Fatal(err)
	}
	if producer != "" {
		h.Producers = map[string]string{pylite.Lang: producer}
	}
	rev := graph.RevCommit(h.Rev.Commit)
	fid := graph.FileID(ubPyHeld)
	g.AssertNode(graph.Node{ID: fid, Kind: "file", Rev: rev})
	g.AssertEdge(graph.Edge{Src: ubPyUnit, Pred: graph.Contains, Dst: fid, Rev: rev})
	if err := graph.SaveFile(s.project, g, h); err != nil {
		t.Fatal(err)
	}
	for rel, body := range map[string]string{
		index.ConfigFile: `{"units":[{"path":"` + ubPyDir + `","mod_root":"` + ubPyDir + `","modules":["zorkheld"],"lang":"py"}]}`,
		ubPyHeld:         ubPyBody,
		ubPyLoose:        ubPyBody,
	} {
		p := filepath.Join(s.project, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// storePyBrief stores ubBrief, rebound at rev, as the prompt side stores
// the py unit's brief for this session.
func (s *planSession) storePyBrief(rev string) []byte {
	return s.storeBrief(ubPyUnit, graph.GlobalID(planRID, ubPyUnit), rev)
}

// editPy runs the real post entry on an Edit of the project's .py file rel
// whose old string is the file's first line.
func (s *planSession) editPy(useID, rel string) string {
	return s.call(true, "Edit", useID, map[string]any{"file_path": filepath.Join(s.project, filepath.FromSlash(rel)),
		"old_string": "x = 1", "new_string": "x = 1  # " + useID})
}

// TestPost_UnitBriefOfAPyUnitDeliversOnceAtTheFirstEdit (card PY-U): under
// an index whose header names this version of the py reader as its py
// producer, the first landed edit of a .py file in a unit whose brief the
// session holds prints the brief, naming the unit's directory, and leaves
// one task-preflight line keyed "unitbrief:" + the py unit's global id +
// "@" + the file's digest, delivered, after the edit's own execution seq;
// the second edit in the unit prints nothing and adds no line, and neither
// edit writes a unitbrief_miss. Break: name the edit's unit by any rule but
// the claiming reader's (the unit of the file's directory under another
// language's id, say), and the brief is absent under that name: no line
// and one miss.
func TestPost_UnitBriefOfAPyUnitDeliversOnceAtTheFirstEdit(t *testing.T) {
	s := ubPySession(t, pylite.Resolver{}.Producer())
	digest, err := unitBriefDigest(s.storePyBrief("c1"))
	if err != nil {
		t.Fatal(err)
	}
	out := s.editPy("toolu_ubpy_1", ubPyHeld)
	if !strings.Contains(out, ubPostTitle) || !strings.Contains(out, ubPyDir) {
		t.Fatalf("the first edit in the py unit printed %q, want the unit's brief naming %s", out, ubPyDir)
	}
	lines := s.briefLines()
	if len(lines) != 1 {
		t.Fatalf("%d task-preflight lines, want 1", len(lines))
	}
	l := lines[0]
	if want := "unitbrief:" + string(graph.GlobalID(planRID, ubPyUnit)) + "@" + digest; l.Key != want {
		t.Errorf("key %q, want %q", l.Key, want)
	}
	if l.State != steer.LineStateDelivered || l.Event != string(steer.EventPostToolUse) {
		t.Errorf("state %q event %q, want delivered at PostToolUse", l.State, l.Event)
	}
	if seq := s.execSeq("toolu_ubpy_1"); l.AfterSeq != seq {
		t.Errorf("after_seq %d, want the edit's execution seq %d", l.AfterSeq, seq)
	}
	if out := s.editPy("toolu_ubpy_2", ubPyHeld); strings.Contains(out, ubPostTitle) || len(s.briefLines()) != 1 {
		t.Errorf("the second edit printed %q and left %d lines, want nothing and 1", out, len(s.briefLines()))
	}
	if n := len(s.misses()); n != 0 {
		t.Errorf("the delivered edit and the given unit's next edit wrote %d misses, want 0", n)
	}
}

// TestUnitBrief_PyUnitMissIsCounted (card PY-U, D7 for a py unit): under the
// same index, a landed edit of a .py file whose unit's brief cannot be said
// writes one unitbrief_miss, critical false, its unit the py unit's global
// id as a keyed digest and its landed_seq the edit's execution seq: absent
// (no file) and stale (a file at c0 under an index at c1).
func TestUnitBrief_PyUnitMissIsCounted(t *testing.T) {
	for _, c := range []struct{ name, reason, rev string }{
		{"absent", store.UnitBriefAbsent, ""},
		{"stale", store.UnitBriefStale, "c0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := ubPySession(t, pylite.Resolver{}.Producer())
			if c.rev != "" {
				s.storePyBrief(c.rev)
			}
			if out := s.editPy("toolu_ubpy_1", ubPyHeld); strings.Contains(out, ubPostTitle) {
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
			unit := intent.PathDigest(st.Key(), string(graph.GlobalID(planRID, ubPyUnit)))
			if m[0].Reason != c.reason || m[0].Unit != unit || m[0].Critical {
				t.Errorf("miss reason %q unit %q critical %v, want %q, %q, false", m[0].Reason, m[0].Unit, m[0].Critical, c.reason, unit)
			}
			if seq := s.execSeq("toolu_ubpy_1"); m[0].LandedSeq != seq {
				t.Errorf("landed_seq %d, want the edit's execution seq %d", m[0].LandedSeq, seq)
			}
		})
	}
}

// TestUnitBrief_PyEditInNoIndexedUnitIsNeitherSaidNorCounted (card PY-U):
// with the py unit's brief on file at the index's commit, a landed edit of
// a .py file prints no brief, leaves no task-preflight line and writes no
// unitbrief_miss where the index header names no py producer, or one other
// than the string this version of the reader states (the overlay writer
// stops at the header, lang.Registry.Unindexed, and clears its gate), and
// where the file is in no unit the configuration declares (the reader
// places it in none). Witness: the same fixture with this version's
// producer and the unit's own file says the brief.
func TestUnitBrief_PyEditInNoIndexedUnitIsNeitherSaidNorCounted(t *testing.T) {
	const older = "pylite/2"
	if older == (pylite.Resolver{}).Producer() {
		t.Fatalf("premise: %s is this version's producer", older)
	}
	for _, c := range []struct {
		name, producer, rel string
		said                bool
	}{
		{"no py producer", "", ubPyHeld, false},
		{"an older py producer", older, ubPyHeld, false},
		{"in no configured unit", pylite.Resolver{}.Producer(), ubPyLoose, false},
		{"witness: this version, the unit's file", pylite.Resolver{}.Producer(), ubPyHeld, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := ubPySession(t, c.producer)
			s.storePyBrief("c1")
			out := s.editPy("toolu_ubpy_1", c.rel)
			said, lines, misses := strings.Contains(out, ubPostTitle), len(s.briefLines()), len(s.misses())
			if c.said {
				if !said || lines != 1 || misses != 0 {
					t.Errorf("printed %q, %d lines, %d misses; want the brief, 1 line, 0 misses", out, lines, misses)
				}
				return
			}
			if said || lines != 0 || misses != 0 {
				t.Errorf("printed %q, %d lines, %d misses; want no brief, no line, no miss", out, lines, misses)
			}
		})
	}
}

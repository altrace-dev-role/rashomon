package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path"
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
func (s *planSession) briefPath() string { return s.briefPathOf(ubPostUnit) }

// briefPathOf is where the prompt side stores unit's brief for this session.
func (s *planSession) briefPathOf(unit graph.ID) string {
	return filepath.Join(graph.UnitBriefDir(s.root, benchSessionID), graph.UnitBriefFileName(s.project, unit))
}

// writeBrief stores ubBrief, rebound at rev, as the prompt side stores the
// unit's brief for this session, and returns the file's bytes.
func (s *planSession) writeBrief(rev string) []byte {
	return s.writeBriefFor(graph.GlobalID(planRID, ubPostUnit), rev)
}

// writeBriefFor is writeBrief with the brief naming unit as its own, which
// may be another unit than the one whose name the file is under.
func (s *planSession) writeBriefFor(unit graph.ID, rev string) []byte {
	return s.storeBrief(ubPostUnit, unit, rev)
}

// storeBrief stores ubBrief, rebound at rev and naming owner as its unit,
// under the file name of the local unit under, and returns the file's bytes.
func (s *planSession) storeBrief(under, owner graph.ID, rev string) []byte {
	s.t.Helper()
	b := engine.Rebind(graph.RevCommit(rev), ubBrief(s.t).Lines)
	data, err := encodeUnitBrief(b, owner, graph.GlobalID(planRID, b.FirstKey()), graph.Algorithmic, "prompt-1")
	if err != nil {
		s.t.Fatal(err)
	}
	if err := writeUnitBrief(s.briefPathOf(under), data); err != nil {
		s.t.Fatal(err)
	}
	return data
}

// addUnit puts the file rel, under the unit of its directory, into the
// project's index at the index's own header, and writes it to disk, so a
// session holds a second unit to edit in. It returns the unit's local id.
func (s *planSession) addUnit(rel string) graph.ID {
	s.t.Helper()
	g, h, err := graph.LoadFile(s.project)
	if err != nil {
		s.t.Fatal(err)
	}
	rev := graph.RevCommit(h.Rev.Commit)
	fid, unit := graph.FileID(rel), graph.UnitID("go", path.Dir(rel))
	g.AssertNode(graph.Node{ID: fid, Kind: "file", Rev: rev})
	g.AssertEdge(graph.Edge{Src: unit, Pred: graph.Contains, Dst: fid, Rev: rev})
	if err := graph.SaveFile(s.project, g, h); err != nil {
		s.t.Fatal(err)
	}
	file := filepath.Join(s.project, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package x\n"), 0o644); err != nil {
		s.t.Fatal(err)
	}
	return unit
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
func (s *planSession) edit(useID string) string { return s.editFile(useID, planHookHeld) }

// editFile runs the real post entry on an Edit of the project's file rel.
func (s *planSession) editFile(useID, rel string) string {
	return s.call(true, "Edit", useID, map[string]any{"file_path": filepath.Join(s.project, filepath.FromSlash(rel)),
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

// TestPost_UnitBriefReadIsBounded (card R, the post side of
// TestUnitBrief_ReadIsBounded): the post reads a unit's file to the cap
// and no further. A file at the cap is said; one byte over is refused,
// one unitbrief_miss and no note. The file is the stored brief behind
// leading spaces, which the decoder skips and the digest does not see, so
// the cap is the one thing that refuses it. A torn file is
// TestUnitBrief_MissIsCountedNeverLoaded's torn case.
func TestPost_UnitBriefReadIsBounded(t *testing.T) {
	for _, c := range []struct {
		name string
		size int
		said bool
	}{
		{"at the cap", maxUnitBriefBytes, true},
		{"one over", maxUnitBriefBytes + 1, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := ubPostSession(t, store.RecordScopeV5)
			data := s.writeBrief("c1")
			if len(data) >= c.size {
				t.Fatalf("premise: the brief is %d B, not under %d", len(data), c.size)
			}
			padded := append(bytes.Repeat([]byte{' '}, c.size-len(data)), data...)
			if err := os.WriteFile(s.briefPath(), padded, 0o600); err != nil {
				t.Fatal(err)
			}
			if out := s.edit("toolu_ub_1"); strings.Contains(out, ubPostTitle) != c.said {
				t.Errorf("a %d B file printed %q, want the brief %v", c.size, out, c.said)
			}
			m := s.misses()
			if c.said && len(m) != 0 || !c.said && (len(m) != 1 || m[0].Reason != store.UnitBriefRefused) {
				t.Errorf("a %d B file left misses %+v, want refused %v", c.size, m, !c.said)
			}
		})
	}
}

// TestUnitBrief_WithheldBriefStaysForTheNextEdit (D6): with steering off
// the first edit in the unit gives no note, leaves no task-preflight line
// and no miss, and the unit's file stays; with steering back on the next
// edit gives the note.
func TestUnitBrief_WithheldBriefStaysForTheNextEdit(t *testing.T) {
	s := ubPostSession(t, store.RecordScopeV5)
	s.writeBrief("c1")
	cfg := filepath.Join(s.root, store.FileConfig)
	if err := os.WriteFile(cfg, []byte(`{"steer_user":"off"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := s.edit("toolu_ub_1"); strings.Contains(out, ubPostTitle) {
		t.Errorf("with steering off the edit printed %q, want no brief", out)
	}
	if n, m := len(s.briefLines()), len(s.misses()); n != 0 || m != 0 {
		t.Errorf("with steering off: %d task-preflight lines and %d misses, want 0 and 0", n, m)
	}
	if _, err := os.Stat(s.briefPath()); err != nil {
		t.Fatalf("the withheld brief's file: %v", err)
	}
	if err := os.Remove(cfg); err != nil {
		t.Fatal(err)
	}
	if out := s.edit("toolu_ub_2"); !strings.Contains(out, ubPostTitle) {
		t.Errorf("with steering on the next edit printed %q, want the brief", out)
	}
}

// ubSecondFile is a file of a second unit, unit:go:asdfghjk.
const ubSecondFile = "asdfghjk/other.go"

// TestUnitBrief_OncePerUnitPerSession (D6): the brief is said once per unit,
// and one unit's once does not stand for another's. Edits in two units the
// session holds briefs for, then in the first unit again, say each unit's
// brief at its first edit only: two delivered lines, each keyed by its own
// unit. The two files hold the same facts, so their keys differ by the unit
// alone.
func TestUnitBrief_OncePerUnitPerSession(t *testing.T) {
	s := ubPostSession(t, store.RecordScopeV5)
	second := s.addUnit(ubSecondFile)
	s.writeBrief("c1")
	s.storeBrief(second, graph.GlobalID(planRID, second), "c1")
	for _, c := range []struct {
		id, file string
		said     bool
	}{
		{"toolu_ub_1", planHookHeld, true},
		{"toolu_ub_2", ubSecondFile, true},
		{"toolu_ub_3", planHookHeld, false},
	} {
		if out := s.editFile(c.id, c.file); strings.Contains(out, ubPostTitle) != c.said {
			t.Errorf("the edit %s in %s printed %q, want the brief %v", c.id, c.file, out, c.said)
		}
	}
	lines := s.briefLines()
	if len(lines) != 2 {
		t.Fatalf("%d task-preflight lines, want 2", len(lines))
	}
	var digests []string
	for i, unit := range []graph.ID{ubPostUnit, second} {
		l := lines[i]
		rest, ok := strings.CutPrefix(l.Key, "unitbrief:"+string(graph.GlobalID(planRID, unit))+"@")
		if !ok || l.State != steer.LineStateDelivered {
			t.Errorf("line %d: key %q state %q, want delivered under %s", i+1, l.Key, l.State, unit)
		}
		digests = append(digests, rest)
	}
	if digests[0] != digests[1] {
		t.Errorf("premise: digests %v differ, so the keys differ by more than the unit", digests)
	}
}

// ubIndexTree is every entry under the project index's directory, by path,
// with its mode and, for a file, its bytes.
func ubIndexTree(t *testing.T, root string) map[string]string {
	t.Helper()
	dir := filepath.Dir(filepath.Join(root, filepath.FromSlash(graph.GraphFile)))
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out[rel] = info.Mode().String()
		if !d.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[rel] += " " + string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestUnitBrief_NothingUnderTheRepoIndex (card R, 15(e)): the precompute
// and the post read write nothing under the repository's index directory:
// it is the same entries, modes and bytes before and after each. Witnesses:
// the precompute stored a unit's brief, and the post said one.
func TestUnitBrief_NothingUnderTheRepoIndex(t *testing.T) {
	t.Run("precompute", func(t *testing.T) {
		dir := t.TempDir()
		graphProject(t, dir)
		cwd := filepath.Join(dir, "services", "auth")
		root := graph.FindIndex(cwd)
		consented := t.TempDir()
		if err := os.WriteFile(filepath.Join(consented, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		before := ubIndexTree(t, root)
		if _, ok := graphBrief(consented, cwd, "s1", "p1", []byte("make session expiry in sessions.py respect the org-level timeout override.")); !ok {
			t.Fatal("no brief")
		}
		if ents, _ := os.ReadDir(graph.UnitBriefDir(consented, "s1")); len(ents) == 0 {
			t.Error("witness: the precompute stored no unit brief")
		}
		if after := ubIndexTree(t, root); !maps.Equal(before, after) {
			t.Errorf("the index directory moved: %d entries before, %d after", len(before), len(after))
		}
	})
	t.Run("post read", func(t *testing.T) {
		s := ubPostSession(t, store.RecordScopeV5)
		s.writeBrief("c1")
		before := ubIndexTree(t, s.project)
		if out := s.edit("toolu_ub_1"); !strings.Contains(out, ubPostTitle) {
			t.Fatalf("witness: the edit printed %q, want the brief", out)
		}
		if after := ubIndexTree(t, s.project); !maps.Equal(before, after) {
			t.Errorf("the index directory moved: %d entries before, %d after", len(before), len(after))
		}
	})
}

// ubPostHead and ubPostRecordHead open the fixture unit's note and its
// record form (ruling 04:58 B), up to the head's line feed.
const (
	ubPostHead       = "rashomon record: an edit in unit 'qwertyui', noted once a session; the repository graph at rev c1 records for it:"
	ubPostRecordHead = "rashomon record: an edit in a unit on record, noted once a session; the repository graph at rev c1 records for it:"
)

// TestPost_UnitBriefLineFollowsTheRecordScope (D4): the note is the same at
// either record scope and only its steer line's form differs, as the
// prompt's graph brief's does (slot Record). Under v5 the line holds the
// brief's paths; under v4 each is a keyed digest, so no path the note
// printed is in the steer log. The lesson's title, a record the repository
// declared and never the request's words, is kept at both. The note's head
// names the unit and neither the request nor any count (ruling 04:58 B);
// the line keeps that head and the unit's slot under v5, and under v4 the
// record head and no unit slot.
func TestPost_UnitBriefLineFollowsTheRecordScope(t *testing.T) {
	paths := []string{"brief/brief.py", "store/loader.py"}
	for _, c := range []struct {
		scope string
		plain bool
	}{
		{store.RecordScopeV5, true},
		{store.RecordScopeV4, false},
	} {
		t.Run(c.scope, func(t *testing.T) {
			s := ubPostSession(t, c.scope)
			s.writeBrief("c1")
			out := s.edit("toolu_ub_1")
			for _, want := range append([]string{ubPostTitle}, paths...) {
				if !strings.Contains(out, want) {
					t.Fatalf("premise: the note %q does not print %q", out, want)
				}
			}
			if !strings.Contains(out, ubPostHead) || strings.Contains(out, "request") || strings.Contains(out, "node(s)") {
				t.Errorf("the note %q, want the unit's head %q and no request or count", out, ubPostHead)
			}
			if lines := s.briefLines(); len(lines) != 1 {
				t.Errorf("%d task-preflight lines, want 1", len(lines))
			} else {
				l, head := lines[0], ubPostRecordHead
				if c.plain {
					head = ubPostHead
				}
				if !strings.HasPrefix(l.ModelText, head) {
					t.Errorf("the line's model text %q, want the head %q", l.ModelText, head)
				}
				if u, ok := l.Slots["unit"]; ok != c.plain || ok && string(u) != `"qwertyui"` {
					t.Errorf("the line's unit slot %s (held %v), want held %v", u, ok, c.plain)
				}
			}
			st, err := store.OpenExisting(s.root)
			if err != nil {
				t.Fatal(err)
			}
			raw, _, err := st.ReadSteer(benchSessionID)
			if err != nil || len(raw) == 0 {
				t.Fatalf("steer log: %d lines, %v", len(raw), err)
			}
			log := bytes.Join(raw, nil)
			for _, p := range paths {
				if got := bytes.Contains(log, []byte(p)); got != c.plain {
					t.Errorf("the steer log holds the path %s %v, want %v", p, got, c.plain)
				}
			}
			if !bytes.Contains(log, []byte(ubPostTitle)) {
				t.Errorf("the steer log lost the lesson's title, a declared record kept at every scope")
			}
		})
	}
}

// TestPost_UnitBriefNamesTheUnitFromTheHooksDirectory (ruling 04:58 B): the
// note names the unit's directory as the agent reaches it from where the
// hook runs, as the prompt's brief names its paths (relTo): '.' from inside
// the unit. TestPost_UnitBriefLineFollowsTheRecordScope is the witness from
// the project root, where it is 'qwertyui'.
func TestPost_UnitBriefNamesTheUnitFromTheHooksDirectory(t *testing.T) {
	s := ubPostSession(t, store.RecordScopeV5)
	s.writeBrief("c1")
	s.cwd = filepath.Join(s.project, "qwertyui")
	out := s.edit("toolu_ub_1")
	if !strings.Contains(out, ubPostTitle) {
		t.Fatalf("premise: the edit from inside the unit printed %q, want the brief", out)
	}
	if want := "rashomon record: an edit in unit '.', noted once a session;"; !strings.Contains(out, want) {
		t.Errorf("the note %q, want %q", out, want)
	}
}

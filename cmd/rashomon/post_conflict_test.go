package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The post's conflict check end to end (W-M2.4 hook seam, card m2-seam1):
// a Go module whose index at c1 holds each file's content digest, with
// internal/graph/g2.go already importing internal/hookio, and a bound spec
// with a boundary rule from internal/graph into internal/hookio and a
// protect_path rule over keep/*.go. The base is written by the real prompt
// entry, every edit lands through the real post entry.
const (
	cfModule = "example.com/cf"
	cfGraph  = "internal/graph/g.go"
	cfGraph2 = "internal/graph/g2.go"
	cfHookio = "internal/hookio/h.go"
	cfKeep   = "keep/p.go"
	cfWordA  = "zqxvwurble"
	cfWordB  = "plinthorax"
	cfSpec   = `{"rashomon_spec": 1, "rules": [` +
		`{"id": "b1", "kind": "boundary", "from": ["unit:go:internal/graph"], "to": ["unit:go:internal/hookio"], "why": "w", "hard_control": "h"}, ` +
		`{"id": "p1", "kind": "protect_path", "paths": ["keep/*.go"], "why": "w", "hard_control": "h"}]}`
	cfCross   = "package graph\n\nimport _ \"" + cfModule + "/internal/hookio\"\n"
	cfNote    = "spec rule b1: this edit in unit 'internal/graph' adds a call, import or reference across a boundary a rule sets."
	cfProtect = "spec rule p1: this edit in unit 'keep' edits a file a rule protects."
)

// cfBodies are the project's files at c1.
var cfBodies = map[string]string{
	cfGraph:  "package graph\n",
	cfGraph2: cfCross,
	cfHookio: "package hookio\n",
	cfKeep:   "package keep\n",
}

// cfProject is the module and its index at c1, in repository planRID.
func cfProject(t testing.TB) string {
	t.Helper()
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module "+cfModule+"\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, body := range cfBodies {
		path := filepath.Join(project, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfIndex(t, project, "c1")
	return project
}

// cfIndex writes the index of cfBodies, in repository planRID, at commit.
func cfIndex(t testing.TB, project, commit string) {
	t.Helper()
	rev := graph.RevCommit(commit)
	s := graph.NewStore()
	for rel, body := range cfBodies {
		fid := graph.FileID(rel)
		s.AssertNode(graph.Node{ID: fid, Kind: "file", Attrs: map[string]string{"digest": graph.ContentDigest([]byte(body)), "lang": "go"}, Rev: rev})
		s.AssertEdge(graph.Edge{Src: graph.UnitID("go", filepath.ToSlash(filepath.Dir(rel))), Pred: graph.Contains, Dst: fid, Rev: rev})
	}
	s.AssertEdge(graph.Edge{Src: graph.FileID(cfGraph2), Pred: graph.Imports, Dst: graph.UnitID("go", "internal/hookio"), Rev: rev})
	if err := graph.SaveFile(project, s, graph.Header{Repo: planRID, Rev: graph.HeaderRev{Commit: commit}}); err != nil {
		t.Fatal(err)
	}
}

// cfSession is a plan session over cfProject with cfSpec bound, keeping the
// session's content when kept is true.
func cfSession(t *testing.T, scope string, kept bool) *planSession {
	t.Helper()
	var s *planSession
	if kept {
		s = ubPostSession(t, scope)
	} else {
		s = newPlanSession(t, scope)
	}
	s.project = cfProject(t)
	cfBindSpec(t, s.root, s.project)
	return s
}

// cfBindSpec writes cfSpec into project and binds it in the store at root,
// as spec trust binds it, its digest as spec.Parse gives it.
func cfBindSpec(t testing.TB, root, project string) {
	t.Helper()
	path := filepath.Join(project, ".claude", "rashomon", "spec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(cfSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	_, digest, err := spec.Parse([]byte(cfSpec))
	if err != nil {
		t.Fatal(err)
	}
	bound := spec.Binding{ProjectRoot: hook.SpecProject(path), SpecPath: path, SpecDigest: digest, BoundAtMS: 1}
	if err := hook.WriteSpecBindings(root, &spec.Bindings{Bindings: []spec.Binding{bound}}); err != nil {
		t.Fatal(err)
	}
}

// prompt runs the real prompt entry (recap pending) on text.
func (s *planSession) prompt(text string) string {
	s.t.Helper()
	head := newBenchHead("UserPromptSubmit")
	head.CWD = s.project
	body := benchJSON(s.t, struct {
		benchHead
		Prompt   string `json:"prompt"`
		PromptID string `json:"prompt_id"`
	}{head, text, "prompt-1"})
	var out, errOut bytes.Buffer
	if code := cmdRecap([]string{"pending", install.Marker, s.id}, bytes.NewReader(body), &out, &errOut); code != exitOK || errOut.Len() != 0 {
		s.t.Fatalf("prompt exited %d, stderr %q", code, errOut.String())
	}
	return out.String()
}

// land writes body to the project's file rel and runs the real post entry
// on that Write.
func (s *planSession) land(useID, rel, body string) string {
	s.t.Helper()
	path := filepath.Join(s.project, filepath.FromSlash(rel))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		s.t.Fatal(err)
	}
	return s.call(true, "Write", useID, map[string]any{"file_path": path, "content": body})
}

// run is the session's run as the store reads it.
func (s *planSession) run() *store.Run {
	s.t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		s.t.Fatal(err)
	}
	run, err := st.ReadRun(benchSessionID)
	if err != nil {
		s.t.Fatal(err)
	}
	return run
}

func (s *planSession) basePath() string { return conflictBasePath(s.root, benchSessionID, s.project) }

// cfIsDigest reports the store's digest form: 64 lowercase hex.
func cfIsDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// TestConflictBase_HoldsNoWordOfThePrompt (D2, D160): the prompt entry
// writes the base while the session's content is kept; it holds the rules
// and the commit and no word of the prompt. Without the marker no base is
// written.
func TestConflictBase_HoldsNoWordOfThePrompt(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph + " the " + cfWordA + " way, " + cfWordB)
	data, err := os.ReadFile(s.basePath())
	if err != nil {
		t.Fatalf("the prompt wrote no conflict base: %v", err)
	}
	for _, w := range []string{cfWordA, cfWordB, "the " + cfWordA} {
		if bytes.Contains(data, []byte(w)) {
			t.Errorf("the base holds %q", w)
		}
	}
	head, ok := readConflictBaseHead(s.basePath())
	if !ok || head.Commit != "c1" || head.Repo != planRID {
		t.Errorf("head %+v ok %v, want commit c1 in %s", head, ok, planRID)
	}
	cb, ok := readConflictBase(s.basePath())
	if !ok || len(cb.Rules) != 2 || len(cb.Records) == 0 || cb.Digests[graph.FileID(cfKeep)] == "" {
		t.Errorf("base: ok %v, %d rules, %d records, digests %v; want both rules, the standing crossing and keep's digest",
			ok, len(cb.Rules), len(cb.Records), cb.Digests)
	}

	off := cfSession(t, store.RecordScopeV5, false)
	off.prompt("fix " + cfGraph)
	if _, err := os.Stat(off.basePath()); !os.IsNotExist(err) {
		t.Errorf("no marker: the base exists (stat %v)", err)
	}
}

// TestPostConflicts_BoundaryFromSpecOnePerDirection (D3, D4, D6): a new
// import from internal/graph into internal/hookio raises boundary_cross
// under b1, as a note and a record row naming the unit at scope v5; the
// reverse import raises nothing, and since internal/hookio is under no
// rule's From, writes no record.
func TestPostConflicts_BoundaryFromSpecOnePerDirection(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	if out := s.land("toolu_cf1", cfGraph, cfCross); !strings.Contains(out, cfNote) {
		t.Errorf("post output %q lacks %q", out, cfNote)
	}
	cs := s.run().ConflictChecks
	if len(cs) != 1 || len(cs[0].Rows) != 1 {
		t.Fatalf("records %+v, want one check with one row", cs)
	}
	c, r := cs[0], cs[0].Rows[0]
	if c.Commit != "c1" || c.LandedSeq != s.execSeq("toolu_cf1") || !cfIsDigest(c.File) {
		t.Errorf("check %+v, want commit c1, the edit's seq and a file digest", c)
	}
	if r.Kind != "boundary_cross" || strings.TrimPrefix(r.RuleID, "spec:") != "b1" || r.UnitPath != "internal/graph" ||
		!strings.HasSuffix(r.Unit, "unit:go:internal/graph") || !cfIsDigest(r.Key) {
		t.Errorf("row %+v, want boundary_cross under b1 in internal/graph, the key a digest", r)
	}

	if out := s.land("toolu_cf2", cfHookio, "package hookio\n\nimport _ \""+cfModule+"/internal/graph\"\n"); cfSaysAConflict(out) {
		t.Errorf("the reverse import: post output %q, want no conflict", out)
	}
	if n := len(s.run().ConflictChecks); n != 1 {
		t.Errorf("after an out-of-scope edit: %d checks, want 1", n)
	}
}

// TestPostConflicts_OutOfScopeEditWritesNoRecord (D4): an edit in no rule's
// scope writes no record, and an in-scope edit that raises nothing writes
// one with no row: the record says the check ran.
func TestPostConflicts_OutOfScopeEditWritesNoRecord(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	s.land("toolu_cf1", cfHookio, "package hookio\n\nfunc H() {}\n")
	if n := len(s.run().ConflictChecks); n != 0 {
		t.Fatalf("an out-of-scope edit: %d checks, want 0", n)
	}
	s.land("toolu_cf2", cfGraph, "package graph\n\nfunc G() {}\n")
	if cs := s.run().ConflictChecks; len(cs) != 1 || len(cs[0].Rows) != 0 {
		t.Errorf("an in-scope edit that raises nothing: %+v, want one check, no row", cs)
	}
}

// TestPostConflicts_PreexistingCrossingIsNotNew (AC-M2.1): an edit of g2.go
// that keeps the import it had at c1 raises nothing; the check is recorded
// with no row.
func TestPostConflicts_PreexistingCrossingIsNotNew(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph2)
	if out := s.land("toolu_cf1", cfGraph2, cfCross+"\nfunc G2() {}\n"); cfSaysAConflict(out) {
		t.Errorf("post output %q, want no conflict for a crossing the base holds", out)
	}
	if cs := s.run().ConflictChecks; len(cs) != 1 || len(cs[0].Rows) != 0 {
		t.Errorf("checks %+v, want one, no row", cs)
	}
}

// TestPostConflicts_RestoredFileRaisesNothing (D3, D5): an edit of a
// protected file raises protected_path; restoring the file's c1 content
// raises nothing, and the Stop's fold reads the conflict as resolved at the
// restoring edit.
func TestPostConflicts_RestoredFileRaisesNothing(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfKeep)
	if out := s.land("toolu_cf1", cfKeep, "package keep\n\nfunc P() {}\n"); !strings.Contains(out, cfProtect) {
		t.Errorf("post output %q lacks %q", out, cfProtect)
	}
	if out := s.land("toolu_cf2", cfKeep, cfBodies[cfKeep]); cfSaysAConflict(out) {
		t.Errorf("the restore: post output %q, want no conflict", out)
	}
	cs := s.run().ConflictChecks
	if len(cs) != 2 || len(cs[0].Rows) != 1 || len(cs[1].Rows) != 0 {
		t.Fatalf("checks %+v, want a row then none", cs)
	}
	seen := conflictsSeen(s.run(), 0)
	if len(seen) != 1 || seen[0].Row.Kind != "protected_path" || seen[0].ResolvedSeq != s.execSeq("toolu_cf2") ||
		!slices.Equal(seen[0].Seqs, []int64{s.execSeq("toolu_cf1")}) {
		t.Errorf("fold %+v, want protected_path raised at the first edit and resolved at the restore", seen)
	}
}

// TestPostConflicts_NoBaseIsAMissNeverALoad (D3): with no base the post
// checks nothing and records nothing; with the base and the index's body
// broken past reading, the post still raises the conflict, so the post
// never loads the index.
func TestPostConflicts_NoBaseIsAMissNeverALoad(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	if out := s.land("toolu_cf1", cfGraph, cfCross); cfSaysAConflict(out) {
		t.Errorf("no base: post output %q, want nothing", out)
	}
	if n := len(s.run().ConflictChecks); n != 0 {
		t.Errorf("no base: %d checks, want 0", n)
	}

	s = cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	s.breakIndexBody()
	if out := s.land("toolu_cf1", cfGraph, cfCross); !strings.Contains(out, cfNote) {
		t.Errorf("an unreadable index body: post output %q lacks %q", out, cfNote)
	}
}

// TestPostConflicts_BaseAtAnotherCommitChecksNothing (D3): a base written
// at c1 says nothing of a tree indexed at c2, so the post of an edit that
// crosses b1 checks and records nothing. Witness: in a session whose base
// was written at c2, the same edit raises the note.
func TestPostConflicts_BaseAtAnotherCommitChecksNothing(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	cfIndex(t, s.project, "c2")
	if out := s.land("toolu_cf1", cfGraph, cfCross); cfSaysAConflict(out) {
		t.Errorf("a base at c1 under an index at c2: post output %q, want nothing", out)
	}
	if n := len(s.run().ConflictChecks); n != 0 {
		t.Errorf("a base at c1 under an index at c2: %d checks, want 0", n)
	}
	w := cfSession(t, store.RecordScopeV5, true)
	cfIndex(t, w.project, "c2")
	w.prompt("fix " + cfGraph)
	if out := w.land("toolu_cf1", cfGraph, cfCross); !strings.Contains(out, cfNote) {
		t.Errorf("witness: with the base at c2, post output %q lacks %q", out, cfNote)
	}
}

// TestPost_ConflictNudgesAndNeverBlocks (D6, R2): the note is context, never
// a block; it is given once a session per commit, while every check is
// recorded whatever was delivered.
func TestPost_ConflictNudgesAndNeverBlocks(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	out := s.land("toolu_cf1", cfGraph, cfCross)
	if !strings.Contains(out, cfNote) || strings.Contains(out, `"decision"`) || strings.Contains(out, "block") {
		t.Errorf("post output %q, want the note and no decision", out)
	}
	if again := s.land("toolu_cf2", cfGraph, cfCross+"\nfunc G() {}\n"); cfSaysAConflict(again) {
		t.Errorf("the same conflict again: post output %q, want it given once", again)
	}
	if cs := s.run().ConflictChecks; len(cs) != 2 || len(cs[1].Rows) != 1 || cs[1].Rows[0].Key != cs[0].Rows[0].Key {
		t.Errorf("checks %+v, want the conflict recorded at both edits under one key", cs)
	}
	// The steer line keeps its pattern, kind and key form; its conflict slot
	// holds the Stop's phrase, the engine's kind word staying in the record's
	// conflict_check row.
	var lines []steer.Line
	for _, l := range s.steerLines() {
		if strings.HasPrefix(l.Key, conflictKeyPrefix) {
			lines = append(lines, l)
		}
	}
	phrase, _ := recap.ConflictPhrase("boundary_cross")
	if len(lines) != 1 || lines[0].Pattern != string(pattern.DeclaredFact) || lines[0].Kind != string(steer.KindConflict) {
		t.Fatalf("conflict steer lines %+v, want one, pattern %s, kind %s", lines, pattern.DeclaredFact, steer.KindConflict)
	}
	var said string
	if err := json.Unmarshal(lines[0].Slots["conflict"], &said); err != nil || said != phrase {
		t.Errorf("the steer line's conflict slot %s (%v), want %q", lines[0].Slots["conflict"], err, phrase)
	}
}

// TestConflictFindings_CapAtNudgeWithNoStopForm (R2): a conflict's finding
// caps at nudge with no stop form. The router lowers a stop to a nudge while
// no project stop setting exists (decide.go, STR-20), so no post output can
// show this cap today; the finding is the pin that holds once one lands.
func TestConflictFindings_CapAtNudgeWithNoStopForm(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	e := hook.LandedGoEdit{Session: "s-cap", Root: root, Cwd: root, Seq: 1, ScopeV5: true}
	live := []engine.Conflict{{Kind: engine.ConflictBoundaryCross, Unit: "unit:go:internal/graph", RuleID: "b1"}}
	rows := []store.ConflictCheckRow{{Key: strings.Repeat("a", 64)}}
	fs := conflictFindings(st, e, cfModule, "c1c1c1c1c1c1c1", live, rows)
	if len(fs) != 1 {
		t.Fatalf("findings %+v, want one", fs)
	}
	if f := fs[0]; f.MaxLevel != steer.LevelNudge || f.StopForm != steer.StopFormNone {
		t.Errorf("finding max level %d, stop form %q, want nudge and none", f.MaxLevel, f.StopForm)
	}
}

// TestPostConflicts_RecordScopeV4HoldsNoPath (D4): at record scope v4 the
// row's unit and subject are digests and its unit path is empty, and the
// note still names the unit.
func TestPostConflicts_RecordScopeV4HoldsNoPath(t *testing.T) {
	s := cfSession(t, store.RecordScopeV4, true)
	s.prompt("fix " + cfGraph)
	if out := s.land("toolu_cf1", cfGraph, cfCross); !strings.Contains(out, cfNote) {
		t.Errorf("post output %q lacks %q", out, cfNote)
	}
	cs := s.run().ConflictChecks
	if len(cs) != 1 || len(cs[0].Rows) != 1 {
		t.Fatalf("checks %+v, want one row", cs)
	}
	if r := cs[0].Rows[0]; r.UnitPath != "" || !cfIsDigest(r.Unit) || !cfIsDigest(r.Subject) ||
		strings.Contains(r.Unit+r.Subject, "internal") {
		t.Errorf("row %+v, want digests and no path", r)
	}
}

// cfCheck is a conflict_check at seq of file at commit, raising keys.
func cfCheck(seq, landed int64, commit, file string, keys ...string) store.ConflictCheck {
	rows := []store.ConflictCheckRow{}
	for _, k := range keys {
		rows = append(rows, store.ConflictCheckRow{Kind: "boundary_cross", RuleID: "b1", Unit: "g:r#unit:go:a", UnitPath: "a",
			Subject: "s-" + k, Key: k})
	}
	return store.ConflictCheck{EventHeader: store.EventHeader{Type: store.TypeConflictCheck, Seq: &seq}, LandedSeq: landed,
		Commit: commit, File: file, Rows: rows}
}

// TestConflictsSeen_FoldFromRecord (D5): a conflict shown in two files
// resolves only when neither shows it; raised again it gains a seq and is
// unresolved; the bound seq reads the fold as of then; no check is nil.
func TestConflictsSeen_FoldFromRecord(t *testing.T) {
	if got := foldConflictChecks(&store.Run{}, 0); got != nil {
		t.Errorf("no check: %+v, want nil", got)
	}
	run := &store.Run{ConflictChecks: []store.ConflictCheck{
		cfCheck(10, 9, "c1", "fA", "k1"),
		cfCheck(12, 11, "c1", "fB", "k1"),
		cfCheck(14, 13, "c1", "fA"),
		cfCheck(16, 15, "c1", "fB"),
		cfCheck(18, 17, "c1", "fA", "k1", "k2"),
	}}
	at := foldConflictChecks(run, 14)
	if len(at) != 1 || at[0].ResolvedSeq != 0 || !slices.Equal(at[0].Seqs, []int64{9}) {
		t.Errorf("at 14, fB still showing k1: %+v, want k1 raised at 9, unresolved", at)
	}
	at = foldConflictChecks(run, 16)
	if len(at) != 1 || at[0].ResolvedSeq != 15 {
		t.Errorf("at 16: %+v, want k1 resolved at 15", at)
	}
	all := foldConflictChecks(run, 0)
	if len(all) != 2 || all[0].Row.Subject != "s-k1" || all[1].Row.Subject != "s-k2" {
		t.Fatalf("whole run: %+v, want k1 then k2", all)
	}
	if !slices.Equal(all[0].Seqs, []int64{9, 17}) || all[0].ResolvedSeq != 0 || !slices.Equal(all[1].Seqs, []int64{17}) {
		t.Errorf("whole run: %+v, want k1 at 9 and 17, unresolved, k2 at 17", all)
	}
}

// TestConflictsSeen_NeverResolvedAcrossCommits (ruling 02:11 item 4): a
// check at another commit resolves nothing, and the same key raised there
// is a conflict of its own.
func TestConflictsSeen_NeverResolvedAcrossCommits(t *testing.T) {
	run := &store.Run{ConflictChecks: []store.ConflictCheck{
		cfCheck(10, 9, "c1", "fA", "k1"),
		cfCheck(12, 11, "c2", "fA"),
		cfCheck(14, 13, "c2", "fA", "k1"),
	}}
	got := foldConflictChecks(run, 0)
	if len(got) != 2 || got[0].ResolvedSeq != 0 || !slices.Equal(got[0].Seqs, []int64{9}) || !slices.Equal(got[1].Seqs, []int64{13}) {
		t.Errorf("fold %+v, want k1 at c1 unresolved and k1 at c2 raised at 13", got)
	}
}

// TestStop_ConflictFromARealPost (AC-M2.2): the conflict a real post
// records reaches the Stop's line through the production fold, the unit
// named.
func TestStop_ConflictFromARealPost(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	s.land("toolu_cf1", cfGraph, cfCross)
	seen := conflictsSeen(s.run(), 0)
	if len(seen) != 1 || seen[0].Row.UnitPath != "internal/graph" || seen[0].ResolvedSeq != 0 ||
		!slices.Equal(seen[0].Seqs, []int64{s.execSeq("toolu_cf1")}) {
		t.Fatalf("fold %+v, want the boundary_cross in internal/graph, unresolved", seen)
	}
	code, out, _ := runStopWith(t, seen)
	if msg := systemMessage(t, out); code != exitOK || !strings.Contains(msg, "internal/graph") {
		t.Errorf("Stop exit %d, message %q; want exit 0 and the unit named", code, msg)
	}
}

// TestConflictMessages_EveryLiveKindRenders (D6): the conflict slot's table
// is the Stop line's phrase for each live kind, which takes no engine kind
// word, and a withheld kind has no phrase and no note; each live kind's note
// renders with a rule the slot takes and with one it refuses (the unruled
// form), and below v5 with its record form, each saying the kind's phrase.
func TestConflictMessages_EveryLiveKindRenders(t *testing.T) {
	for _, k := range verify.ConflictRowKinds {
		phrase, ok := recap.ConflictPhrase(k)
		if !ok {
			t.Errorf("live kind %s has no phrase", k)
			continue
		}
		if err := slot.NewEnum(slot.EnumConflictPhrase, phrase).Validate(); err != nil {
			t.Errorf("kind %s: the slot refuses its phrase %q: %v", k, phrase, err)
		}
		if slot.NewEnum(slot.EnumConflictPhrase, k).Validate() == nil {
			t.Errorf("kind %s: the slot takes the engine's word", k)
		}
		for _, rule := range []string{"b1", "spec:b1", "rule.with_dots"} {
			for _, v5 := range []bool{true, false} {
				c := engine.Conflict{Kind: engine.ConflictKind(k), RuleID: rule}
				m, rec, ok := conflictMessages(c, slot.RelPath("internal/graph"), v5)
				if !ok {
					t.Errorf("kind %s rule %s v5 %v: no note", k, rule, v5)
					continue
				}
				if !strings.Contains(m.Text(), phrase) || !v5 && !strings.Contains(rec.Text(), phrase) {
					t.Errorf("kind %s rule %s v5 %v: note %q, record %q; want each to say %q", k, rule, v5, m.Text(), rec.Text(), phrase)
				}
			}
		}
	}
	for _, k := range verify.WithheldConflictKinds {
		if _, ok := recap.ConflictPhrase(k); ok {
			t.Errorf("withheld kind %s has a phrase", k)
		}
		if _, _, ok := conflictMessages(engine.Conflict{Kind: engine.ConflictKind(k), RuleID: "b1"}, slot.RelPath("internal/graph"), true); ok {
			t.Errorf("withheld kind %s renders a note", k)
		}
	}
}

// TestConflictNote_SaysTheStopsBoundaryPhrase: the post's note for a
// boundary crossing says the Stop line's phrase for it verbatim
// (internal/recap/conflicts.go), in the Stop line's order, the rule and then
// what the edit did, so a reader matching the Stop's words on a session's
// output finds the post's note too. Break: reword either side.
func TestConflictNote_SaysTheStopsBoundaryPhrase(t *testing.T) {
	const phrase = "adds a call, import or reference across a boundary a rule sets"
	if p, ok := recap.ConflictPhrase("boundary_cross"); !ok || p != phrase {
		t.Fatalf("the Stop's phrase is %q (%v), want %q", p, ok, phrase)
	}
	c := engine.Conflict{Kind: engine.ConflictKind("boundary_cross"), RuleID: "spec:b1"}
	m, rec, ok := conflictMessages(c, slot.RelPath("internal/graph"), false)
	if !ok {
		t.Fatal("no note")
	}
	if want := "rashomon record: spec rule b1: this edit in unit 'internal/graph' " + phrase + "."; m.Text() != want {
		t.Errorf("note %q, want %q", m.Text(), want)
	}
	if want := "rashomon record: spec rule b1: an edit " + phrase + "."; rec.Text() != want {
		t.Errorf("record form %q, want %q", rec.Text(), want)
	}
	if !cfSaysAConflict(m.Text()) || !cfSaysAConflict(rec.Text()) || cfSaysAConflict(cfBodies[cfKeep]) {
		t.Error("cfSaysAConflict, the no-note checks' probe, misses the note or matches a file")
	}
}

// TestConflictNote_LongestFitsTheCap: the longest note the slots allow, a
// 48-byte rule id and a 268-byte unit path (four "../" and 256 bytes), with
// each live kind's phrase, renders whole, its record form too, within the
// cap on a rendered note (render.MaxMessageBytes), so no conflict note is
// dropped for its length; the premises pin both lengths as the slots'
// longest. Break: a phrase or format long enough to pass the cap.
func TestConflictNote_LongestFitsTheCap(t *testing.T) {
	rule := "r" + strings.Repeat("a", 47)
	unit := strings.Repeat("../", 4) + strings.Repeat("a", 127) + "/" + strings.Repeat("b", 128)
	if slot.RuleID(rule).Validate() != nil || slot.RuleID(rule+"a").Validate() == nil {
		t.Fatalf("premise: %d bytes is the longest rule id the slot takes", len(rule))
	}
	if slot.RelPath(unit).Validate() != nil || slot.RelPath(unit+"b").Validate() == nil || slot.RelPath("../"+unit).Validate() == nil {
		t.Fatalf("premise: %d bytes is the longest unit path the slot takes", len(unit))
	}
	longest := 0
	for _, k := range verify.ConflictRowKinds {
		phrase, ok := recap.ConflictPhrase(k)
		if !ok {
			t.Fatalf("live kind %s has no phrase", k)
		}
		m, rec, ok := conflictMessages(engine.Conflict{Kind: engine.ConflictKind(k), RuleID: "spec:" + rule}, slot.RelPath(unit), false)
		if !ok {
			t.Errorf("kind %s: the longest note does not render", k)
			continue
		}
		for _, s := range []string{m.Text(), rec.Text()} {
			if err := render.Validate(s); err != nil || !strings.Contains(s, "spec rule "+rule+": ") || !strings.HasSuffix(s, " "+phrase+".") {
				t.Errorf("kind %s: %q (%v), want it whole and valid", k, s, err)
			}
		}
		if !strings.Contains(m.Text(), " unit '"+unit+"' ") {
			t.Errorf("kind %s: %q does not hold the whole unit", k, m.Text())
		}
		longest = max(longest, len(m.Text()))
	}
	if longest == 0 || longest > render.MaxMessageBytes {
		t.Errorf("the longest conflict note is %d bytes, the cap %d", longest, render.MaxMessageBytes)
	}
}

// cfSaysAConflict reports whether out holds a conflict note: any live kind's
// phrase, which every conflict note says (TestConflictNote_*).
func cfSaysAConflict(out string) bool {
	for _, k := range verify.ConflictRowKinds {
		if p, ok := recap.ConflictPhrase(k); ok && strings.Contains(out, p) {
			return true
		}
	}
	return false
}

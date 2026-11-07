package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// graphProject writes the engine's specimen under dir with an index
// configuration whose INC-212 record declares a clause and the purge job
// as a file target, and indexes it.
func graphProject(t *testing.T, dir string) {
	t.Helper()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "internal", "graph", "graphtest", "specimen"))); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"units": []map[string]any{
			{"path": "services/auth", "mod_root": "services/auth", "modules": []string{"auth"}, "lang": "py"},
			{"path": "services/orgs", "mod_root": "services/orgs", "modules": []string{"orgs"}, "lang": "py"},
			{"path": "services/api", "mod_root": "services/api", "modules": []string{"api"}, "lang": "py"},
			{"path": "jobs", "mod_root": "", "modules": []string{"jobs"}, "lang": "py"},
			{"path": "tests", "mod_root": "", "modules": []string{"tests"}, "lang": "py"},
			{"path": "reports", "mod_root": "", "modules": []string{"reports"}, "lang": "py"},
		},
		"service_map": map[string]string{"auth": "services/auth", "orgs": "services/orgs", "api": "services/api"},
		"knowledge": []map[string]any{{
			"id": "les:INC-212", "kind": "lesson", "title": "INC-212 session purge raced the TTL",
			"applies_to": []string{"tbl:public.sessions", "file:jobs/purge_sessions.py"},
			"text":       "INC-212: the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS; any change to session lifetime must change the purge predicate too, or live sessions are deleted (2025-08).",
			"clause":     "the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too",
			"source":     "incidents/INC-212.md",
		}},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".rashomon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".rashomon", "graph.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := graphIndex([]string{dir}, io.Discard); err != nil {
		t.Fatalf("graph index: %v", err)
	}
}

// TestGS5_TheGraphBriefRoutesAsAPreflightFinding (card GRAPH-SLOT1): a
// prompt whose words select code under an indexed repository gets the
// graph brief as a task-preflight finding of its own, routed and logged
// in the preflight's one Route call, so the steer line holds its slots
// and model text, the learn loop's delivered set holds the knowledge its
// lines named, and the Stop recap's delivered notes hold its model text;
// paths print relative to the hook's cwd, here two directories under the
// index root. The explanation is on disk under the store's graph
// directory. The request names no path, so with no graph under its cwd it
// would have had no note at all.
func TestGS5_TheGraphBriefRoutesAsAPreflightFinding(t *testing.T) {
	e := newLessonEnv(t)
	root := e.cwd
	graphProject(t, root)
	e.cwd = filepath.Join(root, "services", "auth")
	e.start("s1")
	out := e.prompt("s1", "p1", "make session expiry respect the org-level timeout override.")
	t.Logf("additionalContext:\n%s", out.context)
	if !strings.Contains(out.context, "rashomon record: the code this request touches, read from the repository graph at rev ") {
		t.Fatalf("the prompt's context has no graph brief:\n%s", out.context)
	}
	want := "lesson INC-212 applies to '../../jobs/purge_sessions.py': failure 'INC-212 session purge raced the TTL'; rule 'the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too'; source 'incidents/INC-212.md'"
	if !strings.Contains(out.context, want) {
		t.Errorf("the lesson line is not\n%s\nin\n%s", want, out.context)
	}
	st, err := store.Open(e.root)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := st.ReadSteer("s1")
	if err != nil {
		t.Fatal(err)
	}
	var line struct {
		Pattern, Key, State, ModelText string `json:"-"`
		P                              string `json:"pattern"`
		K                              string `json:"key"`
		S                              string `json:"state"`
		M                              string `json:"model_text"`
		Slots                          map[string]json.RawMessage
	}
	found := false
	for _, b := range raw {
		if err := json.Unmarshal(b, &line); err != nil {
			t.Fatal(err)
		}
		if line.P == "task-preflight" && line.K == keyGraph {
			found = true
			t.Logf("steer line: state %s, line2 slot %s", line.S, line.Slots["line2"])
			if line.S != "delivered" || !strings.HasPrefix(line.M, "rashomon record: the code this request touches") {
				t.Errorf("graph line state %q model_text %q", line.S, line.M)
			}
			if line.Slots["rev"] == nil || line.Slots["line1"] == nil {
				t.Errorf("graph line slots %v", line.Slots)
			}
		}
	}
	if !found {
		t.Fatal("no task-preflight line with the graph key")
	}
	if given := learn.Delivered(raw); !given["les:INC-212"] {
		t.Errorf("learn.Delivered = %v, want les:INC-212 given", given)
	}
	notes := recap.DeliveredNotes(raw, "p1")
	seen := false
	for _, n := range notes {
		seen = seen || strings.HasPrefix(string(n.ModelText), "rashomon record: the code this request touches")
	}
	if !seen {
		t.Errorf("recap.DeliveredNotes = %d notes, none the graph brief", len(notes))
	}
	if _, err := os.Stat(filepath.Join(e.root, "graph", "s1-p1.txt")); err != nil {
		t.Errorf("evidence file: %v", err)
	}
}

// TestGraphKnowledgeImport_RowsReachTheIndex (W-M1.6, AC-M1.3): the verb,
// run from a directory under an indexed repository, writes the ledger's
// rows under .rashomon/knowledge/ with applies_to as global ids under the
// index's repository, prints the report, and indexes again, so the graph
// holds the lesson with its clause and its edge to the file the row named.
func TestGraphKnowledgeImport_RowsReachTheIndex(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	src := filepath.Join(t.TempDir(), "LEDGER.txt")
	if err := os.WriteFile(src, []byte("id | when | who | what | layer | evidence | catcher | today | gap\n"+
		"B1 | 11-01 | me | changed the session ttl without jobs/purge_sessions.py | purge / DETECT | run 3 | me | no | the nightly purge must change with session lifetime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(dir, "services", "auth"))
	var out strings.Builder
	if err := cmdGraph([]string{"knowledge", "import", src}, &out); err != nil {
		t.Fatalf("graph knowledge import: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "LEDGER.txt: 1 row(s), 1 with a clause, 0 title only, 0 unparsed; 1 title(s) a brief can carry; applies_to 1 resolved, 0 path(s) not in the index -> ") || !strings.Contains(out.String(), "indexed ") {
		t.Errorf("output = %q", out.String())
	}
	s, h, err := graph.LoadFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	rev := graph.RevCommit(h.Rev.Commit)
	n, ok := s.Node("les:B1", rev)
	if !ok || n.Attrs["clause"] != "the nightly purge must change with session lifetime" || n.Tier != graph.Inferred {
		t.Errorf("les:B1 = %+v, %v; want the inferred lesson with its clause", n, ok)
	}
	if e := s.Out("les:B1", rev, graph.AppliesTo); len(e) != 1 || e[0].Dst != "file:jobs/purge_sessions.py" {
		t.Errorf("les:B1 applies_to = %+v; want the purge job", e)
	}
	rows, err := knowledge.ReadDir(dir)
	if err != nil || len(rows) != 1 || len(rows[0].AppliesTo) != 1 || rows[0].AppliesTo[0] != "g:"+h.Repo+"#file:jobs/purge_sessions.py" {
		t.Errorf("knowledge rows = %+v, %v; want one row keyed to this repository", rows, err)
	}
}

// TestGraphFinding_CarriesSourceAndUnit (W-M1.7, AD19, AC-X.5): the brief's
// finding names where its claim came from and what it is about: the store
// class, the global id of the node its first line is keyed on, and the unit
// that contains that node, all under the index's repository id.
func TestGraphFinding_CarriesSourceAndUnit(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	_, h, err := graph.LoadFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	gb, ok := graphBrief("", filepath.Join(dir, "services", "auth"), "s1", "p1", []byte("purge sessions"))
	if !ok || gb.Repo != h.Repo || gb.Unit == "" {
		t.Fatalf("graphBrief = repo %q unit %q, %v; want repo %q and a unit", gb.Repo, gb.Unit, ok, h.Repo)
	}
	f, ok := graphFinding(gb, 9, nil)
	if !ok {
		t.Fatal("no finding")
	}
	if err := f.Check(); err != nil {
		t.Errorf("Check: %v", err)
	}
	first := graph.GlobalID(h.Repo, gb.Lines[0].Key)
	if f.Source.Kind != steer.SourceGraph || f.Source.ID != string(first) || f.Source.Tier == "" {
		t.Errorf("source = %+v; want graph, %s, the node's tier", f.Source, first)
	}
	if _, local, ok := graph.SplitGlobal(graph.ID(f.Unit)); !ok || !strings.HasPrefix(string(local), "unit:py:") || f.Unit != string(gb.Unit) {
		t.Errorf("unit = %q; want this repository's unit:py:... containing %s", f.Unit, gb.Lines[0].Key)
	}
}

// TestHeadCommit_ReadsThroughAWorktree (W-M1.8; a W-M1.4 defect): the index
// header's commit is HEAD, read without git: a branch ref resolved under the
// common dir's refs or its packed-refs (a worktree's HEAD sits in its own
// git dir and the refs in the common dir), or a detached HEAD's own hash;
// C1 outside a repository. Before this, every worktree indexed as C1, so no
// row's rev named a commit and no freshness check could.
func TestHeadCommit_ReadsThroughAWorktree(t *testing.T) {
	a, b, c := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	main := t.TempDir()
	gitdir := filepath.Join(main, ".git")
	write(filepath.Join(gitdir, "HEAD"), "ref: refs/heads/main\n")
	write(filepath.Join(gitdir, "refs", "heads", "main"), a+"\n")
	write(filepath.Join(gitdir, "packed-refs"), "# pack-refs with: peeled fully-peeled sorted\n"+b+" refs/heads/feature\n")
	wt := t.TempDir()
	wtgit := filepath.Join(gitdir, "worktrees", "wt")
	write(filepath.Join(wtgit, "commondir"), "../..\n")
	write(filepath.Join(wtgit, "HEAD"), "ref: refs/heads/feature\n")
	write(filepath.Join(wt, ".git"), "gitdir: "+wtgit+"\n")
	detached := t.TempDir()
	write(filepath.Join(detached, ".git", "HEAD"), c+"\n")
	for name, tc := range map[string]struct{ dir, want string }{
		"main repository, loose ref":             {main, a[:12]},
		"worktree, packed ref in the common dir": {wt, b[:12]},
		"detached HEAD":                          {detached, c[:12]},
		"no repository":                          {t.TempDir(), "C1"},
	} {
		if got := headCommit(tc.dir); got != tc.want {
			t.Errorf("%s: headCommit = %q, want %q", name, got, tc.want)
		}
	}
}

// TestGraphBrief_EvidenceFileHoldsNoWordOfThePrompt (W-M1.9, D160): the
// file the prompt hook writes under the store's graph directory holds the
// selection and the brief, and no word of the prompt: not the prompt line,
// not a token with its weight, not the words a scope row matched.
func TestGraphBrief_EvidenceFileHoldsNoWordOfThePrompt(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	storeRoot := t.TempDir()
	gb, ok := graphBrief(storeRoot, filepath.Join(dir, "services", "auth"), "s1", "p1", []byte("purging the sessions"))
	if !ok {
		t.Fatal("no brief")
	}
	b, err := os.ReadFile(filepath.Join(storeRoot, "graph", "s1-p1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, word := range []string{"purging", "prompt:", "purg("} {
		if strings.Contains(text, word) {
			t.Errorf("the evidence file holds %q:\n%s", word, text)
		}
	}
	for _, r := range gb.Rows {
		if len(r.Hits) > 0 && strings.Contains(text, "["+strings.Join(r.Hits, " ")+"]") {
			t.Errorf("the evidence file names the words %s matched: %v", r.ID, r.Hits)
		}
	}
	if !strings.Contains(text, "\nbrief:\n"+gb.Text()) || !strings.Contains(text, string(gb.Lines[0].Key)) {
		t.Errorf("the evidence file lacks the brief or its first node:\n%s", text)
	}
}

// TestUnitOf_WalksContainingDeclaringAndAppliesToEdges (W-M1.7, AD19): a
// finding's unit is reached from the node its first line is keyed on
// through the edges that contain or declare it, or the file a lesson
// applies to, within four nodes; a unit is its own; a node the store
// lacks, or one four hops from its unit, has none.
func TestUnitOf_WalksContainingDeclaringAndAppliesToEdges(t *testing.T) {
	s := graph.NewStore()
	rev := graph.RevCommit("c1")
	node := func(id graph.ID, kind string) {
		s.AssertNode(graph.Node{ID: id, Kind: kind, Tier: graph.Declared, Conf: 1.0, Rev: rev})
	}
	edge := func(src graph.ID, p graph.Pred, dst graph.ID) {
		s.AssertEdge(graph.Edge{Src: src, Pred: p, Dst: dst, Tier: graph.Declared, Conf: 1.0, Rev: rev})
	}
	unit, file := graph.UnitID("py", "jobs"), graph.FileID("jobs/purge.py")
	node(unit, "unit")
	node(file, "file")
	edge(unit, graph.Contains, file)
	syms := []graph.ID{
		graph.SymID("py", "jobs", "type", "jobs.purge.Job"),
		graph.SymID("py", "jobs", "method", "jobs.purge.Job.run"),
		graph.SymID("py", "jobs", "func", "jobs.purge.Job.run.inner"),
	}
	prev := file
	for _, id := range syms {
		node(id, "sym")
		edge(prev, graph.Declares, id)
		prev = id
	}
	lesson := graph.ID("les:INC-1")
	node(lesson, "lesson")
	edge(lesson, graph.AppliesTo, file)
	for name, tc := range map[string]struct{ from, want graph.ID }{
		"the unit itself":               {unit, unit},
		"a file":                        {file, unit},
		"a symbol the file declares":    {syms[0], unit},
		"three hops from the unit":      {syms[1], unit},
		"four hops from the unit":       {syms[2], ""},
		"a lesson applying to the file": {lesson, unit},
		"a node the store lacks":        {graph.FileID("jobs/none.py"), ""},
	} {
		if got := unitOf(s, rev, tc.from); got != tc.want {
			t.Errorf("%s: unitOf(%s) = %q, want %q", name, tc.from, got, tc.want)
		}
	}
}

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/recap"
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
	want := "lesson INC-212 applies to '../../jobs/purge_sessions.py': 'the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too'; source 'incidents/INC-212.md'"
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

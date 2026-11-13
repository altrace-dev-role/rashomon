package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// benchConflictPost is BenchmarkRunPost's conflict rows (W-M2.4 seam, card
// m2-seam1): the post of one landed Write of cfGraph in cfProject, cfSpec
// bound, its cwd the project, consented and noted as the unit-brief rows
// are, in a store that keeps the session's content, at record scope v5.
// The rows share the payload and differ in the store alone. With no base,
// the join returns at the base's read: the price every consented Go edit
// pays under a bound spec when the session's prompt left no base. With the
// base the session's prompt wrote, each call reads it, diffs the edit
// against it and records one conflict_check of no row: the edit is in b1's
// From unit and imports nothing, so it crosses nothing and no note is
// given, which keeps every call on one branch. The check after the loop
// holds each row to its branch, so a fixture that drifts to another fails
// rather than measures it.
func benchConflictPost(b *testing.B, base bool) {
	b.Setenv("CLAUDE_PROJECT_DIR", "")
	b.Setenv("AI_AGENT", "claude-code_2-1-280_harness")
	b.Setenv("CI", "")
	root, installID := benchStarted(b)
	if err := store.WriteRecordScopeMarker(root, store.RecordScopeV5); err != nil {
		b.Fatal(err)
	}
	if err := store.WriteSteerDisclosureMarker(root, steer.DisclosureVersion); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
		b.Fatal(err)
	}
	s := &planSession{t: b, root: root, id: installID, project: cfProject(b)}
	cfBindSpec(b, root, s.project)
	if base {
		s.prompt("tidy " + cfGraph)
		if _, err := os.Stat(s.basePath()); err != nil {
			b.Fatalf("the prompt left no conflict base: %v", err)
		}
	}
	const body = "package graph\n\nfunc G() {}\n"
	file := filepath.Join(s.project, filepath.FromSlash(cfGraph))
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		b.Fatal(err)
	}
	input := benchJSON(b, struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}{file, body})
	response := []byte(`{"stdout":"","stderr":"","interrupted":false,"isImage":false}`)
	benchToolCallsIn(b, root, installID, postEntry, benchToolCallIn(b, s.project, "PostToolUse", "Write", input, response), false)
	checks := s.run().ConflictChecks
	want := 0
	if base {
		want = b.N + 1
	}
	if len(checks) != want {
		b.Fatalf("%d calls left %d conflict checks, want %d", b.N+1, len(checks), want)
	}
	for _, c := range checks {
		if len(c.Rows) != 0 {
			b.Fatalf("a check raised %+v, want no row", c.Rows)
		}
	}
	if base {
		return
	}
	// Witness: no check is also what a post that never reached the join
	// leaves, so the same store, its prompt now leaving a base, records a
	// check at the next edit.
	s.prompt("tidy " + cfGraph)
	s.land("toolu_cf_witness", cfGraph, body)
	if n := len(s.run().ConflictChecks); n != 1 {
		b.Fatalf("with a base the next edit left %d conflict checks, want 1", n)
	}
}

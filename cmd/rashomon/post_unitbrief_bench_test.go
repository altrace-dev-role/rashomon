package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// benchUnitBriefPost is BenchmarkRunPost's unit-brief rows (W-M2.3 hook
// half, ruling 04:58 C): the post of one landed Edit of planHookProject's
// Go file, its cwd the project, consented and noted as the rows above are,
// in a store that keeps the session's content, at record scope v5. The
// rows share the payload and differ in the store alone: the brief the
// session holds for the file's unit (ubBrief rebound at rev, none for "")
// and whether steering is off. No row ever gives the brief, so every call
// takes one branch of unitBriefJoin: with none held, an absent miss (the
// plain consented post of the payload: an edit in a unit no prompt named);
// at the index's c1 with steering off, the join's gate before the read
// (Post.Unheard), so the row holds a read that moves ahead of the gate,
// about 43 KB and 485 allocations a call before the gate, to its budget; at
// c0, the read, the header scan and a stale miss, the lines never decoded
// (the per-edit miss's price, held by readUnitBrief's header-first order). The
// check after the loop holds each row to its branch, so a fixture that
// drifts to another fails rather than measures it.
func benchUnitBriefPost(b *testing.B, rev string, steerOff bool) {
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
	s := &planSession{t: b, root: root, id: installID, project: planHookProject(b)}
	if rev != "" {
		s.writeBrief(rev)
	}
	cfg := filepath.Join(root, store.FileConfig)
	if steerOff {
		if err := os.WriteFile(cfg, []byte(`{"steer_user":"off"}`), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	file := filepath.Join(s.project, filepath.FromSlash(planHookHeld))
	const oldText, newText = "package x", "package x // edited"
	input := benchJSON(b, struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}{file, oldText, newText, false})
	response := benchJSON(b, struct {
		FilePath     string `json:"filePath"`
		OldString    string `json:"oldString"`
		NewString    string `json:"newString"`
		OriginalFile string `json:"originalFile"`
		UserModified bool   `json:"userModified"`
		ReplaceAll   bool   `json:"replaceAll"`
	}{file, oldText, newText, oldText + "\n", false, false})
	benchToolCallsIn(b, root, installID, postEntry, benchToolCallIn(b, s.project, "PostToolUse", "Edit", input, response), false)
	misses, given := s.misses(), len(s.briefLines())
	want, wantN := "", 0
	switch {
	case rev == "":
		want, wantN = store.UnitBriefAbsent, b.N+1
	case !steerOff:
		want, wantN = store.UnitBriefStale, b.N+1
	}
	if len(misses) != wantN || given != 0 {
		b.Fatalf("%d calls left %d misses and %d task-preflight lines, want %d and 0", b.N+1, len(misses), given, wantN)
	}
	for _, m := range misses {
		if m.Reason != want {
			b.Fatalf("a miss of reason %q, want %q", m.Reason, want)
		}
	}
	if !steerOff {
		return
	}
	// Witness: no miss and no line is also what an early return leaves, so
	// the same store with steering back on gives the brief at the next edit.
	if err := os.Remove(cfg); err != nil {
		b.Fatal(err)
	}
	if out := s.edit("toolu_ub_witness"); !strings.Contains(out, ubPostTitle) {
		b.Fatalf("with steering back on the next edit printed %q, want the brief", out)
	}
}

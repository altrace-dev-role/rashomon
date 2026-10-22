package digest

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

func decl(seq int64, id, tool, promptID, transcript string, ms int64) store.Declaration {
	d := store.Declaration{
		Seq: seq, ToolUseID: id, ToolName: tool, SessionID: "s1",
		TranscriptPath: transcript, RecordedAtMS: ms,
	}
	if promptID != "" {
		p := promptID
		d.PromptID = &p
	}
	return d
}

func subagentDecl(seq int64, id, tool, promptID, transcript, agentID string, ms int64) store.Declaration {
	d := decl(seq, id, tool, promptID, transcript, ms)
	a := agentID
	d.AgentID = &a
	return d
}

// TestSelectTurn_GroupsByPromptIDAloneAcrossTranscripts is H-89's unit-level
// guard: a subagent's declarations carry the SAME prompt_id as the parent
// turn under a DIFFERENT transcript_path. Keying on (transcript, prompt_id)
// -- the way chains.go groups for the causal view -- would silently split
// this into two groups and undercount the turn.
func TestSelectTurn_GroupsByPromptIDAloneAcrossTranscripts(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl(1, "m1", "Bash", "prompt-1", "/main.jsonl", 100),
		subagentDecl(2, "a1", "Bash", "prompt-1", "/main.jsonl/subagents/agent-1.jsonl", "agent-1", 101),
		subagentDecl(3, "a2", "Read", "prompt-1", "/main.jsonl/subagents/agent-1.jsonl", "agent-1", 102),
	}}

	w := selectTurn(run, "prompt-1")
	if !w.known {
		t.Fatal("window not known for a prompt with declarations")
	}
	if len(w.declarations) != 3 {
		t.Fatalf("got %d declarations, want 3 (main + 2 subagent, same prompt_id, different "+
			"transcript_path): %+v", len(w.declarations), w.declarations)
	}
}

// TestSelectTurn_BreakOnTranscriptKeyUndercounts demonstrates the break H-89
// names directly: grouping by (transcript_path, prompt_id) instead of
// prompt_id alone loses the subagent's calls into a second bucket.
func TestSelectTurn_BreakOnTranscriptKeyUndercounts(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl(1, "m1", "Bash", "prompt-1", "/main.jsonl", 100),
		subagentDecl(2, "a1", "Bash", "prompt-1", "/main.jsonl/subagents/agent-1.jsonl", "agent-1", 101),
	}}

	// The broken grouping the spec's literal text would produce.
	type key struct{ transcript, prompt string }
	byKey := map[key]int{}
	for _, d := range run.Declarations {
		byKey[key{d.TranscriptPath, *d.PromptID}]++
	}
	if len(byKey) != 2 {
		t.Fatalf("premise: (transcript, prompt) grouping should split these into 2 buckets, got %d", len(byKey))
	}
	mainBucket := byKey[key{"/main.jsonl", "prompt-1"}]
	if mainBucket != 1 {
		t.Fatalf("the (transcript, prompt) grouping counts %d for the main bucket, "+
			"want 1 -- demonstrating it drops the subagent call selectTurn correctly keeps", mainBucket)
	}
}

// TestSelectTurn_DefaultsToTheLatestStartingTurn covers "current turn" with
// no --prompt given: the group whose earliest declaration started last.
func TestSelectTurn_DefaultsToTheLatestStartingTurn(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100),
		decl(2, "b1", "Bash", "prompt-2", "/t.jsonl", 200),
	}}

	w := selectTurn(run, "")
	if w.promptID != "prompt-2" {
		t.Errorf("promptID = %q, want prompt-2 (the later-starting turn)", w.promptID)
	}
}

// TestSelectTurn_WindowEndsWhereTheNextTurnStarts is the boundary a coverage
// record or a dropped terminal is joined against.
func TestSelectTurn_WindowEndsWhereTheNextTurnStarts(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100),
		decl(2, "b1", "Bash", "prompt-2", "/t.jsonl", 500),
	}}

	w := selectTurn(run, "prompt-1")
	if w.startMS != 100 {
		t.Errorf("startMS = %d, want 100", w.startMS)
	}
	if w.endMS != 500 {
		t.Errorf("endMS = %d, want 500 (where prompt-2 starts)", w.endMS)
	}
	if w.contains(500) {
		t.Error("the window is half-open; 500 belongs to the NEXT turn")
	}
	if !w.contains(499) {
		t.Error("499 is still inside prompt-1's window")
	}
}

// TestSelectTurn_LatestTurnWindowIsOpen: the current turn's window has no
// upper bound, since nothing yet says when it ends.
func TestSelectTurn_LatestTurnWindowIsOpen(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100),
	}}
	w := selectTurn(run, "prompt-1")
	if !w.contains(1 << 40) {
		t.Error("the open turn's window should contain any time far in the future")
	}
}

// TestSelectTurn_UnknownPromptHasNoWindow is the case Digest.Unknown depends
// on: a prompt_id that named nothing in this run has no basis for a window.
func TestSelectTurn_UnknownPromptHasNoWindow(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100),
	}}
	w := selectTurn(run, "prompt-404")
	if w.known {
		t.Error("known = true for a prompt_id with zero declarations")
	}
	if len(w.declarations) != 0 {
		t.Errorf("declarations = %+v, want none", w.declarations)
	}
}

// TestSelectTurn_EmptyRunHasNoDefaultTurn covers a session with no prompt_id
// anywhere -- a fresh session, or one recorded before v2.
func TestSelectTurn_EmptyRunHasNoDefaultTurn(t *testing.T) {
	w := selectTurn(&store.Run{}, "")
	if w.known {
		t.Error("known = true on an empty run")
	}
}

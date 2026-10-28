package digest

import (
	"fmt"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

func decl(seq int64, id, tool, promptID, transcript string, ms int64) store.Declaration {
	d := store.Declaration{
		Seq: seq, ToolUseID: id, ToolName: tool, SessionID: "s1",
		TranscriptPath: transcript, RecordedAtMS: ms,
		// One known directory: the readers pair runs only where the location
		// is known, and an empty one is not.
		CWDDigest: "w",
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

// TestTURN_SelectTurnCopiesOnlyTheChosenTurn (card 2.3c R3, R4): selectTurn
// copies the chosen turn's declarations once, into a slice of that turn's
// size, not every prompt's into slices that grow. The run is built in memory
// and out of seq order, which no read run is (the reader yields declarations
// in seq order), so that it also separates the minSeq update and the sort: b
// is listed first at seq 1000 and holds seqs 1 to 199 after it, c and d are in
// order, and a is listed from seq 799 down to 600. With the update the minSeqs
// are b 1, c 200, d 400 and a 600, and a, the highest, is the current turn;
// without it b's stays 1000 and b is chosen. a's declarations arrive
// descending, so only the sort makes them ascend.
//
// The ceiling is the two-pass shape's count on go1.27.3: four group structs
// (stored in the map, so they escape), order's three growth steps (1, 2, 4),
// sort.Slice's box of order and its Swapper's string closure, the presized
// slice, and sort.SliceStable's box, scratch element and Swapper closure: 13,
// as measured green, plain and under -race.
//
// The one-pass shape it replaced, measured red on the same run, spent 48: the
// same less the presized slice, plus nine growth steps (1 to 276) for each
// prompt's slice of 200.
func TestTURN_SelectTurnCopiesOnlyTheChosenTurn(t *testing.T) {
	const ceiling = 13
	var decls []store.Declaration
	add := func(prompt string, seq int64) {
		decls = append(decls, decl(seq, fmt.Sprint("toolu_", seq), "Bash", prompt, "/t.jsonl", 10*seq))
	}
	add("b", 1000)
	for seq := int64(1); seq < 200; seq++ {
		add("b", seq)
	}
	for seq := int64(200); seq < 400; seq++ {
		add("c", seq)
	}
	for seq := int64(400); seq < 600; seq++ {
		add("d", seq)
	}
	for seq := int64(799); seq >= 600; seq-- {
		add("a", seq)
	}
	run := &store.Run{Declarations: decls}

	w := selectTurn(run, "")
	if w.promptID != "a" || len(w.declarations) != 200 {
		t.Fatalf("selectTurn chose %q with %d declarations, want a with 200", w.promptID, len(w.declarations))
	}
	for i := 1; i < len(w.declarations); i++ {
		if prev, seq := w.declarations[i-1].Seq, w.declarations[i].Seq; prev >= seq {
			t.Fatalf("declarations %d and %d are seqs %d then %d, want ascending", i-1, i, prev, seq)
		}
	}
	n := testing.AllocsPerRun(10, func() { selectTurn(run, "") })
	if n > ceiling {
		t.Errorf("selectTurn allocates %v on four prompts of 200, want at most %d", n, ceiling)
	}
	t.Logf("selectTurn allocates %v on four prompts of 200", n)
}

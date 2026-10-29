package steer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// logLine is a steer line of session s1 as the writer stores it.
func logLine(t *testing.T, prompt *string, at int64, state LineState, level Level, pat, key string) []byte {
	t.Helper()
	b, err := json.Marshal(Line{
		Type: LineType, V: LineVersion, SessionID: "s1", PromptID: prompt, AtUnixMS: at,
		Pattern: pat, Key: key, Level: level.String(), State: state,
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

func countOf(t *testing.T, log []byte, prompt *string) counts {
	t.Helper()
	c, err := countBudget(bytes.NewReader(log), &Context{SessionID: "s1", PromptID: prompt})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func ptr(s string) *string { return &s }

// TestST7d_CountsIgnoreOtherPromptsAndUnconfirmedLines: another prompt's
// three delivered notes, an unconfirmed note of this prompt and a
// subagent's delivered note of this prompt: one note counts, the
// subagent's (tech 4.2.5: "subagents included"), and only delivered
// lines count (4.2.3: unconfirmed "counts in no ... budget").
func TestST7d_CountsIgnoreOtherPromptsAndUnconfirmedLines(t *testing.T) {
	p1, p2 := ptr("p1"), ptr("p2")
	var log []byte
	for i := range 3 {
		log = append(log, logLine(t, p1, int64(10+i), LineStateDelivered, LevelNudge, "regression-location", fmt.Sprint("k", i))...)
	}
	log = append(log, logLine(t, p2, 20, LineStateUnconfirmed, LevelNudge, "regression-location", "u")...)
	sub, err := json.Marshal(Line{SessionID: "s1", PromptID: p2, AgentID: ptr("agent-1"), AtUnixMS: 21,
		Pattern: "subtask-gate", Key: "task:1", Level: "nudge", State: LineStateDelivered})
	if err != nil {
		t.Fatal(err)
	}
	log = append(log, append(sub, '\n')...)
	c := countOf(t, log, p2)
	if c.notes != 1 || c.flags != 0 || !c.keys[findingKey{"subtask-gate", "", "task:1"}] || len(c.keys) != 1 {
		t.Errorf("counts = %+v, want the subagent's one note and its key", c)
	}
}

// TestST7e_ALateForeignBatchAtTheTailIsSkipped (A4): P2 delivered three
// notes, then a late append of the previous prompt's Stop recap wrote two
// lines; P2's next call still counts three, because one append's lines
// share one at_unix_ms and the scan skips one such foreign batch at the
// tail. A second foreign line past it ends the prompt, and a second
// straddling batch is the residual R6 states: it hides the prompt.
func TestST7e_ALateForeignBatchAtTheTailIsSkipped(t *testing.T) {
	p1, p2 := ptr("p1"), ptr("p2")
	var log []byte
	log = append(log, logLine(t, p1, 5, LineStateDelivered, LevelNudge, "regression-location", "old")...)
	for i := range 3 {
		log = append(log, logLine(t, p2, int64(10+i), LineStateDelivered, LevelNudge, "regression-location", fmt.Sprint("k", i))...)
	}
	late := append(logLine(t, p1, 30, LineStateCandidate, LevelFlag, "integrity", "a"),
		logLine(t, p1, 30, LineStateDelivered, LevelFlag, "integrity", "b")...)
	if c := countOf(t, append(append([]byte(nil), log...), late...), p2); c.notes != 3 {
		t.Errorf("with one late batch at the tail, notes = %d, want 3", c.notes)
	}
	if c := countOf(t, log, p2); c.notes != 3 {
		t.Errorf("with no late batch, notes = %d, want 3", c.notes)
	}
	// The previous prompt's line before P2's first is the boundary: P1's
	// delivered note is not counted.
	if c := countOf(t, log, p2); c.keys[findingKey{"regression-location", "", "old"}] {
		t.Error("the previous prompt's key was counted")
	}
	// A first call in a new prompt: the tail is all another prompt's lines,
	// and nothing counts.
	if c := countOf(t, log, ptr("p3")); c.notes != 0 || len(c.keys) != 0 {
		t.Errorf("a new prompt counts %+v, want nothing", c)
	}
	// The residual: two straddling batches hide the prompt behind them.
	twice := append(append(append([]byte(nil), log...), late...), logLine(t, p1, 31, LineStateDelivered, LevelFlag, "integrity", "c")...)
	if c := countOf(t, twice, p2); c.notes != 0 {
		t.Errorf("behind two foreign batches, notes = %d; the residual R6 states is an undercount, so 0", c.notes)
	}
}

// TestST_CountsReadEveryLineOfThePrompt: flags and notes are counted apart,
// a torn tail and a damaged line are skipped (ReadSteer's rule), a null
// prompt matches only null, and a prompt longer than one 64 KiB chunk is
// read to its first line, a line straddling a chunk boundary included.
func TestST_CountsReadEveryLineOfThePrompt(t *testing.T) {
	p := ptr("p1")
	var log []byte
	log = append(log, logLine(t, ptr("p0"), 1, LineStateDelivered, LevelNudge, "regression-location", "before")...)
	log = append(log, logLine(t, p, 2, LineStateDelivered, LevelNudge, "regression-location", "first")...)
	pad := strings.Repeat("x", 700)
	for i := range 300 {
		log = append(log, logLine(t, p, int64(3+i), LineStateSuppressed, LevelNudge, "regression-location", pad)...)
	}
	log = append(log, logLine(t, p, 400, LineStateDelivered, LevelFlag, "integrity", "f1")...)
	log = append(log, []byte("{\"session_id\":\"s1\",\"prompt_id\":\"p1\",\"state\":\"deliv\n")...)
	log = append(log, logLine(t, p, 401, LineStateDelivered, LevelStop, "subtask-gate", "s")...)
	log = append(log, []byte(`{"session_id":"s1","prompt_id":"p1","state":"delivered","level":"nudge"`)...)
	if len(log) < 3*budgetChunk {
		t.Fatalf("the log is %d bytes, want it past three chunks", len(log))
	}
	c := countOf(t, log, p)
	if c.notes != 2 || c.flags != 1 {
		t.Errorf("counts = %+v, want 2 notes (the first line's and the stop) and 1 flag", c)
	}
	if !c.keys[findingKey{"regression-location", "", "first"}] || c.keys[findingKey{"regression-location", "", "before"}] {
		t.Errorf("keys = %v, want the prompt's first line in and the line before it out", c.keys)
	}
	for shift := range 64 {
		shifted := append(bytes.Repeat([]byte("\n"), shift), log...)
		if got := countOf(t, shifted, p); got.notes != 2 || got.flags != 1 {
			t.Fatalf("shifted by %d bytes, counts = %+v", shift, got)
		}
	}
	// The prompt's first line is the file's first line: read too.
	first := bytes.IndexByte(log, '\n') + 1
	if got := countOf(t, log[first:], p); !got.keys[findingKey{"regression-location", "", "first"}] {
		t.Errorf("with the prompt's first line first in the file, keys = %v, want it read", got.keys)
	}
	none := append(logLine(t, p, 1, LineStateDelivered, LevelFlag, "integrity", "x"),
		logLine(t, nil, 2, LineStateDelivered, LevelFlag, "integrity", "y")...)
	if got := countOf(t, none, nil); got.flags != 1 {
		t.Errorf("a null prompt counts %d flags, want its own 1", got.flags)
	}
	if got := countOf(t, nil, p); got.notes != 0 || got.flags != 0 {
		t.Errorf("an empty log counts %+v", got)
	}
}

// TestST_KeysCompareUnescaped: json.Marshal writes & < > as \u escapes, so a
// key carrying one is compared after unescaping, never as raw bytes.
func TestST_KeysCompareUnescaped(t *testing.T) {
	p := ptr("p&<1>")
	log := logLine(t, p, 1, LineStateDelivered, LevelNudge, "platform-rule", "rule|a&b<c>.go")
	if !bytes.Contains(log, []byte(`\u0026`)) {
		t.Fatalf("the writer did not escape &: %s", log)
	}
	c := countOf(t, log, p)
	if c.notes != 1 || !c.keys[findingKey{"platform-rule", "", "rule|a&b<c>.go"}] {
		t.Errorf("counts = %+v, want the escaped prompt matched and the key unescaped", c)
	}
}

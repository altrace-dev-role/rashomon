package report

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// A transcript line can be longer than TranscriptMaxLine -- a prompt with a
// few hundred pasted images measured 178 MiB -- and the lines after it are
// as much the session as the lines before. Every reader here steps over such
// a line and reads on, and each says what not reading it costs.

var (
	longLineOnce sync.Once
	longLineText string
)

// longLine is a user line one byte over the bound: its content and newline
// come to TranscriptMaxLine + 1 bytes.
func longLine() string {
	longLineOnce.Do(func() {
		head, tail := `{"type":"user","message":{"role":"user","content":"`, `"}}`
		longLineText = head + strings.Repeat("x", TranscriptMaxLine-len(head)-len(tail)) + tail
	})
	return longLineText
}

func writeLines(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const (
	useA    = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_a"}]}}`
	useB    = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_b"}]}}`
	resultB = `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_b","content":"ok"}]}}`
)

func TestRubricPERF6_LongLineTranscriptIDsStepsOverAndCounts(t *testing.T) {
	if n := len(longLine()) + 1; n != TranscriptMaxLine+1 {
		t.Fatalf("premise: the long line and its newline are %d bytes, want %d", n, TranscriptMaxLine+1)
	}
	path := writeLines(t, useA, longLine(), useB, resultB)
	ids, results, _, _, files, tooLong, err := transcriptIDsAt(path)
	if err != nil {
		t.Fatalf("TranscriptIDs: %v", err)
	}
	if !ids["toolu_a"] || !ids["toolu_b"] || !results["toolu_b"] || files != 1 || tooLong != 1 {
		t.Errorf("ids %v, results %v, files %d, too long %d; want both ids, toolu_b's result, 1 file and 1 line too long",
			ids, results, files, tooLong)
	}
}

// TestRubricPERF6_LongLineUnverifiesTheTranscript: every id and result
// matches the store, but a line was not read and may hold a call the store
// does not, so the group counts it and the session adds transcript_mismatch.
func TestRubricPERF6_LongLineUnverifiesTheTranscript(t *testing.T) {
	path := writeLines(t, useA, longLine(), useB, resultB)
	sess := buildFromDisk(&store.Run{
		Declarations: []store.Declaration{
			{ToolUseID: "toolu_a", ToolName: "Bash", SessionID: "s", TranscriptPath: path},
			{ToolUseID: "toolu_b", ToolName: "Bash", SessionID: "s", TranscriptPath: path},
		},
		Executions: []store.Execution{{ToolUseID: "toolu_b", SessionID: "s"}},
	})
	tr := groupFor(t, sess, path)
	if !tr.Readable || tr.LinesTooLong == nil || *tr.LinesTooLong != 1 {
		t.Fatalf("readable %v, lines too long %v; want a readable transcript with 1", tr.Readable, tr.LinesTooLong)
	}
	if len(tr.MissingFromStore)+len(tr.MissingFromTranscript)+len(tr.ExecutedButUnrecorded) != 0 {
		t.Fatalf("premise: the ids do not all match: %+v", tr)
	}
	if !slices.Contains(sess.Coverage.Reasons, ReasonTranscriptMismatch) {
		t.Errorf("coverage reasons %v: a transcript with a line not read reads as matching the store", sess.Coverage.Reasons)
	}
}

func TestRubricPERF6_LongLineFinalAssistantTextReadsOn(t *testing.T) {
	first := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}`
	last := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"last"}]}}`
	if got, ok := FinalAssistantText(openT(t, writeLines(t, first, longLine(), last))); !ok || got != "last" {
		t.Errorf("text after a line too long: %q, %v; want \"last\"", got, ok)
	}
	// The line not read may be a later reply, so nothing before it is the
	// last word.
	if got, ok := FinalAssistantText(openT(t, writeLines(t, first, longLine()))); ok {
		t.Errorf("text before a final line too long: %q, %v; want none", got, ok)
	}
}

// TestRubricPERF6_LongLineFinalAssistantTextsEndsTheTie: a line not read
// may be the next prompt, or the turn's last words, so it is taken as an
// undecodable line is: the turn's words so far are dropped and the tie ends.
// The prompts after it are read.
func TestRubricPERF6_LongLineFinalAssistantTextsEndsTheTie(t *testing.T) {
	user := func(p string) string {
		return `{"type":"user","promptId":"` + p + `","message":{"role":"user","content":"go"}}`
	}
	say := func(text string) string {
		return `{"type":"assistant","timestamp":"2026-10-13T10:00:00Z","message":{"id":"msg_` + text +
			`","role":"assistant","content":[{"type":"text","text":"` + text + `"}]}}`
	}
	path := writeLines(t, user("p1"), say("one"), longLine(), say("stray"), user("p2"), say("two"))
	got := FinalAssistantTexts(openT(t, path), map[string]bool{"p1": true, "p2": true})
	if got["p1"].Said {
		t.Errorf("p1 kept %q across a line too long", got["p1"].Text)
	}
	if tf := got["p2"]; !tf.Said || tf.Text != "two" {
		t.Errorf("p2 = %+v; want its words \"two\": the turns after a line too long are read", tf)
	}
}

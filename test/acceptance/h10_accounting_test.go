package acceptance

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeTranscript builds a fixture in the shape Claude Code writes: one JSON
// object per line, tool_use blocks inside assistant messages, subagent
// transcripts beside the main one. The tool inputs carry a canary so that a
// reader that persisted anything beyond ids would be caught by H-13's sweep.
func writeTranscript(t *testing.T, dir, sessionID string, ids []string, subagents map[string][]string) string {
	t.Helper()
	line := func(id string) string {
		return fmt.Sprintf(`{"type":"assistant","uuid":"u-%s","message":{"role":"assistant","content":[{"type":"text","text":"Running it."},{"type":"tool_use","id":%q,"name":"Bash","input":{"command":"echo TRANSCRIPT-CANARY-%s"}}]}}`, id, id, id)
	}
	result := func(id string) string {
		return fmt.Sprintf(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]}}`, id)
	}
	body := `{"type":"summary","summary":"a session"}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"plain string content"}}` + "\n"
	for _, id := range ids {
		body += line(id) + "\n" + result(id) + "\n"
	}
	body += "this line is not JSON and must be skipped\n"

	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, subIDs := range subagents {
		sub := ""
		for _, id := range subIDs {
			sub += line(id) + "\n"
		}
		subDir := filepath.Join(dir, sessionID, "subagents")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "agent-"+name+".jsonl"), []byte(sub), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// hookIDs runs the hook once per id against a transcript path.
func (e *env) hookIDs(transcript string, ids ...string) {
	e.t.Helper()
	for _, id := range ids {
		p := defaultPayload()
		p.ToolUseID = id
		p.TranscriptPath = transcript
		e.mustHook(p.build(e.t))
	}
}

// TestH10_AccountingEquationHolds is the whole point: recorded id set equals
// the distinct id set parsed from the transcript and its subagent files.
func TestH10_AccountingEquationHolds(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	transcript := writeTranscript(t, t.TempDir(), testSession,
		[]string{"toolu_a", "toolu_b", "toolu_c"},
		map[string][]string{"explore": {"toolu_d"}, "plan": {"toolu_e", "toolu_a"}})
	e.hookIDs(transcript, "toolu_a", "toolu_b", "toolu_c", "toolu_d", "toolu_e")
	e.probe("end", testSession)

	rep := e.report(testSession)
	tr := rep.Transcript
	if tr == nil || !tr.Readable {
		t.Fatalf("transcript was not read: %+v", tr)
	}
	if tr.Files == nil || *tr.Files != 3 {
		t.Errorf("read %v transcript files, want 3 (main + 2 subagents)", tr.Files)
	}
	if tr.IDsInTranscript == nil || *tr.IDsInTranscript != 5 {
		t.Errorf("ids_in_transcript is %v, want 5 distinct (toolu_a appears twice)", tr.IDsInTranscript)
	}
	if tr.IDsRecorded != 5 {
		t.Errorf("ids_recorded is %d, want 5", tr.IDsRecorded)
	}
	if len(tr.MissingFromStore) != 0 || len(tr.MissingFromTranscript) != 0 {
		t.Errorf("sets differ: missing from store %v, missing from transcript %v", tr.MissingFromStore, tr.MissingFromTranscript)
	}
	if rep.Coverage.State != "verified" {
		t.Errorf("coverage is %s (%v), want verified", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH10_MissingDeclarationIsNamed: one id the transcript has and the store
// does not is one dropped declaration, and it is named.
func TestH10_MissingDeclarationIsNamed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	transcript := writeTranscript(t, t.TempDir(), testSession,
		[]string{"toolu_a", "toolu_b", "toolu_c"}, map[string][]string{"sub": {"toolu_d"}})
	e.hookIDs(transcript, "toolu_a", "toolu_b", "toolu_c") // toolu_d never recorded
	e.probe("end", testSession)

	rep := e.report(testSession)
	if !reflect.DeepEqual(rep.Transcript.MissingFromStore, []string{"toolu_d"}) {
		t.Errorf("missing_from_store is %v, want [toolu_d]", rep.Transcript.MissingFromStore)
	}
	if !e.hasReason(rep, "transcript_mismatch") || rep.Coverage.State != "unverified" {
		t.Errorf("coverage is %s (%v), want unverified with transcript_mismatch", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH10_SetsNotCounts is the v2 defect made concrete. A handler that records
// one call in fifty passes "one entry per recorded id"; so does one that
// records the right number of the wrong ids. Only set equality catches both.
func TestH10_SetsNotCounts(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	transcript := writeTranscript(t, t.TempDir(), testSession, []string{"toolu_a", "toolu_b", "toolu_c", "toolu_d"}, nil)
	e.hookIDs(transcript, "toolu_a", "toolu_b", "toolu_c", "toolu_zz") // same count, different set
	e.probe("end", testSession)

	rep := e.report(testSession)
	if rep.Transcript.IDsRecorded != *rep.Transcript.IDsInTranscript {
		t.Fatalf("premise broken: counts differ (%d vs %d)", rep.Transcript.IDsRecorded, *rep.Transcript.IDsInTranscript)
	}
	if !reflect.DeepEqual(rep.Transcript.MissingFromStore, []string{"toolu_d"}) {
		t.Errorf("missing_from_store is %v, want [toolu_d]", rep.Transcript.MissingFromStore)
	}
	if !reflect.DeepEqual(rep.Transcript.MissingFromTranscript, []string{"toolu_zz"}) {
		t.Errorf("missing_from_transcript is %v, want [toolu_zz]", rep.Transcript.MissingFromTranscript)
	}
	if !e.hasReason(rep, "transcript_mismatch") {
		t.Errorf("equal counts with different sets was not reported as a mismatch: %v", rep.Coverage.Reasons)
	}
}

// TestH10_UnreadableTranscriptIsNotZero: could not read and read zero are
// different facts, and only one of them is a count.
func TestH10_UnreadableTranscriptIsNotZero(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.hookIDs(filepath.Join(t.TempDir(), "never-written.jsonl"), "toolu_a")
	e.probe("end", testSession)

	rep := e.report(testSession)
	tr := rep.Transcript
	if tr == nil {
		t.Fatal("transcript section is absent although a path was recorded")
	}
	if tr.Readable {
		t.Fatal("a missing transcript reads as readable")
	}
	if tr.IDsInTranscript != nil || tr.Files != nil {
		t.Errorf("an unreadable transcript rendered counts: ids=%v files=%v; unknown must be null", tr.IDsInTranscript, tr.Files)
	}
	if tr.MissingFromStore != nil || tr.MissingFromTranscript != nil {
		t.Errorf("an unreadable transcript rendered set differences: %v / %v", tr.MissingFromStore, tr.MissingFromTranscript)
	}
	if e.hasReason(rep, "transcript_mismatch") {
		t.Errorf("an unreadable transcript was reported as a mismatch")
	}
}

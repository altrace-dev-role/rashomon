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
	if len(rep.Transcripts) != 1 {
		t.Fatalf("got %d accounting groups, want 1: %+v", len(rep.Transcripts), rep.Transcripts)
	}
	tr := rep.Transcripts[0]
	if !tr.Readable {
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
	if len(rep.Transcripts) != 1 {
		t.Fatalf("got %d accounting groups, want 1: %+v", len(rep.Transcripts), rep.Transcripts)
	}
	if !reflect.DeepEqual(rep.Transcripts[0].MissingFromStore, []string{"toolu_d"}) {
		t.Errorf("missing_from_store is %v, want [toolu_d]", rep.Transcripts[0].MissingFromStore)
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
	if len(rep.Transcripts) != 1 {
		t.Fatalf("got %d accounting groups, want 1: %+v", len(rep.Transcripts), rep.Transcripts)
	}
	tr := rep.Transcripts[0]
	if tr.IDsInTranscript == nil || tr.IDsRecorded != *tr.IDsInTranscript {
		t.Fatalf("premise broken: counts differ (%d vs %v)", tr.IDsRecorded, tr.IDsInTranscript)
	}
	if !reflect.DeepEqual(tr.MissingFromStore, []string{"toolu_d"}) {
		t.Errorf("missing_from_store is %v, want [toolu_d]", tr.MissingFromStore)
	}
	if !reflect.DeepEqual(tr.MissingFromTranscript, []string{"toolu_zz"}) {
		t.Errorf("missing_from_transcript is %v, want [toolu_zz]", tr.MissingFromTranscript)
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
	if len(rep.Transcripts) != 1 {
		t.Fatalf("got %d accounting groups although one path was recorded: %+v", len(rep.Transcripts), rep.Transcripts)
	}
	tr := rep.Transcripts[0]
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

// twoTranscripts writes what a nested `claude -p` leaves behind: the parent's
// transcript with a subagent file beside it, and the spawned run's own
// transcript. Both are named by declarations carrying the one session id,
// because the nested run inherits it. They are named so that the path order
// the report sorts by is the order they are returned in.
func twoTranscripts(t *testing.T) (parent, spawned string) {
	t.Helper()
	dir := t.TempDir()
	parent = writeTranscript(t, dir, "parent", []string{"toolu_a", "toolu_b"},
		map[string][]string{"explore": {"toolu_c"}})
	spawned = writeTranscript(t, dir, "spawned", []string{"toolu_d", "toolu_e"}, nil)
	return parent, spawned
}

// TestH10_PerTranscriptAccounting: one run, two transcripts, two equations.
// Held against a single transcript the same records read as a mismatch in both
// directions, and the run would render unverified for nothing.
func TestH10_PerTranscriptAccounting(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	parent, spawned := twoTranscripts(t)
	e.hookIDs(parent, "toolu_a", "toolu_b", "toolu_c")
	e.hookIDs(spawned, "toolu_d", "toolu_e")
	e.probe("end", testSession)

	rep := e.report(testSession)
	want := []struct {
		path  string
		files int
		ids   int
	}{
		{parent, 2, 3},
		{spawned, 1, 2},
	}
	if len(rep.Transcripts) != len(want) {
		t.Fatalf("got %d accounting groups, want %d: %+v", len(rep.Transcripts), len(want), rep.Transcripts)
	}
	for i, w := range want {
		tr := rep.Transcripts[i]
		if tr.Path != w.path {
			t.Fatalf("group %d is %s, want %s: groups are sorted by path", i, tr.Path, w.path)
		}
		if !tr.Readable {
			t.Fatalf("%s was not read: %+v", tr.Path, tr)
		}
		if tr.Files == nil || *tr.Files != w.files {
			t.Errorf("%s: read %v files, want %d", tr.Path, tr.Files, w.files)
		}
		if tr.IDsInTranscript == nil || *tr.IDsInTranscript != w.ids {
			t.Errorf("%s: ids_in_transcript is %v, want %d", tr.Path, tr.IDsInTranscript, w.ids)
		}
		if tr.IDsRecorded != w.ids {
			t.Errorf("%s: ids_recorded is %d, want %d", tr.Path, tr.IDsRecorded, w.ids)
		}
		if len(tr.MissingFromStore) != 0 || len(tr.MissingFromTranscript) != 0 {
			t.Errorf("%s: sets differ: missing from store %v, missing from transcript %v",
				tr.Path, tr.MissingFromStore, tr.MissingFromTranscript)
		}
	}
	if rep.Coverage.State != "verified" {
		t.Errorf("coverage is %s (%v), want verified", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH10_PerTranscriptMismatchStaysInItsGroup: a dropped declaration is named
// against the transcript that holds its id, and leaves the other transcript's
// equation balanced.
func TestH10_PerTranscriptMismatchStaysInItsGroup(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	parent, spawned := twoTranscripts(t)
	e.hookIDs(parent, "toolu_a", "toolu_b", "toolu_c")
	e.hookIDs(spawned, "toolu_d") // toolu_e never recorded
	e.probe("end", testSession)

	rep := e.report(testSession)
	if len(rep.Transcripts) != 2 {
		t.Fatalf("got %d accounting groups, want 2: %+v", len(rep.Transcripts), rep.Transcripts)
	}
	first, second := rep.Transcripts[0], rep.Transcripts[1]
	if first.Path != parent || second.Path != spawned {
		t.Fatalf("groups are %s then %s, want %s then %s, sorted by path", first.Path, second.Path, parent, spawned)
	}
	if len(first.MissingFromStore) != 0 || len(first.MissingFromTranscript) != 0 {
		t.Errorf("%s balances, but was reported as missing %v from the store and %v from the transcript",
			first.Path, first.MissingFromStore, first.MissingFromTranscript)
	}
	if !reflect.DeepEqual(second.MissingFromStore, []string{"toolu_e"}) {
		t.Errorf("%s: missing_from_store is %v, want [toolu_e]", second.Path, second.MissingFromStore)
	}
	if len(second.MissingFromTranscript) != 0 {
		t.Errorf("%s: missing_from_transcript is %v, want none", second.Path, second.MissingFromTranscript)
	}
	if !e.hasReason(rep, "transcript_mismatch") || rep.Coverage.State != "unverified" {
		t.Errorf("coverage is %s (%v), want unverified with transcript_mismatch", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH10_DeclarationsWithoutATranscript: a declaration naming no transcript
// joins no equation, because there is nothing to check it against. It is still
// a record this store holds, and that count is real -- zero there means none,
// not unknown.
func TestH10_DeclarationsWithoutATranscript(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	transcript := writeTranscript(t, t.TempDir(), testSession, []string{"toolu_a"}, nil)
	for _, id := range []string{"toolu_x", "toolu_y"} {
		p := defaultPayload()
		p.ToolUseID = id
		p.TranscriptPath = ""
		e.mustHook(p.build(t))
	}
	e.hookIDs(transcript, "toolu_a")
	e.probe("end", testSession)

	rep := e.report(testSession)
	if rep.Declarations.WithoutTranscript != 2 {
		t.Errorf("without_transcript is %d, want 2", rep.Declarations.WithoutTranscript)
	}
	if rep.Declarations.Recorded != 3 {
		t.Errorf("recorded is %d, want 3: the pathless declarations are records too", rep.Declarations.Recorded)
	}
	if len(rep.Transcripts) != 1 || rep.Transcripts[0].Path != transcript {
		t.Fatalf("got %d accounting groups (%+v), want one, for %s", len(rep.Transcripts), rep.Transcripts, transcript)
	}
	tr := rep.Transcripts[0]
	if len(tr.MissingFromStore) != 0 || len(tr.MissingFromTranscript) != 0 {
		t.Errorf("the pathless declarations were held against %s: missing from store %v, missing from transcript %v",
			tr.Path, tr.MissingFromStore, tr.MissingFromTranscript)
	}
	if rep.Coverage.State != "verified" {
		t.Errorf("coverage is %s (%v), want verified", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

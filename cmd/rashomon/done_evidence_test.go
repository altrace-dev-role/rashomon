package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestDONE1n_CoverageNoticeBetweenLeavesTheDigest: the done flag is decided
// before the coverage notice quiets the bare incomplete line, and the
// notice's markers are no input to the digest: with the notice standing the
// flag prints once and returns the evidence's digest; once that digest is
// shown, the line falls through as an unflagged one and returns no digest.
func TestDONE1n_CoverageNoticeBetweenLeavesTheDigest(t *testing.T) {
	v := platformView(nil)
	s := stopTurn{view: v, speaks: true, summary: true, stops: true, stands: true, notice: "notice", marker: "marker"}
	flag, digest := s.doneFlag("sess-1", false)
	if !strings.Contains(flag, "done was claimed") || digest != verify.DoneEvidence(v) {
		t.Fatalf("doneFlag = %q, %q; want the flag and the view's digest %q", flag, digest, verify.DoneEvidence(v))
	}
	line, ok, _, done := s.line("sess-1", false, flag, digest)
	if !ok || line != flag || done != digest {
		t.Errorf("with nothing shown, line = %q, %v, done %q; want the flag and %q", line, ok, done, digest)
	}
	s.doneShown = digest
	line, _, _, done = s.line("sess-1", false, flag, digest)
	if strings.Contains(line, "done was claimed") || done != "" {
		t.Errorf("with the digest shown, line = %q, done %q; want no flag and no digest", line, done)
	}
}

// doneMarksOf is recap.json's done-evidence entry for session under root.
func doneMarksOf(t *testing.T, root, session string) recap.DoneMarks {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, recap.StateFileName))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Done map[string]recap.DoneMarks `json:"done_evidence"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("%s: %v", recap.StateFileName, err)
	}
	return s.Done[session]
}

// TestDONE1q_SupersededNoteLeavesNoDigest: a prompt whose done note a
// resume briefing outranks takes the flag but leaves the digest
// undelivered, so the next flagged Stop over the same evidence delivers the
// note.
func TestDONE1q_SupersededNoteLeavesNoDigest(t *testing.T) {
	e := newLessonEnv(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-279_harness")
	standInRead(t)
	const session = "sess-done1q"
	e.start(session)
	tr := compactTranscript(t, filepath.Join(t.TempDir(), session+".jsonl"), "b-cmd-1")
	e.compactStart(session, tr)
	t.Setenv("AI_AGENT", "claude-code_2-1-316_harness")
	e.startAt(session, tr, "resume")
	if !e.armed(session) {
		t.Fatal("premise: the resume start did not arm the briefing")
	}
	now := time.Now()
	recap.ClaimStop(e.root, session, "p0", now, true, "", recap.Owed{}, recap.DoneMarks{Flagged: "d1"}, true)

	p := e.promptAt(session, "p1", "carry on", tr)
	if !strings.HasPrefix(p.context, "rashomon record: the person's words") {
		t.Fatalf("premise: the first prompt after the resume carried %q, want the briefing", p.context)
	}
	if m := doneMarksOf(t, e.root, session); m.Flagged != "d1" || m.Delivered != "" {
		t.Errorf("done marks after the superseded note = %+v; want d1 flagged and nothing delivered", m)
	}

	recap.ClaimStop(e.root, session, "p1", now, true, "", recap.Owed{}, recap.DoneMarks{Flagged: "d1"}, true)
	f, ok := doneFinding(0)
	want := f.Message.Text()
	if p := e.promptAt(session, "p2", "and then", tr); !ok || want == "" || p.context != want {
		t.Errorf("the prompt after the second flagged Stop carried %q; want the done note %q", p.context, want)
	}
	if m := doneMarksOf(t, e.root, session); m.Delivered != "d1" {
		t.Errorf("done marks after the note = %+v; want d1 delivered", m)
	}
}

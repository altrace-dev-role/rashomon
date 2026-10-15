package report

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// sessionNaming builds a closed run whose one declaration names transcript.
func sessionNaming(transcript string) Session {
	run := &store.Run{
		Declarations: []store.Declaration{{ToolUseID: "toolu_a", ToolName: "Bash", TranscriptPath: transcript}},
		Terminals:    []store.Terminal{{ToolUseID: "toolu_a", Outcome: store.OutcomeOK}},
		Coverage: []store.Coverage{
			{Phase: store.PhaseStart, State: store.StateVerified},
			{Phase: store.PhaseEnd, State: store.StateVerified},
		},
	}
	sess := build(run)
	sess.Coverage.finish()
	return sess
}

// TestRubricERRORS3_UnreadableTranscriptIsAReason: a transcript that exists
// and cannot be read takes the H-10 accounting check away, and the run says
// so with transcript_unreadable instead of rendering verified beside
// "readable: no". Before, the branch for that error was a no-op.
func TestRubricERRORS3_UnreadableTranscriptIsAReason(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 does not stop this user reading the file")
	}
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) //nolint:errcheck // TempDir removal follows

	sess := sessionNaming(path)
	if !slices.Contains(sess.Coverage.Reasons, ReasonTranscriptUnreadable) || sess.Coverage.State != store.StateUnverified {
		t.Errorf("coverage = %s %v, want unverified with %s", sess.Coverage.State, sess.Coverage.Reasons, ReasonTranscriptUnreadable)
	}
	if len(sess.Transcripts) != 1 || sess.Transcripts[0].Readable {
		t.Errorf("transcripts = %+v, want one, unreadable", sess.Transcripts)
	}
	if got := sess.Transcripts[0].Status; got != TranscriptUnreadable {
		t.Errorf("transcript status = %q, want %q", got, TranscriptUnreadable)
	}
}

// TestRubricERRORS3_MainTranscriptReadableButASubagentIsNot: the main
// transcript exists and reads fine; one of its subagent files is a dangling
// symlink. TranscriptIDs returns ErrNotExist -- the subagent file's error,
// not the main file's -- and before, accounting classified on that error
// alone, so the whole group read as missing and the H-10 check it names was
// silently skipped. It must classify on the main transcript's own Stat: the
// main file is there, so any later error, this one included, is
// transcript_unreadable.
func TestRubricERRORS3_MainTranscriptReadableButASubagentIsNot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subagents := filepath.Join(dir, "sess", "subagents")
	if err := os.MkdirAll(subagents, 0o755); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(subagents, "agent-1.jsonl")
	if err := os.Symlink(filepath.Join(subagents, "nonexistent-target"), dangling); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	sess := sessionNaming(path)
	if !slices.Contains(sess.Coverage.Reasons, ReasonTranscriptUnreadable) || sess.Coverage.State != store.StateUnverified {
		t.Errorf("coverage = %s %v, want unverified with %s", sess.Coverage.State, sess.Coverage.Reasons, ReasonTranscriptUnreadable)
	}
	if len(sess.Transcripts) != 1 || sess.Transcripts[0].Readable {
		t.Errorf("transcripts = %+v, want one, unreadable", sess.Transcripts)
	}
	if got := sess.Transcripts[0].Status; got != TranscriptUnreadable {
		t.Errorf("transcript status = %q, want %q", got, TranscriptUnreadable)
	}
}

// TestRubricERRORS3_MissingTranscriptIsNotAReason pins the written decision
// on ENOENT: Claude Code deletes transcripts past its cleanup period, so a
// missing one is the ordinary state of every old run, and a reason for it
// would turn every such run unverified after the fact. The group still
// renders readable: no, and verified then excludes the H-10 check.
func TestRubricERRORS3_MissingTranscriptIsNotAReason(t *testing.T) {
	sess := sessionNaming(filepath.Join(t.TempDir(), "never-written.jsonl"))
	if sess.Coverage.State != store.StateVerified {
		t.Errorf("a missing transcript made the run %s %v, want verified", sess.Coverage.State, sess.Coverage.Reasons)
	}
	if len(sess.Transcripts) != 1 || sess.Transcripts[0].Readable {
		t.Errorf("transcripts = %+v, want one, unreadable", sess.Transcripts)
	}
	if got := sess.Transcripts[0].Status; got != TranscriptMissing {
		t.Errorf("transcript status = %q, want %q: missing and unreadable both leave Readable false, "+
			"and Status is the field that is supposed to tell them apart", got, TranscriptMissing)
	}
}

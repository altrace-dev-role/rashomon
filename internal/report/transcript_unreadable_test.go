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
}

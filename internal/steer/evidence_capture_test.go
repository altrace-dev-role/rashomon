package steer

import (
	"path/filepath"
	"testing"
)

// capturedNote is the additionalContext the capture's hook printed.
const capturedNote = "rashomon record: fixture note for the channel-evidence scanner."

// TestCE1f_ARealCaptureReadsAsSeenAndAlteredAsAbsent: testdata/transcript is
// the session transcript of a real claude -p run (Claude Code 2.1.308, haiku,
// a PostToolUse hook on Bash printing capturedNote as additionalContext, the
// model running `true`), scrubbed by test/tools/scrub and trimmed to the five
// records around the hook call; the kept records are what Claude Code wrote,
// bar the placeholders for ids and paths. It also holds a hook_success record
// whose stdout carries the note, which must not count. The twin differs in
// the one attachment's content, so the same note is absent.
func TestCE1f_ARealCaptureReadsAsSeenAndAlteredAsAbsent(t *testing.T) {
	for _, tc := range []struct {
		file string
		want Evidence
	}{
		{"posttooluse-seen.jsonl", EvidenceSeen},
		{"posttooluse-absent.jsonl", EvidenceAbsent},
	} {
		t.Run(tc.file, func(t *testing.T) {
			ctx := routerContext(t)
			l := Line{Type: LineType, V: LineVersion, ID: "cap1", AtUnixMS: ctx.NowUnixMS, SessionID: ctx.SessionID,
				State: LineStateDelivered, ModelText: capturedNote, Event: "PostToolUse",
				ClientVersion: "2.1.280", VerifiedFloor: "2.1.280"}
			appendLine(t, ctx, &l)
			path := filepath.Join("testdata", "transcript", tc.file)
			got := CheckEvidence(ctx.Store, ctx.SessionID, path, evNow(ctx), bigLimits(evNow(ctx)))
			if len(got) != 1 || got[0].Evidence != tc.want {
				t.Errorf("%s read as %+v, want %s", tc.file, got, tc.want)
			}
		})
	}
}

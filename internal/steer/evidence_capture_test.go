package steer

import (
	"path/filepath"
	"testing"
	"time"
)

// captureSessionID is the placeholder the scrub left in every record's
// sessionId; captureAttachmentUnixMS is the attachment's timestamp,
// 2026-10-31T04:51:42.590Z.
const (
	captureSessionID        = "00000000-0000-4000-8000-000000000003"
	captureAttachmentUnixMS = int64(1793422302590)
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
			// The note was delivered a second before the capture's attachment
			// (04:51:42.590Z), in the capture's own session, so the scrubbed
			// records are the session's and end well past the note.
			ctx.SessionID = captureSessionID
			ctx.NowUnixMS = captureAttachmentUnixMS - time.Second.Milliseconds()
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

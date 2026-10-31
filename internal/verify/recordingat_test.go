package verify

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSV1m_RecordingAtIsRecording: status's reader answers what recording
// answers, notes in the same order, for a complete record, a record with a
// call left open and a record with no coverage at all. Break: RecordingAt
// drops the notes, and status says incomplete without saying why.
func TestSV1m_RecordingAtIsRecording(t *testing.T) {
	open := &store.Run{Declarations: []store.Declaration{{ToolUseID: "tu-1", Seq: 1}}}
	for _, c := range []struct {
		run *store.Run
		at  int64
	}{{&store.Run{}, 0}, {open, 1}, {open, 0}, {&store.Run{Skipped: 2}, 3}} {
		want, wantNotes := recording(c.run, c.at)
		got, notes := RecordingAt(c.run, c.at)
		if got != want || !slices.Equal(notes, wantNotes) {
			t.Errorf("RecordingAt(%+v, %d) = %s %q, want %s %q", c.run, c.at, got, notes, want, wantNotes)
		}
	}
}

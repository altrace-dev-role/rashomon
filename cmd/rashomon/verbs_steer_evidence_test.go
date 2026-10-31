package main

import (
	"io"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSTC1n_AnEvidenceLineIsNotAnUnreadableOne: steer.ndjson also holds the
// channel-evidence lines (type steer_evidence, CHANNEL-EVIDENCE1), which are
// valid and are not decisions. steerRows counts them in neither rows nor
// skipped, so `steer log` and `scorecard` do not say a line "could not be
// read" in every session that had a delivered note. One note line and one
// evidence line: one row, none skipped. Break: drop the evidence skip in
// steerRows and skipped is 1.
func TestSTC1n_AnEvidenceLineIsNotAnUnreadableOne(t *testing.T) {
	st, _ := stcStore(t)
	stcAppend(t, st, stcDecision{"s1", "regression-location", "PostToolUse", "nudge", "delivered", "", "", 1_800_000_001_000})
	ev := &steer.EvidenceLine{Type: steer.EvidenceLineType, V: steer.LineVersion, SteerID: "x", SessionID: "s1",
		AtUnixMS: 1_800_000_002_000, Event: "PostToolUse", Evidence: steer.EvidenceSeen}
	if err := st.AppendSteer("s1", func(io.ReaderAt) ([]store.SteerLine, error) { return []store.SteerLine{ev}, nil }); err != nil {
		t.Fatal(err)
	}
	rows, skipped, err := steerRows(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || skipped != 0 {
		t.Errorf("steerRows = %d rows, %d skipped; want 1 row (the note's) and 0 skipped", len(rows), skipped)
	}
}

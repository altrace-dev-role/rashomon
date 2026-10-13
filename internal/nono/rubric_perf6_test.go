package nono

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRubricPerf6_TheLineBoundIsNonoMaxLine pins the trail's line bound at its
// edge. The first line of the real capture is session_started, the record
// whose command line can be long; it is padded inside the command so the line
// is exactly the longest the scanner holds (nonoMaxLine less the newline) and
// then one byte longer. The first reads as an observed session; the second is
// a read that failed partway, which this package reports as unreadable, never
// as a session that went quiet after its first line.
func TestRubricPerf6_TheLineBoundIsNonoMaxLine(t *testing.T) {
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	first, rest, ok := bytes.Cut(raw, []byte("\n"))
	if !ok || !bytes.Contains(first, []byte(`session_started`)) {
		t.Fatalf("premise: the capture's first line is not session_started")
	}
	// Inside event_json, a JSON string holding JSON: the shell's name is
	// written \"sh\" there, and padding it keeps both levels valid.
	anchor := []byte(`[\"sh\"`)
	if bytes.Count(first, anchor) != 1 {
		t.Fatalf("premise: %s does not occur once in the first line", anchor)
	}
	window := fixtureWindow(t)
	for _, tc := range []struct {
		length   int
		observed bool
	}{
		// The documented bound, written out: a test that read the
		// constant back would pass whatever value it was given.
		{8<<20 - 1, true},
		{8 << 20, false},
	} {
		pad := bytes.Repeat([]byte("x"), tc.length-len(first))
		line := bytes.Replace(first, anchor, append(append([]byte(`[\"sh`), pad...), `\"`...), 1)
		if len(line) != tc.length {
			t.Fatalf("built a %d-byte line, want %d", len(line), tc.length)
		}
		path := filepath.Join(t.TempDir(), "audit-events.ndjson")
		body := append(append(line, '\n'), rest...)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		obs := Read(path, window)
		if obs.Observed != tc.observed {
			t.Errorf("a %d-byte session_started line: observed=%v (%s), want %v",
				tc.length, obs.Observed, obs.Reason, tc.observed)
		}
		if !tc.observed && obs.Reason != NotObservedUnreadable {
			t.Errorf("a line over the bound reads as %q, want %q", obs.Reason, NotObservedUnreadable)
		}
	}
}

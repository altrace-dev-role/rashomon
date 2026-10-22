package nono

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRubricPERF6_TheLineBoundIsNonoMaxLine pins the trail's line bound at its
// edge. The first line of the real capture is session_started, the record
// whose command line can be long; it is padded inside the command so the line
// is exactly the longest the scanner holds (nonoMaxLine less the newline) and
// then one byte longer. The first reads as the capture does. The second is
// one record not read: counted in Skipped, the session it opened not counted,
// and every line after it read -- one long command never costs the rest of
// the trail.
func TestRubricPERF6_TheLineBoundIsNonoMaxLine(t *testing.T) {
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
	whole := Read(fixture, window)
	if !whole.Observed || whole.Sessions != 1 || len(whole.Events) == 0 {
		t.Fatalf("premise: the capture reads as %+v, want one observed session with events", whole)
	}
	for _, tc := range []struct {
		length  int
		skipped bool
	}{
		// The documented bound, written out: a test that read the
		// constant back would pass whatever value it was given.
		{8<<20 - 1, false},
		{8 << 20, true},
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
		wantSkipped, wantSessions := whole.Skipped, whole.Sessions
		if tc.skipped {
			wantSkipped, wantSessions = whole.Skipped+1, whole.Sessions-1
		}
		if !obs.Observed || obs.Skipped != wantSkipped || obs.Sessions != wantSessions || len(obs.Events) != len(whole.Events) {
			t.Errorf("a %d-byte session_started line: observed %v (%s), skipped %d, sessions %d, %d events; want observed, %d, %d and %d",
				tc.length, obs.Observed, obs.Reason, obs.Skipped, obs.Sessions, len(obs.Events),
				wantSkipped, wantSessions, len(whole.Events))
		}
	}
}

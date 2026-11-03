package directive

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTranscript writes lines, each followed by a line feed, to a transcript
// file under a test directory and returns its path.
func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestCSL1_LastBoundaryIsTheLastCompactRecord: the boundary is the last
// top-level system record whose subtype is compact_boundary, with its uuid
// and its line counted from 1. The word inside a message, a nested record,
// a record of another type with the subtype, a line too long to be a
// boundary record and a line that is not JSON are passed over; a transcript
// with none says so, one over the cap is not read, and a missing one is an
// error. Break: count from 0, take the first boundary, match the word
// anywhere or under any type, or read past the cap, and this fails.
func TestCSL1_LastBoundaryIsTheLastCompactRecord(t *testing.T) {
	long := `{"type":"system","subtype":"compact_boundary","uuid":"b-long","pad":"` + strings.Repeat("x", maxBoundaryLine) + `"}`
	p := writeTranscript(t,
		`{"type":"user","uuid":"u1","message":{"role":"user","content":"keep the split"}}`,
		`{"type":"system","subtype":"compact_boundary","uuid":"b-1","content":"Conversation compacted"}`,
		`{"type":"assistant","uuid":"a1","message":{"content":[{"type":"text","text":"the \"compact_boundary\" record"}]}}`,
		`{"type":"system","subtype":"compact_boundary","uuid":"b-2","content":"Conversation compacted"}`,
		`{"type":"assistant","data":{"type":"system","subtype":"compact_boundary","uuid":"b-nested"}}`,
		`{"type":"user","subtype":"compact_boundary","uuid":"b-user"}`,
		long,
		`{"type":"system","subtype":"compact_boundary","uuid":"b-broken"`,
		`{"type":"user","uuid":"u2","message":{"role":"user","content":"after"}}`,
	)
	b, ok, err := LastBoundary(p, 1<<20)
	if err != nil || !ok || b != (Boundary{UUID: "b-2", Line: 4}) {
		t.Errorf("LastBoundary = %+v, %v, %v; want b-2 at line 4", b, ok, err)
	}
	none := writeTranscript(t, `{"type":"user","uuid":"u1","message":{"role":"user","content":"keep the split"}}`)
	if b, ok, err := LastBoundary(none, 1<<20); err != nil || ok {
		t.Errorf("a transcript with no boundary: %+v, %v, %v; want none", b, ok, err)
	}
	if _, ok, err := LastBoundary(p, 1024); !errors.Is(err, ErrTooLarge) || ok {
		t.Errorf("a transcript over the cap: %v, %v; want ErrTooLarge", ok, err)
	}
	if _, ok, err := LastBoundary(filepath.Join(t.TempDir(), "gone.jsonl"), 1<<20); err == nil || ok {
		t.Errorf("a missing transcript: %v, %v; want an error", ok, err)
	}
	last := writeTranscript(t, `{"type":"user","uuid":"u1"}`, `{"type":"system","subtype":"compact_boundary","uuid":"b-9"}`)
	if b, ok, err := LastBoundary(last, 1<<20); err != nil || !ok || b != (Boundary{UUID: "b-9", Line: 2}) {
		t.Errorf("a boundary on the last line = %+v, %v, %v; want b-9 at line 2", b, ok, err)
	}
}

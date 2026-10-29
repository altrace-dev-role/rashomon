package pgraph

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// platformFixtureLog is the small graph whose encoded log is the rashomon
// module's internal/platform fixture (card 6.1): two packages, a type with a
// method, calls within and across them, and one retraction, so every line
// form that decoder reads is in it. Two reconciles at two commits, as Sync
// writes them.
func platformFixtureLog(t *testing.T) *Log {
	t.Helper()
	const m = "example.com/m"
	first := []Triple{
		{"pkg:" + m + "/a", Declares, "func:" + m + "/a.F"},
		{"pkg:" + m + "/a", Declares, "type:" + m + "/a.T"},
		{"type:" + m + "/a.T", Declares, "method:" + m + "/a.T.M"},
		{"method:" + m + "/a.T.M", Calls, "func:" + m + "/a.F"},
		{"pkg:" + m + "/b", Declares, "func:" + m + "/b.G"},
		{"func:" + m + "/b.G", Calls, "func:" + m + "/a.F"},
		{"pkg:" + m + "/b", Declares, "var:" + m + "/b.Old"},
	}
	cfg := Config{GOOS: "linux", GOARCH: "amd64"}.String()
	var l Log
	if _, err := l.Reconcile(gotypesProducer(), cfg, "c1", first); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reconcile(gotypesProducer(), cfg, "c2", first[:len(first)-1]); err != nil {
		t.Fatal(err)
	}
	return &l
}

// TestPL8h_FixturePinned: testdata/platform/facts.log is the current Encode
// of the fixture graph, byte for byte. internal/platform reads a copy of it
// and pins that copy to this file, so a codec change fails on both sides of
// the boundary at once. PGRAPH_WRITE_FIXTURE=1 rewrites the file from Encode.
func TestPL8h_FixturePinned(t *testing.T) {
	got := platformFixtureLog(t).Encode()
	path := filepath.Join("testdata", "platform", "facts.log")
	if os.Getenv("PGRAPH_WRITE_FIXTURE") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is not the fixture graph's Encode:\ngot  %q\nwant %q", path, got, want)
	}
	if _, err := DecodeLog(want); err != nil {
		t.Errorf("the pinned fixture does not decode: %v", err)
	}
}

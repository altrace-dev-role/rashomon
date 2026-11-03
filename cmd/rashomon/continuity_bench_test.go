package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/directive"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// benchCompactTranscript writes size bytes of transcript shaped as
// internal/directive's BenchmarkBoundary reads (37a15912 on 11-02: about
// 2 KiB a line, one line over 64 KiB, 15 compact boundaries) and returns its
// path: 2 KiB user records, a 66 KiB line every 8 MiB and a boundary every
// size/16 bytes.
func benchCompactTranscript(b *testing.B, size int) string {
	b.Helper()
	rec := func(n int, kind string) []byte {
		head := `{"type":"user","uuid":"u","message":{"role":"user","content":"` + kind
		tail := `"}}` + "\n"
		return []byte(head + strings.Repeat("x", n-len(head)-len(tail)) + tail)
	}
	every := size / 16
	nextBoundary, nextLong := every, 8<<20
	out := make([]byte, 0, size+(66<<10))
	for n := 1; len(out) < size; n++ {
		switch {
		case len(out) >= nextBoundary:
			out = append(out, `{"type":"system","subtype":"compact_boundary","uuid":"b-`+strconv.Itoa(n)+`","content":"Conversation compacted"}`+"\n"...)
			nextBoundary += every
		case len(out) >= nextLong:
			out, nextLong = append(out, rec(66<<10, "long ")...), nextLong+8<<20
		default:
			out = append(out, rec(2048, "")...)
		}
	}
	p := filepath.Join(b.TempDir(), benchSessionID+".jsonl")
	if err := os.WriteFile(p, out, 0o600); err != nil {
		b.Fatal(err)
	}
	return p
}

// BenchmarkCompact is probe start at a compaction (COMPACT-SLOT1; mgr
// 11-02: the 256 MiB cap's condition, measured, not assumed) on a 40 MiB
// transcript: the boundary pass, the dedupe's steer-log read, a stand-in
// Read of five directives (DIRECTIVE-KEEP1's body is not here, and its own
// read of the transcript is its row's), the briefing's render, the run read,
// the spec walk, the route, its line and the print. The steer log is removed
// before each call, so every call gives the compaction's briefing. Its ns/op
// is the compact start's wall time in process, which no gate reads (RES-36);
// its B/op and allocs/op are its row.
func BenchmarkCompact(b *testing.B) {
	b.Run("40MiB", func(b *testing.B) {
		root, installID := benchStarted(b)
		benchSteerPins(b)
		b.Setenv("AI_AGENT", "claude-code_2-1-316_harness")
		if err := store.WriteSteerDisclosureMarker(root, steer.DisclosureVersion); err != nil {
			b.Fatal(err)
		}
		tr := benchCompactTranscript(b, 40<<20)
		was := readDirectives
		readDirectives = func(string, int64) ([]directive.Directive, error) {
			return []directive.Directive{
				{Kind: directive.KindDirective, Said: "they go vertical, we go horizontal", Line: 1},
				{Kind: directive.KindDirective, Said: "put the release notes in a follow-up", Line: 2},
				{Kind: directive.KindCorrection, Said: "keep the release notes in the same commit", Line: 3, Restates: 2},
				{Kind: directive.KindDirective, Said: "every row card names its killer test", Line: 4},
				{Kind: directive.KindDirective, Said: "the slot carries named tests and rows only", Line: 5},
			}, nil
		}
		defer func() { readDirectives = was }()
		st, err := store.Open(root)
		if err != nil {
			b.Fatal(err)
		}
		steerLog := filepath.Join(st.RunDir(benchSessionID), store.FileSteer)
		input := benchJSON(b, struct {
			benchHead
			Source string `json:"source"`
		}{benchHead{benchSessionID, tr, benchCWD, "default", "SessionStart"}, "compact"})
		args := []string{"start", install.Marker, installID}
		rd := bytes.NewReader(input)
		var stdout, stderr benchSink
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			if err := os.Remove(steerLog); err != nil && !errors.Is(err, fs.ErrNotExist) {
				b.Fatal(err)
			}
			rd.Reset(input)
			b.StartTimer()
			if code := cmdProbe(args, rd, &stdout, &stderr); code != exitOK {
				b.Fatalf("probe exited %d", code)
			}
		}
		b.StopTimer()
		if stdout.writes != b.N || stderr.writes != 0 || !bytes.Contains(stdout.lastWrite(), []byte(`"additionalContext"`)) {
			b.Fatalf("%d writes for %d calls, stderr %q, last %s; want the briefing every call", stdout.writes, b.N, stderr.lastWrite(), stdout.lastWrite())
		}
	})
}

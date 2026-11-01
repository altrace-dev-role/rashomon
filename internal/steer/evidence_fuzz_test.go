package steer

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"unicode/utf8"
)

// FuzzScanTranscript: the Stop's read of a transcript Claude Code wrote,
// whose records hold agent and tool text. On any file, any note text and any
// window: found has one entry per needle; the read is deterministic; a
// deadline already past is never complete and finds nothing; a read is done
// well inside a deadline of a second (its bound, ScanLimits); and a needle
// whose text is in no byte of a valid UTF-8 file with no escape is not found.
// A regression guard over the scanner's own code (line, stringsOf, textOf,
// parseStamp, the window), which is new over the fuzzed jsonscan.
func FuzzScanTranscript(f *testing.F) {
	at := func(stamp string) time.Time {
		v, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			f.Fatal(err)
		}
		return v
	}
	for _, name := range []string{"posttooluse-seen.jsonl", "posttooluse-absent.jsonl"} {
		b, err := os.ReadFile(filepath.Join("testdata", "transcript", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b, "rashomon", "", uint16(0))
		f.Add(b, "rashomon", "s1", uint16(512))
	}
	line := `{"type":"attachment","timestamp":"2026-10-26T12:00:00.000Z","sessionId":"s1","attachment":{"type":"hook_additional_context","content":["rashomon record: a note"]}}`
	for _, s := range []string{
		userLine + "\n" + line + "\n",
		line + "\n" + `{"timestamp":"2026-10-26T12:00:01.000Z","sessionId":"s2"}` + "\n",
		`{"type":"attachment","timestamp":"2026-02-29T12:00:00.000Z","attachment":{"type":"hook_additional_context","content":"rashomon record: a note"}}` + "\n",
		`{"timestamp":"2026-10-26T12:00:00.5+02:00","attachment":{"type":"hook_additional_context","content":[1,"rashomon",null]}}`,
		"{\n{}\n\n" + line,
		"",
	} {
		f.Add([]byte(s), "rashomon record", "s1", uint16(64))
	}
	dir := f.TempDir()
	notedAt := at("2026-10-26T11:59:59.000Z")
	f.Fuzz(func(t *testing.T, data []byte, text, session string, window uint16) {
		p := filepath.Join(dir, "t.jsonl")
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		needles := []Needle{{Text: text, At: notedAt}, {Text: "rashomon record", At: notedAt.Add(-time.Hour)}}
		lim := ScanLimits{MaxBytes: int64(window) + 1, MaxLine: 4 << 10}
		if window == 0 {
			lim.MaxBytes = 16 << 20
		}
		found, complete := ScanTranscript(p, session, needles, lim)
		if len(found) != len(needles) {
			t.Fatalf("found has %d entries for %d needles", len(found), len(needles))
		}
		again, complete2 := ScanTranscript(p, session, needles, lim)
		if !slices.Equal(found, again) || complete != complete2 {
			t.Fatalf("not deterministic: %v %v, then %v %v", found, complete, again, complete2)
		}
		if text != "" && utf8.Valid(data) && !bytes.ContainsRune(data, '\\') && !bytes.Contains(data, []byte(text)) && found[0] {
			t.Fatalf("found %q, which is in no byte of the file", text)
		}
		past := lim
		past.Deadline = time.Now().Add(-time.Second)
		if found, complete := ScanTranscript(p, session, needles, past); complete || slices.Contains(found, true) {
			t.Fatalf("a deadline already past read %v, complete %v", found, complete)
		}
		start := time.Now()
		timed := lim
		timed.Deadline = start.Add(time.Second)
		ScanTranscript(p, session, needles, timed)
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("a one-second deadline read for %v", d)
		}
	})
}

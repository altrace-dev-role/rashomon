package spend

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/report"
)

// TestRubricTEST8_TokenCountAtNAndNPlusOne: a count of exactly maxTokens is a
// response's; one more is not, in every field a usage carries.
func TestRubricTEST8_TokenCountAtNAndNPlusOne(t *testing.T) {
	if maxTokens != 100_000_000 {
		t.Fatalf("maxTokens = %d, want 100,000,000", maxTokens)
	}
	set := []func(u *usage, v int64){
		func(u *usage, v int64) { u.Input = v },
		func(u *usage, v int64) { u.Output = v },
		func(u *usage, v int64) { u.CacheRead = v },
		func(u *usage, v int64) { u.CacheCreation = v },
		func(u *usage, v int64) { u.CacheSplit = &cacheCreation{Ephemeral5m: v} },
		func(u *usage, v int64) { u.CacheSplit = &cacheCreation{Ephemeral1h: v} },
		func(u *usage, v int64) { u.Iterations = []iteration{{tokens: tokens{Input: v}}} },
	}
	for i, f := range set {
		var at, over usage
		f(&at, maxTokens)
		f(&over, maxTokens+1)
		if !at.plausible() || over.plausible() {
			t.Errorf("field %d: %d plausible %v, %d plausible %v; want true, false", i, int64(maxTokens), at.plausible(), int64(maxTokens+1), over.plausible())
		}
	}
}

// TestRubricTEST8_SpendLineAtN: a transcript line of exactly
// report.TranscriptMaxLine bytes, counting its newline, is read and its
// response priced; one byte more is stepped over and counted as a usage line
// that could not be read, and the file is not unreadable.
func TestRubricTEST8_SpendLineAtN(t *testing.T) {
	if report.TranscriptMaxLine != 64<<20 {
		t.Fatalf("report.TranscriptMaxLine = %d, want 64 MiB", report.TranscriptMaxLine)
	}
	for _, tc := range []struct {
		n    int
		read bool
	}{{report.TranscriptMaxLine, true}, {report.TranscriptMaxLine + 1, false}} {
		c := newConfig(t)
		base := resp{id: "big", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1000, stop: "end_turn"}.line("text")
		head := strings.TrimSuffix(base, "}") + `,"pad":"`
		line := head + strings.Repeat("x", tc.n-len(head)-len("\"}\n")) + "\"}"
		p := c.write("proj/sess-a.jsonl", line)
		if fi, err := os.Stat(p); err != nil || fi.Size() != int64(tc.n) {
			t.Fatalf("built %v bytes, want %d (%v)", fi.Size(), tc.n, err)
		}
		s := c.summary(30)
		if read := s.Responses == 1 && s.scan.Unparsed == 0; read != tc.read || s.scan.Unreadable != 0 {
			t.Errorf("a %d-byte line: %d unreadable, %d unparsed, %d responses; want read %v, none unreadable", tc.n, s.scan.Unreadable, s.scan.Unparsed, s.Responses, tc.read)
		}
	}
}

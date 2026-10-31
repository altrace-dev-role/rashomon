package steer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// benchTranscript is a 4 MiB transcript of 2,000 records: user and assistant
// records of about 2 KiB, and every fiftieth a hook_additional_context
// attachment, forty in all, none carrying the needle, so the scan reads to the
// end and finds nothing, the work an absent note costs.
func benchTranscript(tb testing.TB) string {
	tb.Helper()
	var b strings.Builder
	pad := strings.Repeat("x", 1900)
	for i := range 2000 {
		stamp := fmt.Sprintf("2026-10-26T00:%02d:%02d.000Z", i/60%60, i%60)
		if i%50 == 25 {
			fmt.Fprintf(&b, `{"type":"attachment","timestamp":%q,"attachment":{"type":"hook_additional_context","hookEvent":"PostToolUse","content":["context %d %s"]}}`+"\n", stamp, i, pad[:200])
			continue
		}
		fmt.Fprintf(&b, `{"type":"user","timestamp":%q,"message":{"role":"user","content":"%s"}}`+"\n", stamp, pad)
	}
	p := filepath.Join(tb.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		tb.Fatal(err)
	}
	return p
}

// BenchmarkEvidenceScan is the Stop's transcript read (card CHANNEL-EVIDENCE1):
// ScanTranscript over a 4 MiB transcript for one needle it does not hold, under
// the Stop's own limits. Its row in budgets.json is the read's allocation
// budget; the read's bytes, line and time bounds are DefaultScanLimits'.
func BenchmarkEvidenceScan(b *testing.B) {
	path := benchTranscript(b)
	needles := []Needle{{"rashomon record: a note that is not in the transcript", time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)}}
	b.ReportAllocs()
	for b.Loop() {
		found, complete := ScanTranscript(path, "", needles, DefaultScanLimits(time.Now()))
		if found[0] || !complete {
			b.Fatalf("found %v, complete %v", found, complete)
		}
	}
}

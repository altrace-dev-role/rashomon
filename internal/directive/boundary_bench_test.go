package directive

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// benchTranscript is size bytes of transcript shaped as 37a15912 read on
// 11-02 (20,259 lines in 41.0 MB, so about 2 KiB a line; one line over 64
// KiB; 15 compact_boundary records, and the word on no other line): 2 KiB
// user records, a 66 KiB line every 8 MiB, which the pass steps over, and a
// boundary every size/16 bytes, the word in no other record. It returns the
// last boundary, the one LastBoundary finds.
func benchTranscript(size int) (out []byte, last Boundary) {
	rec := func(n int, kind string) []byte {
		head := `{"type":"user","uuid":"u","message":{"role":"user","content":"` + kind
		tail := `"}}` + "\n"
		return []byte(head + strings.Repeat("x", n-len(head)-len(tail)) + tail)
	}
	every := size / 16
	nextBoundary, nextLong := every, 8<<20
	out = make([]byte, 0, size+(66<<10))
	for n := 1; len(out) < size; n++ {
		switch {
		case len(out) >= nextBoundary:
			id := "b-" + strconv.Itoa(n)
			out = append(out, `{"type":"system","subtype":"compact_boundary","uuid":"`+id+`","content":"Conversation compacted"}`+"\n"...)
			last, nextBoundary = Boundary{UUID: id, Line: n}, nextBoundary+every
		case len(out) >= nextLong:
			out, nextLong = append(out, rec(66<<10, "long ")...), nextLong+8<<20
		default:
			out = append(out, rec(2048, "")...)
		}
	}
	return out, last
}

// BenchmarkBoundary is a compact start's boundary pass (COMPACT-SLOT1;
// mgr 11-02: measured, not assumed) over a 40 MiB transcript shaped as the
// longest one on this machine (benchTranscript): one forward read, a line at
// a time through the 64 KiB reader. The file was just written, so it reads
// from the page cache, as a transcript Claude Code has just appended to does.
// Its ns/op is the pass's wall time, which no gate reads (RES-36); its B/op
// and allocs/op are its row.
func BenchmarkBoundary(b *testing.B) {
	b.Run("40MiB", func(b *testing.B) {
		data, want := benchTranscript(40 << 20)
		p := filepath.Join(b.TempDir(), "s.jsonl")
		if err := os.WriteFile(p, data, 0o600); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			got, ok, err := LastBoundary(p, 256<<20)
			if err != nil || !ok || got != want {
				b.Fatalf("LastBoundary = %+v, %v, %v; want %+v", got, ok, err, want)
			}
		}
	})
}

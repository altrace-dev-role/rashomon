package main

import (
	"fmt"
	"strings"
	"testing"
)

// FuzzParse runs Parse over `go test -bench` output (rubric fix list entry 23,
// errors-7). Output that parses names only benchmarks, and a well-formed
// result line appended to it reads back as written, or as a better run.
func FuzzParse(f *testing.F) {
	f.Add(sample, "Derive/x", uint32(4096), uint32(31))
	f.Add("BenchmarkNoAllocs-8   20   100 ns/op\n", "A", uint32(0), uint32(0))
	f.Add("Benchmark ns/op 1 B/op 1 allocs/op\n", "B-1", uint32(1), uint32(1))
	f.Fuzz(func(t *testing.T, out, name string, b, n uint32) {
		if len(out) > 60<<10 {
			return
		}
		got, err := Parse(strings.NewReader(out))
		if err != nil {
			return
		}
		for k, r := range got {
			if !strings.HasPrefix(k, "Benchmark") || r.BytesPerOp < 0 || r.AllocsPerOp < 0 {
				t.Fatalf("Parse(%q) holds %q: %+v", out, k, r)
			}
		}
		if f := strings.Fields(name); len(f) != 1 || f[0] != name || strings.Contains(name, "-") {
			return
		}
		ln := fmt.Sprintf("Benchmark%s-8   20   100 ns/op   %d B/op   %d allocs/op\n", name, b, n)
		again, err := Parse(strings.NewReader(out + "\n" + ln))
		if err != nil {
			t.Fatalf("Parse of %q then %q: %v", out, ln, err)
		}
		r, ok := again["Benchmark"+name]
		if !ok || r.BytesPerOp > int64(b) || r.AllocsPerOp > int64(n) {
			t.Fatalf("Parse of %q then %q = %+v", out, ln, again)
		}
	})
}

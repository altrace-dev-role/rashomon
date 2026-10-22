package main

import (
	"fmt"
	"strings"
	"testing"
)

// FuzzParse runs Parse over `go test -bench` output (rubric fix list entry 23,
// errors-7). Output that parses names only benchmarks, and a well-formed
// result line appended to it reads back exactly: as written when the output
// held no run of that benchmark, and when it did, as whichever of the two
// runs has the lower B/op, ties broken by allocs/op, kept whole. Every other
// benchmark reads as it did without the line.
func FuzzParse(f *testing.F) {
	for _, s := range parseSeeds {
		f.Add(s.out, s.name, s.b, s.n)
	}
	f.Fuzz(checkParse)
}

var parseSeeds = []struct {
	out, name string
	b, n      uint32
}{
	{sample, "Derive/x", 4096, 31},
	{"BenchmarkNoAllocs-8   20   100 ns/op\n", "A", 0, 0},
	{"Benchmark ns/op 1 B/op 1 allocs/op\n", "B-1", 1, 1},
	// An earlier run of the same benchmark, better on one figure and worse
	// on the other: the run with the lower B/op is kept whole, never a blend
	// of the two runs' figures.
	{"BenchmarkA-8   20   100 ns/op   50 B/op   3 allocs/op\n", "A", 40, 5},
	{"BenchmarkA-8   20   100 ns/op   50 B/op   3 allocs/op\n", "A", 60, 2},
}

// TestRubricERRORS7_ParseSeeds runs FuzzParse's property over its seeds as a
// plain test, which is what a mutant names.
func TestRubricERRORS7_ParseSeeds(t *testing.T) {
	for _, s := range parseSeeds {
		checkParse(t, s.out, s.name, s.b, s.n)
	}
}

func checkParse(t *testing.T, out, name string, b, n uint32) {
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
	key := "Benchmark" + name
	want := Result{int64(b), int64(n)}
	if prev, ok := got[key]; ok {
		if prev.BytesPerOp < want.BytesPerOp || (prev.BytesPerOp == want.BytesPerOp && prev.AllocsPerOp <= want.AllocsPerOp) {
			want = prev
		}
	}
	if r, ok := again[key]; !ok || r != want {
		t.Fatalf("Parse of %q then %q: %s = %+v (present %v), want %+v", out, ln, key, r, ok, want)
	}
	for k, r := range got {
		if k != key && again[k] != r {
			t.Fatalf("Parse of %q then %q moved %s from %+v to %+v", out, ln, k, r, again[k])
		}
	}
	wantLen := len(got) + 1
	if _, ok := got[key]; ok {
		wantLen--
	}
	if len(again) != wantLen {
		t.Fatalf("Parse of %q then %q holds %d benchmarks, want %d", out, ln, len(again), wantLen)
	}
}

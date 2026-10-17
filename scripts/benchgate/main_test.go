package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

const sample = `goos: darwin
BenchmarkDerive/bash_short-8         	      20	     12345 ns/op	    4096 B/op	      31 allocs/op
BenchmarkDerive/bash_short-8         	      20	     12000 ns/op	    4000 B/op	      30 allocs/op
BenchmarkRecordBytesPerCall-8        	      20	    900000 ns/op	 1850.00 MB/s	   16384 B/op	     120 allocs/op
BenchmarkLexTokens-8                 	      20	     37465 ns/op	 109.33 MB/s	       120.4 B/token	      1068 tokens/op	  128544 B/op	     750 allocs/op
BenchmarkRecordBytesPerCall-8   	      20	  25018085 ns/op	   0.08 MB/s	      2082 B/call	      1478 records-B/call	   16242 B/op	      84 allocs/op
PASS
`

func TestRES36b_ParseKeepsTheBestRun(t *testing.T) {
	got, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if r := got["BenchmarkDerive/bash_short"]; r.BytesPerOp != 4000 || r.AllocsPerOp != 30 {
		t.Errorf("best of two runs: %+v", r)
	}
	if r := got["BenchmarkRecordBytesPerCall"]; r.BytesPerOp != 16242 || r.AllocsPerOp != 84 {
		t.Errorf("best of a plain line and one with hyphenated custom units: %+v", r)
	}
	if r := got["BenchmarkLexTokens"]; r.BytesPerOp != 128544 || r.AllocsPerOp != 750 {
		t.Errorf("line with custom metrics between ns/op and B/op: %+v", r)
	}
	if _, err := Parse(strings.NewReader("BenchmarkNoAllocs-8   20   100 ns/op\n")); err == nil {
		t.Error("a benchmark without ReportAllocs parsed as gateable")
	}
}

// TestRES36c_CompareFailsClosed: over budget, budgeted but absent, a row with
// no figures, and a benchmark that ran with no row are each a problem, judged
// at the gate's own tolerance.
func TestRES36c_CompareFailsClosed(t *testing.T) {
	results := map[string]Result{"BenchmarkA": {1000, 10}, "BenchmarkB": {2000, 20}, "BenchmarkD": {5, 1}}
	budgets := []budget.Benchmark{
		{Name: "BenchmarkA", BytesPerOpMax: 1000, AllocsPerOpMax: 10},
		{Name: "BenchmarkB", BytesPerOpMax: 1500, AllocsPerOpMax: 20},
		{Name: "BenchmarkC", BytesPerOpMax: 1, AllocsPerOpMax: 1},
		{Name: "BenchmarkA", BytesPerOpMax: 0, AllocsPerOpMax: 0},
	}
	problems := Compare(results, budgets, tolerance)
	want := []string{
		"BenchmarkB: 2000 B/op over budget 1500",
		"BenchmarkC: budgeted but did not run",
		"BenchmarkA: budget row has no figures",
		"BenchmarkD: ran without a budget row",
	}
	if strings.Join(problems, "|") != strings.Join(want, "|") {
		t.Errorf("problems %q\nwant %q", problems, want)
	}
	if p := Compare(map[string]Result{"BenchmarkA": {1100, 11}}, budgets[:1], tolerance); len(p) != 0 {
		t.Errorf("ten percent over must pass: %q", p)
	}
	if p := Compare(map[string]Result{"BenchmarkA": {1101, 10}}, budgets[:1], tolerance); len(p) != 1 {
		t.Errorf("more than ten percent over must fail: %q", p)
	}
	if p := Compare(map[string]Result{"BenchmarkA": {1000, 12}}, budgets[:1], tolerance); len(p) != 1 {
		t.Errorf("allocs more than ten percent over must fail: %q", p)
	}
}

// TestRES36e_GateToleranceIsTenPercent: RES-36's "fail at > 10% over" is the
// gate's one margin, so the constant is pinned to it, not merely used.
func TestRES36e_GateToleranceIsTenPercent(t *testing.T) {
	if tolerance != 0.10 {
		t.Errorf("benchgate tolerance %v, RES-36 says 0.10", tolerance)
	}
}

// fixture writes a budgets file naming goVersion and one benchmark row.
func fixture(t *testing.T, goVersion string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budgets.json")
	doc := `{"budgets_version": 1, "go_version": "` + goVersion + `", "benchmarks": [` +
		`{"name": "BenchmarkLexTokens", "measured_bytes_per_op": 128544, "measured_allocs_per_op": 750, "bytes_per_op_max": 128544, "allocs_per_op_max": 750}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const lexLine = "BenchmarkLexTokens-8   20   37465 ns/op   128544 B/op   750 allocs/op\n"

// TestRES36g_ToolchainMismatchFailsClosed: rows measured under one toolchain
// are not compared with another's figures (go1.25.8 allocates three times
// what go1.27.2 does on BenchmarkBuild/188); the gate exits 2 naming both
// versions before it reads a single result, and a file naming no toolchain
// fails the same way.
func TestRES36g_ToolchainMismatchFailsClosed(t *testing.T) {
	for _, c := range []struct {
		name, file, running string
		want                int
		says                []string
	}{
		{"match", "go1.27.2", "go1.27.2", 0, []string{"1 benchmarks within budget"}},
		{"mismatch", "go1.27.2", "go1.25.8", 2, []string{"go1.27.2", "go1.25.8"}},
		{"unnamed", "", "go1.27.2", 2, []string{"go_version"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(fixture(t, c.file), strings.NewReader(lexLine), &stdout, &stderr, c.running)
			if code != c.want {
				t.Fatalf("exit %d, want %d\nstdout %s\nstderr %s", code, c.want, stdout.String(), stderr.String())
			}
			out := stdout.String() + stderr.String()
			for _, s := range c.says {
				if !strings.Contains(out, s) {
					t.Errorf("output does not name %q:\n%s", s, out)
				}
			}
			if c.want == 2 && strings.Contains(stdout.String(), "B/op") {
				t.Errorf("results printed before the toolchain check:\n%s", stdout.String())
			}
		})
	}
}

// TestRES36k_NoBenchmarkRowsComparesNothing: a budgets file with no benchmark
// rows gates nothing, so the gate exits 2 saying so, whatever ran; with rows,
// output in which no benchmark ran fails on every row as "did not run".
func TestRES36k_NoBenchmarkRowsComparesNothing(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name, benchmarks, input string
		want                    int
		says                    string
	}{
		{"no key, nothing ran", ``, "", 2, "no benchmark rows"},
		{"no key, one ran", ``, lexLine, 2, "no benchmark rows"},
		{"empty list, nothing ran", `, "benchmarks": []`, "", 2, "no benchmark rows"},
		{"rows, nothing ran", `, "benchmarks": [{"name": "BenchmarkLexTokens", "bytes_per_op_max": 1, "allocs_per_op_max": 1}]`, "", 1, "BenchmarkLexTokens: budgeted but did not run"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(c.name, " ", "_")+".json")
			if err := os.WriteFile(path, []byte(`{"budgets_version": 1, "go_version": "go1.27.2"`+c.benchmarks+`}`), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := run(path, strings.NewReader(c.input), &stdout, &stderr, "go1.27.2")
			if code != c.want || !strings.Contains(stderr.String(), c.says) {
				t.Errorf("exit %d, want %d naming %q\nstdout %s\nstderr %s", code, c.want, c.says, stdout.String(), stderr.String())
			}
		})
	}
}

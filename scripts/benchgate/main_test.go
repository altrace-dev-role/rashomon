package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
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
BenchmarkCrossed-8                   	      20	     10000 ns/op	    1000 B/op	      50 allocs/op
BenchmarkCrossed-8                   	      20	     11000 ns/op	    1200 B/op	      10 allocs/op
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
	// BenchmarkCrossed's two runs disagree on which field is lower: run one
	// has the lower B/op (1000 vs 1200) but the higher allocs/op (50 vs 10).
	// Parse must keep run one whole (1000, 50), never blend in run two's
	// allocs/op to report the impossible pair (1000, 10) neither run
	// produced.
	if r := got["BenchmarkCrossed"]; r.BytesPerOp != 1000 || r.AllocsPerOp != 50 {
		t.Errorf("crossed runs: got %+v, want the lower-B/op run whole {1000 50}", r)
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
		{Name: "BenchmarkA", BytesPerOpMax: -1, AllocsPerOpMax: 0},
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

func TestRES36r_ZeroAllocationsAreGateable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budgets.json")
	doc := `{"budgets_version": 1, "go_version": "go1.27.2", "benchmark_platform": "linux/amd64", "benchmarks": [` +
		`{"name": "BenchmarkZeroAlloc", "measured_bytes_per_op": 0, "measured_allocs_per_op": 0, "bytes_per_op_max": 0, "allocs_per_op_max": 0}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	line := "BenchmarkZeroAlloc-8 20 100 ns/op 0 B/op 0 allocs/op\n"
	var stdout, stderr bytes.Buffer
	if code := run(path, strings.NewReader(line), &stdout, &stderr, "go1.27.2", "linux/amd64"); code != 0 {
		t.Fatalf("exit %d, want 0\nstdout %s\nstderr %s", code, stdout.String(), stderr.String())
	}
}

// TestRES36e_GateToleranceIsTenPercent: RES-36's "fail at > 10% over" is the
// gate's one margin, so the constant is pinned to it, not merely used.
func TestRES36e_GateToleranceIsTenPercent(t *testing.T) {
	if tolerance != 0.10 {
		t.Errorf("benchgate tolerance %v, RES-36 says 0.10", tolerance)
	}
}

// fixturePlatform is the benchmark_platform fixture writes into every file it
// produces. TestRES36g varies goVersion/running to exercise CheckToolchain
// only; naming a fixed platform here and passing the same value as run's
// platform argument keeps CheckPlatform out of that test's way.
const fixturePlatform = "linux/amd64"

// fixture writes a budgets file naming goVersion, fixturePlatform, and one
// benchmark row.
func fixture(t *testing.T, goVersion string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budgets.json")
	doc := `{"budgets_version": 1, "go_version": "` + goVersion + `", "benchmark_platform": "` + fixturePlatform + `", "benchmarks": [` +
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
			code := run(fixture(t, c.file), strings.NewReader(lexLine), &stdout, &stderr, c.running, fixturePlatform)
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

// fixtureOnPlatform is fixture with an explicit benchmark_platform, for
// TestRES36n where the platform (not the toolchain) is the thing under test.
func fixtureOnPlatform(t *testing.T, platform string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budgets.json")
	doc := `{"budgets_version": 1, "go_version": "go1.27.2", "benchmark_platform": "` + platform + `", "benchmarks": [` +
		`{"name": "BenchmarkLexTokens", "measured_bytes_per_op": 128544, "measured_allocs_per_op": 750, "bytes_per_op_max": 128544, "allocs_per_op_max": 750}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRES36n_PlatformMismatchFailsClosed: rows measured on one GOOS/GOARCH
// are not compared with a run on another (long $TMPDIR paths on darwin skew
// B/op against the same benchmark on linux); the gate exits 2 naming both
// platforms before it reads a single result, and a file naming no platform
// fails the same way. Mirrors TestRES36g_ToolchainMismatchFailsClosed, this
// time holding the toolchain fixed and varying the platform.
func TestRES36n_PlatformMismatchFailsClosed(t *testing.T) {
	for _, c := range []struct {
		name, file, running string
		want                int
		says                []string
	}{
		{"match", "linux/amd64", "linux/amd64", 0, []string{"1 benchmarks within budget"}},
		{"mismatch", "linux/amd64", "darwin/arm64", 2, []string{"linux/amd64", "darwin/arm64"}},
		{"unnamed", "", "linux/amd64", 2, []string{"benchmark_platform"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(fixtureOnPlatform(t, c.file), strings.NewReader(lexLine), &stdout, &stderr, "go1.27.2", c.running)
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
				t.Errorf("results printed before the platform check:\n%s", stdout.String())
			}
		})
	}
}

// fixtureWithOverride writes a budgets file whose reference platform is
// darwin/arm64 (top-level rows, max 1 -- deliberately tighter than
// BenchmarkLexTokens' real figure, so a run gated against them fails), plus
// one override row for linux/amd64 at overrideMax under
// platforms.linux/amd64.benchmarks (RES-36p).
func fixtureWithOverride(t *testing.T, overrideMax int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budgets.json")
	doc := `{"budgets_version": 1, "go_version": "go1.27.2", "benchmark_platform": "darwin/arm64", "benchmarks": [` +
		`{"name": "BenchmarkLexTokens", "measured_bytes_per_op": 1, "measured_allocs_per_op": 1, "bytes_per_op_max": 1, "allocs_per_op_max": 1}` +
		`], "platforms": {"linux/amd64": {"benchmarks": [` +
		`{"name": "BenchmarkLexTokens", "measured_bytes_per_op": ` + strconv.FormatInt(overrideMax, 10) + `, "measured_allocs_per_op": 750, "bytes_per_op_max": ` + strconv.FormatInt(overrideMax, 10) + `, "allocs_per_op_max": 750}` +
		`]}}}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRES36p_RunGatesAPlatformAgainstItsOwnMeasuredRows: once a platform
// other than benchmark_platform has its own rows committed
// (platforms.<plat>.benchmarks, RES-36p), the gate compares against those,
// not the reference platform's: BenchmarkLexTokens' real figures (128544 B,
// 750 allocs) fail against the reference fixture's deliberately tight max
// (1) but pass against linux/amd64's own, looser, measured max.
func TestRES36p_RunGatesAPlatformAgainstItsOwnMeasuredRows(t *testing.T) {
	path := fixtureWithOverride(t, 128544)
	var stdout, stderr bytes.Buffer
	code := run(path, strings.NewReader(lexLine), &stdout, &stderr, "go1.27.2", "linux/amd64")
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstdout %s\nstderr %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 benchmarks within budget") {
		t.Errorf("did not gate against the platform's own row:\n%s", stdout.String())
	}
	// The reference platform itself still reads its own (tight) row: proof
	// the pass above came from linux/amd64's own override, not a change that
	// made every platform read the loose row.
	stdout.Reset()
	stderr.Reset()
	if code := run(path, strings.NewReader(lexLine), &stdout, &stderr, "go1.27.2", "darwin/arm64"); code != 1 {
		t.Errorf("reference platform against its own tight row: exit %d, want 1\n%s", code, stderr.String())
	}
}

// TestRES36q_MeasurePrintsRowsWithNoBudgetsFileInput: -measure's rows are the
// run's own measurement, max equal to measured, read from no budgets file
// (RES-36p): it never refuses a toolchain or a platform, and prints nothing
// when nothing ran.
func TestRES36q_MeasurePrintsRowsWithNoBudgetsFileInput(t *testing.T) {
	got := Measure(map[string]Result{"BenchmarkB": {2000, 20}, "BenchmarkA": {1000, 10}})
	want := []budget.Benchmark{
		{Name: "BenchmarkA", MeasuredBytesPerOp: 1000, MeasuredAllocsPerOp: 10, BytesPerOpMax: 1000, AllocsPerOpMax: 10},
		{Name: "BenchmarkB", MeasuredBytesPerOp: 2000, MeasuredAllocsPerOp: 20, BytesPerOpMax: 2000, AllocsPerOpMax: 20},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Measure = %+v, want %+v (sorted, max == measured)", got, want)
	}
	var stdout, stderr bytes.Buffer
	if code := runMeasure(strings.NewReader(lexLine), &stdout, &stderr, "go1.99.9", "plan9/amd64"); code != 0 {
		t.Fatalf("exit %d, want 0\nstderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"name": "BenchmarkLexTokens"`) {
		t.Errorf("-measure printed no row for the one benchmark that ran:\n%s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runMeasure(strings.NewReader(""), &stdout, &stderr, "go1.27.2", fixturePlatform); code != 2 {
		t.Errorf("-measure with nothing parsed: exit %d, want 2", code)
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
			if err := os.WriteFile(path, []byte(`{"budgets_version": 1, "go_version": "go1.27.2", "benchmark_platform": "linux/amd64"`+c.benchmarks+`}`), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := run(path, strings.NewReader(c.input), &stdout, &stderr, "go1.27.2", "linux/amd64")
			if code != c.want || !strings.Contains(stderr.String(), c.says) {
				t.Errorf("exit %d, want %d naming %q\nstdout %s\nstderr %s", code, c.want, c.says, stdout.String(), stderr.String())
			}
		})
	}
}

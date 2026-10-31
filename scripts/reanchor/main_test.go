package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

var fixedNow = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

// work copies the fixture budgets file to a temp dir and returns its path
// and its bytes.
func work(t *testing.T) (string, []byte) {
	t.Helper()
	raw, err := os.ReadFile("testdata/budgets.json")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "budgets.json")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p, raw
}

func runTool(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, fixedNow)
	return code, out.String(), errb.String()
}

func macos(budgets string, extra ...string) []string {
	return append([]string{"--run", "41166204201", "--dir", "testdata/macos", "--platform", "darwin/arm64", "--budgets", budgets, "--no-generate"}, extra...)
}

func linux(budgets string, extra ...string) []string {
	return append([]string{"--run", "41166204201", "--dir", "testdata/linux", "--platform", "linux/amd64", "--budgets", budgets, "--no-generate"}, extra...)
}

func lines(b []byte) []string { return strings.Split(string(b), "\n") }

// diffLines lists the 0-based indexes of the lines that differ, failing the
// test if the line counts differ: a row stays one line.
func diffLines(t *testing.T, a, b []byte) []int {
	t.Helper()
	la, lb := lines(a), lines(b)
	if len(la) != len(lb) {
		t.Fatalf("line count %d became %d", len(la), len(lb))
	}
	var d []int
	for i := range la {
		if la[i] != lb[i] {
			d = append(d, i)
		}
	}
	return d
}

func TestSelectedBenchmarkRowTakesTheBestOfThree(t *testing.T) {
	p, before := work(t)
	code, out, errb := runTool(t, macos(p, "--rows", "RunPost/edit_314k")...)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	after, _ := os.ReadFile(p)
	// Only the row's own line changes (the entry naming another benchmark and
	// line 6 stay).
	d := diffLines(t, before, after)
	if len(d) != 1 {
		t.Fatalf("changed lines %v, want only the row's", d)
	}
	row := lines(after)[d[0]]
	// The whole run with the lowest B/op: 475956 B and 294 allocs, not the
	// 290 allocs of a run with more bytes.
	for _, want := range []string{
		`"measured_bytes_per_op": 475956, "measured_allocs_per_op": 294, "bytes_per_op_max": 475956, "allocs_per_op_max": 294`,
		`run 41166204201`, `macos-latest`, `macos-26-arm64 20261013.0424.1`, `go1.27.3`, `2026-10-31`,
		`GOMAXPROCS 3 of 3 CPUs`, `replaces 471379 B/op and 271 allocs/op`,
	} {
		if !strings.Contains(row, want) {
			t.Errorf("row lacks %q: %s", want, row)
		}
	}
	if strings.Contains(row, "provisional") {
		t.Errorf("the provisional basis survived: %s", row)
	}
	if !strings.HasSuffix(row, `"},`) {
		t.Errorf("row lost its line's shape: %s", row)
	}
	b, err := budget.Decode(after)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range b.Benchmarks {
		if r.BytesPerOpMax != r.MeasuredBytesPerOp || r.AllocsPerOpMax != r.MeasuredAllocsPerOp {
			t.Errorf("%s carries a margin: %+v", r.Name, r)
		}
	}
}

func TestEntryAllocFollowsItsBenchmarkRow(t *testing.T) {
	p, before := work(t)
	code, out, errb := runTool(t, macos(p, "--rows", "BenchmarkRunHook/bash_short")...)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	after, _ := os.ReadFile(p)
	if d := diffLines(t, before, after); len(d) != 2 {
		t.Fatalf("changed lines %v, want the row and its entry", d)
	}
	// Tie on 56093 B/op: the run with the fewer allocations, 388.
	if !strings.Contains(string(after), `"measured_bytes_per_op": 56093, "measured_allocs_per_op": 388`) ||
		!strings.Contains(string(after), `"benchmark": "BenchmarkRunHook/bash_short", "alloc_bytes_max": 56093,`) {
		t.Errorf("row and entry not in step:\n%s", after)
	}
	if !strings.Contains(out, "TestRES36f") {
		t.Errorf("no note about the entry: %s", out)
	}
}

func TestProvisionalSelectsOnlyProvisionalRows(t *testing.T) {
	p, before := work(t)
	code, out, errb := runTool(t, macos(p, "--provisional", "--kind", "bench")...)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	after, _ := os.ReadFile(p)
	if d := diffLines(t, before, after); len(d) != 1 || !strings.Contains(lines(after)[d[0]], "BenchmarkRunPost/edit_314k") {
		t.Errorf("changed lines %v", d)
	}
}

func TestLinuxRowsLiveInLineSixAndStayThere(t *testing.T) {
	p, before := work(t)
	code, out, errb := runTool(t, linux(p, "--all")...)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	after, _ := os.ReadFile(p)
	// Line 6 (index 5) holds the platforms object; it alone changes.
	if d := diffLines(t, before, after); len(d) != 1 || d[0] != 5 {
		t.Fatalf("changed lines %v, want [5]", d)
	}
	l6 := lines(after)[5]
	for _, want := range []string{
		// The darwin platform's text is untouched, escapes and all.
		`"set_from": "fixture \u00e9", "tolerance": 0.15`,
		`"entries": {"hook.bash_short": {"p50": 6.98, "p95": 6.98}`,
		// The two linux rows: one had a provisional basis, one none.
		`"measured_bytes_per_op": 410000, "measured_allocs_per_op": 255, "bytes_per_op_max": 410000, "allocs_per_op_max": 255, "basis": "re-anchored 2026-10-31 from run 41166204201's bench-linux artifact (ubuntu-latest, go1.27.3)`,
		`"measured_bytes_per_op": 50000, "measured_allocs_per_op": 380, "bytes_per_op_max": 50000, "allocs_per_op_max": 380, "basis": "re-anchored`,
		// RSS: p50 and p95 move together; a case budget is a bare number.
		`"entries": {"hook.bash_short": {"p50": 5.63, "p95": 5.63}}, "case_budgets": {"hook.400B": 5.63}`,
		// The RSS basis rides in set_from.
		`reanchor 2026-10-31: 2 RSS rows re-anchored from run 41166204201's Perf gates (linux) job (ubuntu-latest, go1.27.3)`,
		`hook.bash_short 5.59 to 5.63, hook.400B 5.59 to 5.63.`,
	} {
		if !strings.Contains(l6, want) {
			t.Errorf("line 6 lacks %q:\n%s", want, l6)
		}
	}
	if _, err := budget.Decode(after); err != nil {
		t.Fatal(err)
	}
}

func TestRSSRowsMoveByIDAndLeaveTheRest(t *testing.T) {
	p, before := work(t)
	code, out, errb := runTool(t, macos(p, "--rows", "hook.400B,post.bash_short")...)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	after, _ := os.ReadFile(p)
	if d := diffLines(t, before, after); len(d) != 1 || d[0] != 5 {
		t.Fatalf("changed lines %v, want [5]", d)
	}
	l6 := lines(after)[5]
	for _, want := range []string{`"post.bash_short": {"p50": 7.0, "p95": 7.0}`, `"hook.400B": 6.91, "hook.1MiB": 10.4}`, `"hook.bash_short": {"p50": 6.98, "p95": 6.98}`} {
		if !strings.Contains(l6, want) {
			t.Errorf("line 6 lacks %q:\n%s", want, l6)
		}
	}
	if !strings.Contains(l6, `fixture \u00e9 reanchor 2026-10-31`) {
		t.Errorf("set_from lost its escape or its place: %s", l6)
	}
}

func TestReportWritesNothingAndShowsOldArtifactDelta(t *testing.T) {
	p, before := work(t)
	code, out, errb := runTool(t, macos(p, "--report")...)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Error("--report wrote the file")
	}
	for _, want := range []string{
		"BenchmarkRunPost/edit_314k", "old 471379 B/op 271 allocs/op", "artifact 475956 B/op 294 allocs/op", "+0.97% B/op +8.49% allocs/op",
		"rss case  hook.400B", "old 6.95 MiB", "artifact 6.91 MiB", "-0.58%",
		"BenchmarkRunHook/bash_short",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "NewRowless") {
		t.Error("a benchmark without a row is not a row to report")
	}
}

func TestRefusals(t *testing.T) {
	p, before := work(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"toolchain", macos(p, "--all", "--toolchain", "go1.27.2"), "go1.27.3"},
		{"platform", append(linux(p, "--all"), "--dir", darwinAsLinux(t), "--toolchain", "go1.27.3"), "measured on darwin/arm64"},
		{"unmatched selector", macos(p, "--rows", "RunPost/nope"), "matching no row"},
		{"no selection", macos(p), "select rows"},
		{"bad run", append(macos(p, "--all"), "--run", "abc"), "numeric"},
	}
	for _, c := range cases {
		code, _, errb := runTool(t, c.args...)
		if code != 2 || !strings.Contains(errb, c.want) {
			t.Errorf("%s: exit %d, stderr %q, want 2 and %q", c.name, code, errb, c.want)
		}
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Error("a refused run wrote the file")
	}
}

func TestMissingArtifactValueWritesNothing(t *testing.T) {
	p, before := work(t)
	// Linux's artifact carries no BenchmarkRecap/100, which darwin's rows have;
	// a linux --rows naming a darwin-only row matches nothing, and an RSS id
	// absent from the linux log is missing.
	code, _, errb := runTool(t, linux(p, "--rows", "post.bash_short")...)
	if code != 2 {
		t.Errorf("exit %d: %s", code, errb)
	}
	// hook.400B is in linux's log; BenchmarkRunHook is in its bench; drop the log.
	dir := t.TempDir()
	raw, _ := os.ReadFile("testdata/linux/bench.txt")
	os.WriteFile(filepath.Join(dir, "bench.txt"), raw, 0o644)
	code, _, errb = runTool(t, "--run", "41166204201", "--dir", dir, "--platform", "linux/amd64", "--budgets", p, "--no-generate", "--rows", "hook.400B", "--toolchain", "go1.27.3")
	if code != 2 || !strings.Contains(errb, "not in the job log") {
		t.Errorf("exit %d: %s", code, errb)
	}
	// And with no log and no --toolchain, the toolchain is unknown: refused.
	code, _, errb = runTool(t, "--run", "41166204201", "--dir", dir, "--platform", "linux/amd64", "--budgets", p, "--no-generate", "--all")
	if code != 2 || !strings.Contains(errb, "toolchain is unknown") {
		t.Errorf("exit %d: %s", code, errb)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Error("a refused run wrote the file")
	}
}

// darwinAsLinux is a directory whose bench.txt is the darwin fixture's.
func darwinAsLinux(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := os.ReadFile("testdata/macos/bench-macos/bench.txt")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "bench.txt"), raw, 0o644)
	return dir
}

func TestOverFlagIsTheGatesTenPercent(t *testing.T) {
	if overFlag(100, 110, 5, 5) != "" || overFlag(100, 111, 5, 5) == "" || overFlag(100, 100, 100, 111) == "" || overFlag(100, 50, 5, 1) != "" {
		t.Error("overFlag is not over-by-more-than-ten-percent on either figure")
	}
}

func TestBudgetsToolchainMismatchRefused(t *testing.T) {
	p, raw := work(t)
	raw = bytes.Replace(raw, []byte(`"go_version": "go1.27.3"`), []byte(`"go_version": "go1.27.2"`), 1)
	os.WriteFile(p, raw, 0o644)
	code, _, errb := runTool(t, macos(p, "--all")...)
	if code != 2 || !strings.Contains(errb, "measured under go1.27.2 and this run is go1.27.3") {
		t.Errorf("exit %d: %s", code, errb)
	}
}

// The real test/perf/budgets.json: re-anchoring every darwin benchmark row
// from an artifact that carries the file's own figures changes nothing but
// each row's basis, on the same lines, line 6 included untouched.
func TestRealFileLayoutSurvivesAnAllRowsRewrite(t *testing.T) {
	realPath := filepath.Join("..", "..", "test", "perf", "budgets.json")
	raw, err := os.ReadFile(realPath)
	if err != nil {
		t.Skip(err)
	}
	b, err := budget.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var bench strings.Builder
	bench.WriteString("goos: darwin\ngoarch: arm64\n")
	for _, r := range b.Benchmarks {
		fmt.Fprintf(&bench, "%s-3 \t 20\t 1000 ns/op\t %d B/op\t %d allocs/op\n", r.Name, r.MeasuredBytesPerOp, r.MeasuredAllocsPerOp)
	}
	os.WriteFile(filepath.Join(dir, "bench.txt"), []byte(bench.String()), 0o644)
	p := filepath.Join(t.TempDir(), "budgets.json")
	os.WriteFile(p, raw, 0o644)
	code, out, errb := runTool(t, "--run", "41166204201", "--dir", dir, "--platform", "darwin/arm64", "--budgets", p,
		"--no-generate", "--all", "--kind", "bench", "--toolchain", b.GoVersion)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	after, _ := os.ReadFile(p)
	d := diffLines(t, raw, after)
	if len(d) != len(b.Benchmarks) {
		t.Errorf("%d lines changed for %d rows", len(d), len(b.Benchmarks))
	}
	basis := regexp.MustCompile(`, "basis": "(?:[^"\\]|\\.)*"\}`)
	la, lb := lines(raw), lines(after)
	for _, i := range d {
		if basis.ReplaceAllString(la[i], "}") != basis.ReplaceAllString(lb[i], "}") {
			t.Errorf("line %d changed beyond its basis:\n%s\n%s", i+1, la[i], lb[i])
		}
		if i == 5 {
			t.Error("line 6, the platforms object, changed")
		}
	}
	if _, err := budget.Decode(after); err != nil {
		t.Fatal(err)
	}
}

func TestNoEditsIsByteIdentical(t *testing.T) {
	raw, _ := os.ReadFile("testdata/budgets.json")
	if got := (&planned{}).apply(raw); !bytes.Equal(got, raw) {
		t.Error("an empty plan changed bytes")
	}
}

func TestPyStringIsJSONDumps(t *testing.T) {
	for in, want := range map[string]string{
		`a "b" \ c`:    `"a \"b\" \\ c"`,
		"caf\u00e9":    `"caf\u00e9"`,
		"<a> & \u2014": `"<a> & \u2014"`,
		"\U0001F600":   `"\ud83d\ude00"`,
		"x\ny\x01":     `"x\ny\u0001"`,
	} {
		if got := pyString(in); got != want {
			t.Errorf("pyString(%q) = %s, want %s", in, got, want)
		}
	}
	for v, want := range map[float64]string{7: "7.0", 6.95: "6.95", 10.41: "10.41", 0.1: "0.1"} {
		if got := pyFloat(v); got != want {
			t.Errorf("pyFloat(%v) = %s, want %s", v, got, want)
		}
	}
}

// reanchor parses bench output by benchgate's rule with its own copy of the
// line pattern; a change to one must be made to the other.
func TestBenchLinePatternMatchesBenchgates(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "benchgate", "main.go"))
	if err != nil {
		t.Skip(err)
	}
	if !strings.Contains(string(src), "const benchLinePattern = `"+benchLinePattern+"`") {
		t.Error("benchLinePattern differs from scripts/benchgate's")
	}
}

func TestParseLogReadsGatedAndOverReadings(t *testing.T) {
	a := &artifact{Readings: map[string]float64{}}
	log := "x rss_test.go:522: hook.400B: 5.63 MiB against 5.59 (+10%)\n" +
		"y rss_test.go:561: recap.stop: median peak RSS 8.90 MiB over budget 7.46 MiB (+10%)\n" +
		"z rss_test.go:131: probe.start: 5.00 MiB, report-only on linux/amd64: floor\n"
	if err := parseLog([]byte(log), a); err != nil {
		t.Fatal(err)
	}
	if a.Readings["hook.400B"] != 5.63 || a.Readings["recap.stop"] != 8.9 {
		t.Errorf("%v", a.Readings)
	}
	if _, ok := a.Readings["probe.start"]; ok {
		t.Error("a report-only reading anchored a row")
	}
	if err := parseLog([]byte(log+"w rss_test.go:522: hook.400B: 5.70 MiB against 5.59 (+10%)\n"), a); err == nil {
		t.Error("two values for one row were not refused")
	}
}

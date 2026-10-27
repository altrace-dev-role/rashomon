// Command benchgate compares `go test -bench` output against the benchmark
// rows of test/perf/budgets.json and exits 1 when any benchmark is over
// budget on B/op or allocs/op by more than the tolerance, when a budgeted
// benchmark did not run, or when a benchmark ran without a row (tech spec
// 8.2.6; RES-36). It exits 2, comparing nothing, when it runs under a
// toolchain other than the one budgets.json's go_version names, when the
// running platform has no rows of its own to compare against (neither the
// reference platform's rows nor its own measured override, RES-36p), when
// the file has no benchmark rows at all, or when the file or the output
// cannot be read. It reads the benchmark output from stdin, takes the best
// of repeated runs, and never looks at wall time.
//
//	go test -run '^$' -bench . -benchtime 20x -count 3 ./... | go run ./scripts/benchgate
//
// -measure skips the comparison and instead prints the rows this run's
// platform would need to gate itself: a JSON array, one Benchmark object per
// benchmark that ran, with bytes_per_op_max and allocs_per_op_max equal to
// what was measured (test/perf/budgets.json's own convention). It reads no
// budgets file, so it never refuses a toolchain or a platform; the printed
// rows name neither, and a human pastes them into
// test/perf/budgets.json's platforms.<goos>/<goarch>.benchmarks by hand
// (RES-36p) -- this command does not write that file itself, so a row
// reaches it only after someone has read where it came from.
//
//	go test -run '^$' -bench . -benchtime 20x -count 3 ./... | go run ./scripts/benchgate -measure
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// tolerance is the fraction over budget that still passes (archive 85.3.13).
const tolerance = 0.10

func main() {
	measure := flag.Bool("measure", false, "print this platform's own benchmark rows instead of comparing")
	flag.Parse()
	if *measure {
		os.Exit(runMeasure(os.Stdin, os.Stdout, os.Stderr, runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH))
		return
	}
	os.Exit(run("test/perf/budgets.json", os.Stdin, os.Stdout, os.Stderr, runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH))
}

// run is the gate: 0 when every benchmark is within budget, 1 when one is
// not, 2 when nothing could be compared. The toolchain and the platform are
// both checked before any result is read, because the rows are one
// toolchain's figures taken on one platform (CheckToolchain, CheckPlatform).
func run(budgetsPath string, in io.Reader, stdout, stderr io.Writer, goVersion, platform string) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "benchgate:", err)
		return 2
	}
	raw, err := os.ReadFile(budgetsPath)
	if err != nil {
		return fail(err)
	}
	b, err := budget.Decode(raw)
	if err != nil {
		return fail(err)
	}
	if err := b.CheckToolchain(goVersion); err != nil {
		return fail(err)
	}
	if err := b.CheckPlatform(platform); err != nil {
		return fail(err)
	}
	benchmarks := b.BenchmarksFor(platform)
	if len(benchmarks) == 0 {
		return fail(fmt.Errorf("%s has no benchmark rows, so there is nothing to compare", budgetsPath))
	}
	results, err := Parse(in)
	if err != nil {
		return fail(err)
	}
	problems := Compare(results, benchmarks, tolerance)
	for _, name := range sortedNames(results) {
		r := results[name]
		fmt.Fprintf(stdout, "%-48s %10d B/op %6d allocs/op\n", name, r.BytesPerOp, r.AllocsPerOp)
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(stderr, "benchgate:", p)
		}
		return 1
	}
	fmt.Fprintf(stdout, "benchgate: %d benchmarks within budget\n", len(benchmarks))
	return 0
}

// runMeasure prints in reads as rows for goVersion/platform: 2 when nothing
// ran or the output could not be parsed, 0 otherwise. It never reads
// budgets.json, so a row it prints carries no claim about any file; Measure
// is the pure part, kept separate so a test can check the rows without
// capturing stdout.
func runMeasure(in io.Reader, stdout, stderr io.Writer, goVersion, platform string) int {
	results, err := Parse(in)
	if err != nil {
		fmt.Fprintln(stderr, "benchgate:", err)
		return 2
	}
	if len(results) == 0 {
		fmt.Fprintln(stderr, "benchgate: -measure read no benchmark output, so there is nothing to measure")
		return 2
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(Measure(results)); err != nil {
		fmt.Fprintln(stderr, "benchgate:", err)
		return 2
	}
	fmt.Fprintf(stderr, "benchgate: measured %d benchmarks on %s under %s; paste the array above into "+
		"test/perf/budgets.json's platforms.%s.benchmarks\n", len(results), platform, goVersion, platform)
	return 0
}

// Measure turns results into the rows test/perf/budgets.json commits for a
// platform: a Benchmark per name, sorted, whose max is the measurement
// itself (the file's own "a row's max equals its measurement" rule), never
// a figure read from an existing budgets file -- -measure takes no budgets
// file as input, so there is nothing here to inflate or shrink one by.
func Measure(results map[string]Result) []budget.Benchmark {
	rows := make([]budget.Benchmark, 0, len(results))
	for _, name := range sortedNames(results) {
		r := results[name]
		rows = append(rows, budget.Benchmark{
			Name:                name,
			MeasuredBytesPerOp:  r.BytesPerOp,
			MeasuredAllocsPerOp: r.AllocsPerOp,
			BytesPerOpMax:       r.BytesPerOp,
			AllocsPerOpMax:      r.AllocsPerOp,
		})
	}
	return rows
}

// Result is a benchmark's best allocation figures over its runs.
type Result struct {
	BytesPerOp  int64
	AllocsPerOp int64
}

// benchLinePattern is one `go test -bench` result line with -benchmem figures:
// BenchmarkName-8   20   123 ns/op   456 B/op   7 allocs/op
// Any custom metrics a benchmark reports (MB/s, B/token, tokens/op) sit
// between ns/op and B/op and are skipped. The optional -N suffix is Go's
// GOMAXPROCS marker; a benchmark whose own name ended in -digits would lose
// them here under GOMAXPROCS=1 and read "budgeted but did not run", which
// fails closed; no benchmark is named that way. It is compiled per Parse,
// not at package scope (RES-2's rule, kept here too).
const benchLinePattern = `^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+[\d.]+ ns/op(?:\s+[\d.]+ [A-Za-z0-9/_-]+)*?\s+(\d+) B/op\s+(\d+) allocs/op`

// maxOutputLine bounds one line of `go test -bench` output. A result line is
// under 200 bytes; the bound is for what a test logs between them, which must
// not end the parse. A line over 1 MiB does, with bufio.ErrTooLong, and the
// gate fails closed. The scan starts at bufio's default 64 KiB and grows only
// for such a line.
const maxOutputLine = 1 << 20

// Parse reads benchmark output and keeps, per benchmark, the one run with
// the lowest B/op (ties broken by allocs/op) across repeated runs -- the
// whole run, not each field's own minimum independently, which could report
// a B/op from one run beside an allocs/op from another and hide a
// regression that showed up in only one or two of the runs. A line that
// starts with "Benchmark" and carries ns/op but no B/op is an error: a
// benchmark without ReportAllocs cannot be gated and must not read as
// passing.
func Parse(r io.Reader) (map[string]Result, error) {
	benchLine := regexp.MustCompile(benchLinePattern)
	best := map[string]Result{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxOutputLine)
	for sc.Scan() {
		ln := sc.Text()
		if !strings.HasPrefix(ln, "Benchmark") || !strings.Contains(ln, "ns/op") {
			continue
		}
		m := benchLine.FindStringSubmatch(ln)
		if m == nil {
			return nil, fmt.Errorf("benchmark line without allocation figures: %q", ln)
		}
		bytesPerOp, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return nil, err
		}
		allocs, err := strconv.ParseInt(m[3], 10, 64)
		if err != nil {
			return nil, err
		}
		name := m[1]
		run := Result{bytesPerOp, allocs}
		if prev, ok := best[name]; !ok || betterRun(run, prev) {
			best[name] = run
		}
	}
	return best, sc.Err()
}

// betterRun reports whether a is the run Parse should keep over b: the
// lower B/op, the lower allocs/op on a tie, a whole run either way, never a
// blend of the two.
func betterRun(a, b Result) bool {
	if a.BytesPerOp != b.BytesPerOp {
		return a.BytesPerOp < b.BytesPerOp
	}
	return a.AllocsPerOp < b.AllocsPerOp
}

// Compare lists every budget a result breaks, every budgeted benchmark that
// did not run, and every benchmark that ran without a row: the static check
// that every benchmark has a row reads names from source, and a b.Run inside
// a helper or a duplicate name (which Go runs as name#01) would otherwise run
// ungated. A budget of zero fails closed: it means the row was never filled
// in.
func Compare(results map[string]Result, budgets []budget.Benchmark, tol float64) []string {
	var problems []string
	budgeted := make(map[string]bool, len(budgets))
	for _, b := range budgets {
		budgeted[b.Name] = true
		r, ok := results[b.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: budgeted but did not run", b.Name))
			continue
		}
		// Zero B/op and zero allocs/op are valid measured results for
		// allocation-free benchmarks and are still enforced by over below.
		if b.BytesPerOpMax < 0 || b.AllocsPerOpMax < 0 {
			problems = append(problems, fmt.Sprintf("%s: budget row has no figures", b.Name))
			continue
		}
		if over(r.BytesPerOp, b.BytesPerOpMax, tol) {
			problems = append(problems, fmt.Sprintf("%s: %d B/op over budget %d", b.Name, r.BytesPerOp, b.BytesPerOpMax))
		}
		if over(r.AllocsPerOp, b.AllocsPerOpMax, tol) {
			problems = append(problems, fmt.Sprintf("%s: %d allocs/op over budget %d", b.Name, r.AllocsPerOp, b.AllocsPerOpMax))
		}
	}
	for _, name := range sortedNames(results) {
		if !budgeted[name] {
			problems = append(problems, fmt.Sprintf("%s: ran without a budget row", name))
		}
	}
	return problems
}

func over(got, max int64, tol float64) bool {
	return float64(got) > float64(max)*(1+tol)
}

func sortedNames(m map[string]Result) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

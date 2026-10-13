// Command benchgate compares `go test -bench` output against the benchmark
// rows of test/perf/budgets.json and exits 1 when any benchmark is over
// budget on B/op or allocs/op by more than the tolerance, when a budgeted
// benchmark did not run, or when a benchmark ran without a row (tech spec
// 8.2.6; RES-36). It exits 2, comparing nothing, when it runs under a
// toolchain other than the one budgets.json's go_version names, when the file
// has no benchmark rows, or when the file or the output cannot be read. It reads the benchmark output from
// stdin, takes the best of repeated runs, and never looks at wall time.
//
//	go test -run '^$' -bench . -benchtime 20x -count 3 ./... | go run ./scripts/benchgate
package main

import (
	"bufio"
	"encoding/json"
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
	os.Exit(run("test/perf/budgets.json", os.Stdin, os.Stdout, os.Stderr, runtime.Version()))
}

// run is the gate: 0 when every benchmark is within budget, 1 when one is
// not, 2 when nothing could be compared. The toolchain is checked before any
// result is read, because the rows are one toolchain's figures.
func run(budgetsPath string, in io.Reader, stdout, stderr io.Writer, goVersion string) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "benchgate:", err)
		return 2
	}
	raw, err := os.ReadFile(budgetsPath)
	if err != nil {
		return fail(err)
	}
	var b budget.Budgets
	if err := json.Unmarshal(raw, &b); err != nil {
		return fail(err)
	}
	if err := b.CheckToolchain(goVersion); err != nil {
		return fail(err)
	}
	if len(b.Benchmarks) == 0 {
		return fail(fmt.Errorf("%s has no benchmark rows, so there is nothing to compare", budgetsPath))
	}
	results, err := Parse(in)
	if err != nil {
		return fail(err)
	}
	problems := Compare(results, b.Benchmarks, tolerance)
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
	fmt.Fprintf(stdout, "benchgate: %d benchmarks within budget\n", len(b.Benchmarks))
	return 0
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

// Parse reads benchmark output and keeps the best (lowest) B/op and
// allocs/op per benchmark across repeated runs. A line that starts with
// "Benchmark" and carries ns/op but no B/op is an error: a benchmark without
// ReportAllocs cannot be gated and must not read as passing.
func Parse(r io.Reader) (map[string]Result, error) {
	benchLine := regexp.MustCompile(benchLinePattern)
	out := map[string]Result{}
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
		if prev, ok := out[name]; ok {
			if bytesPerOp > prev.BytesPerOp {
				bytesPerOp = prev.BytesPerOp
			}
			if allocs > prev.AllocsPerOp {
				allocs = prev.AllocsPerOp
			}
		}
		out[name] = Result{bytesPerOp, allocs}
	}
	return out, sc.Err()
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
		if b.BytesPerOpMax <= 0 || b.AllocsPerOpMax <= 0 {
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

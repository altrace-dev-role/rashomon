// Command benchgate compares `go test -bench` output against the benchmark
// rows of test/perf/budgets.json and exits 1 when any benchmark is over
// budget on B/op or allocs/op by more than the tolerance, or when a budgeted
// benchmark did not run (tech spec 8.2.6; RES-36). It reads the benchmark
// output from stdin, takes the best of repeated runs, and never looks at
// wall time.
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
	"sort"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// tolerance is the fraction over budget that still passes (archive 85.3.13).
const tolerance = 0.10

func main() {
	raw, err := os.ReadFile("test/perf/budgets.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchgate:", err)
		os.Exit(2)
	}
	var b budget.Budgets
	if err := json.Unmarshal(raw, &b); err != nil {
		fmt.Fprintln(os.Stderr, "benchgate:", err)
		os.Exit(2)
	}
	results, err := Parse(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchgate:", err)
		os.Exit(2)
	}
	problems := Compare(results, b.Benchmarks, tolerance)
	for _, name := range sortedNames(results) {
		r := results[name]
		fmt.Printf("%-48s %10d B/op %6d allocs/op\n", name, r.BytesPerOp, r.AllocsPerOp)
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "benchgate:", p)
		}
		os.Exit(1)
	}
	fmt.Printf("benchgate: %d benchmarks within budget\n", len(b.Benchmarks))
}

// Result is a benchmark's best allocation figures over its runs.
type Result struct {
	BytesPerOp  int64
	AllocsPerOp int64
}

// benchLine is one `go test -bench` result line with -benchmem figures:
// BenchmarkName-8   20   123 ns/op   456 B/op   7 allocs/op
// Any custom metrics a benchmark reports (MB/s, B/token, tokens/op) sit
// between ns/op and B/op and are skipped. The optional -N suffix is Go's
// GOMAXPROCS marker; a benchmark whose own name ended in -digits would lose
// them here under GOMAXPROCS=1 and read "budgeted but did not run", which
// fails closed; no benchmark is named that way.
var benchLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+[\d.]+ ns/op(?:\s+[\d.]+ [A-Za-z0-9/_-]+)*?\s+(\d+) B/op\s+(\d+) allocs/op`)

// Parse reads benchmark output and keeps the best (lowest) B/op and
// allocs/op per benchmark across repeated runs. A line that starts with
// "Benchmark" and carries ns/op but no B/op is an error: a benchmark without
// ReportAllocs cannot be gated and must not read as passing.
func Parse(r io.Reader) (map[string]Result, error) {
	out := map[string]Result{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
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

// Compare lists every budget a result breaks and every budgeted benchmark
// that did not run. A budget of zero fails closed: it means the row was never
// filled in.
func Compare(results map[string]Result, budgets []budget.Benchmark, tol float64) []string {
	var problems []string
	for _, b := range budgets {
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

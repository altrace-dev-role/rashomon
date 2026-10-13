// Package perf holds the resource gates: the pure comparisons the gates
// decide with (this file, built and tested everywhere) and the harness that
// measures the release binary (rss_test.go, behind the perf build tag).
package perf

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OverBudget reports whether measured exceeds budget by more than tolerance
// (a fraction: 0.10 is ten percent). A budget of zero or less is a defect in
// the budgets file and reads as over, never as unlimited.
func OverBudget(measured, budget, tolerance float64) bool {
	if budget <= 0 {
		return true
	}
	return measured > budget*(1+tolerance)
}

// RatioTolerance is the tolerance RES-30's per-process ratio gets where the
// harness runs, read through getenv: the platform's runner tolerance on GitHub
// Actions, which sets GITHUB_ACTIONS to "true" and where macos-latest's ratio
// has not been measured yet, and none anywhere else, because the ratio was
// measured on the reference machine and holds there as stated (ruling
// 2026-10-13). A machine that is neither is held to the ratio as stated,
// which fails closed.
func RatioTolerance(platformTolerance float64, getenv func(string) string) float64 {
	if getenv("GITHUB_ACTIONS") == "true" {
		return platformTolerance
	}
	return 0
}

// Tolerance is the fraction over budget a platform's runner is allowed at
// now: its tolerance until the date it names, its tolerance_after from then.
// An unparseable date fails closed to the tighter of the two.
func Tolerance(tolerance float64, until string, after float64, now time.Time) float64 {
	tight := tolerance
	if after < tight {
		tight = after
	}
	if until == "" {
		return tight
	}
	limit, err := time.Parse("2006-01-02", until)
	if err != nil {
		return tight
	}
	if now.Before(limit) {
		return tolerance
	}
	return after
}

// ToleranceNote explains a failure that the schedule, not the code, produced:
// when now is past the date and the measurement would have passed under the
// earlier tolerance, the note says so, so a red on the switch date reads as
// the schedule tightening and not as a regression. Empty otherwise.
func ToleranceNote(measured, budget, tolerance float64, until string, after float64, now time.Time) string {
	if until == "" || after >= tolerance {
		return ""
	}
	current := Tolerance(tolerance, until, after, now)
	if current != after {
		return ""
	}
	if OverBudget(measured, budget, after) && !OverBudget(measured, budget, tolerance) {
		return "tolerance tightened per budgets.json tolerance_until " + until + " (" + percent(tolerance) + " to " + percent(after) + ")"
	}
	return ""
}

func percent(f float64) string { return strconv.Itoa(int(f*100+0.5)) + "%" }

// Median of xs; false with none.
func Median(xs []float64) (float64, bool) {
	if len(xs) == 0 {
		return 0, false
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if n := len(s); n%2 == 1 {
		return s[n/2], true
	} else {
		return (s[n/2-1] + s[n/2]) / 2, true
	}
}

// InitTrace is what GODEBUG=inittrace=1,gctrace=1 reported for a process:
// the bytes and allocations of every package init summed, and whether the
// runtime printed a GC line before main (RES-2).
type InitTrace struct {
	Packages int
	Bytes    int64
	Allocs   int64
	GCLines  int
}

// ParseInitTrace reads the runtime's trace from stderr. Lines it does not
// recognise are ignored only when they are not init or gc lines; an init
// line it cannot parse is an error, so a format change fails the gate
// rather than reading as a small init.
func ParseInitTrace(stderr string) (InitTrace, error) {
	var t InitTrace
	sc := bufio.NewScanner(strings.NewReader(stderr))
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(ln, "gc "):
			t.GCLines++
		case strings.HasPrefix(ln, "init "):
			// init <pkg> @<t> ms, <d> ms clock, <b> bytes, <n> allocs: eleven
			// fields, the last at index 10, so fewer than eleven is refused
			// before any is read.
			f := strings.Fields(ln)
			if len(f) < 11 || f[8] != "bytes," || f[10] != "allocs" {
				return t, fmt.Errorf("unrecognised init line: %q", ln)
			}
			b, err := strconv.ParseInt(f[7], 10, 64)
			if err != nil {
				return t, fmt.Errorf("init line bytes: %q", ln)
			}
			n, err := strconv.ParseInt(f[9], 10, 64)
			if err != nil {
				return t, fmt.Errorf("init line allocs: %q", ln)
			}
			t.Packages++
			t.Bytes += b
			t.Allocs += n
		}
	}
	return t, sc.Err()
}

// InitBudgetBytes is RES-2's bound on process init: 64 KiB, and no GC cycle.
const InitBudgetBytes = 64 << 10

// Burst is what a RES-30 run left behind: the records Ways concurrent
// hook-and-post pairs on one session landed in the ordered stream, by kind,
// with their seqs; the spill file's lines and the coverage records that gave
// up on a lock; each pair's peak footprint in MiB, the larger of its hook's
// and its post's, which run one after the other; and OneWayPeakMiB, the same
// figure for one pair run alone, the median of OneWaySamples runs, the 1-way
// figure the per-process rule holds every pair in the burst to.
type Burst struct {
	Ways                                int
	Declarations, Terminals, Executions int
	CallCoverage, PostCoverage          int
	Seqs                                []int64
	Spilled, LockTimeouts               int
	PairPeakMiB                         []float64
	OneWayPeakMiB                       float64
}

// PerProcessPeakRatio is RES-30's per-process rule, restated from
// measurement (ruling 2026-10-13, P-09): the largest pair peak in a 64-way
// burst is at most 1.25 times the 1-way figure, the median of OneWaySamples
// solo pairs. The largest stands for every process, so one process in the
// tail over the ratio fails the burst however typical the rest.
//
// Measured on the reference machine (Apple M3, 8 GB, go1.27.2, release
// build, darwin peak footprint, load average 3.4 to 3.9) with GOMAXPROCS
// unchanged: the largest pair peak ran 1.08 to 1.21 times the solo median
// over six runs with nine solo pairs, and 1.10 to 1.19 times over eleven runs
// with 25 (largest 4.02 to 4.31 MiB against solo medians of 3.61 to 3.69
// MiB), while the burst's median stayed at 1.02 to 1.05 times. The tail, not
// the typical process, grows with contention, which is why 1.1 failed on
// most runs and why the median is not what is judged. GOMAXPROCS(1) in hook
// and post held the largest to 1.03 to 1.09 times over twenty runs with 25
// solo pairs, and was rejected: it changes the runtime for every user to
// suit a gate, and the spec keeps GOMAXPROCS unchanged (85.3.8).
//
// On the reference machine both sides are measured in the same run, so the
// ratio holds as stated, with no tolerance. macos-latest is a different
// machine whose ratio has not been measured; until it is, a CI runner gets
// its platform's tolerance on the ratio (RatioTolerance).
// test/perf/budgets.json's parallel row records the same figures.
const PerProcessPeakRatio = 1.25

// OneWaySamples is how many solo pairs the 1-way figure is the median of.
// It was nine; 25 measure the same p50 more closely without moving the rule
// (over eleven runs with 25 the median moved 3.61 to 3.69 MiB), and each
// pair costs about 20 ms.
const OneWaySamples = 25

// LargestMiB is the largest per-process peak in the burst: the largest pair
// peak, each already the larger of its hook's and its post's. Zero with none.
// It is what the per-process rule judges.
func (b Burst) LargestMiB() float64 {
	largest := 0.0
	for _, m := range b.PairPeakMiB {
		largest = max(largest, m)
	}
	return largest
}

// Problems lists every way the burst missed RES-30 against a summed-footprint
// budget and its tolerance, and the per-process ratio and the tolerance on it
// (zero where the ratio has been measured; see PerProcessPeakRatio): a record of any kind that did not land in the
// ordered stream, a seq run that is not 1..n without a gap or a repeat, a
// spilled line or a lock timeout (each a hook that waited out its lock
// budget), and every footprint problem footprintProblems names. A burst of no
// pairs measured nothing and fails.
func (b Burst) Problems(budgetMiB, tolerance, ratioTolerance float64) []string {
	var out []string
	if b.Ways <= 0 {
		return []string{"the burst ran no pairs"}
	}
	for _, c := range []struct {
		what string
		got  int
	}{
		{"declarations", b.Declarations}, {"terminals", b.Terminals}, {"executions", b.Executions},
		{"call coverage records", b.CallCoverage}, {"post coverage records", b.PostCoverage},
	} {
		if c.got != b.Ways {
			out = append(out, fmt.Sprintf("%d of %d %s landed", c.got, b.Ways, c.what))
		}
	}
	if n := b.Declarations + b.Terminals + b.Executions; len(b.Seqs) != n {
		out = append(out, fmt.Sprintf("%d seqs for %d records in the ordered stream", len(b.Seqs), n))
	}
	seqs := append([]int64(nil), b.Seqs...)
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	for i, s := range seqs {
		if s != int64(i+1) {
			out = append(out, fmt.Sprintf("seq at position %d is %d: the ordered stream has a gap or a repeat", i, s))
			break
		}
	}
	if b.Spilled > 0 || b.LockTimeouts > 0 {
		out = append(out, fmt.Sprintf("%d spilled lines and %d lock timeouts, want none", b.Spilled, b.LockTimeouts))
	}
	return append(out, b.footprintProblems(budgetMiB, tolerance, ratioTolerance)...)
}

// footprintProblems is the memory half of RES-30: a pair left unmeasured, a
// summed footprint over budget, a 1-way figure that was never measured, and
// any pair whose peak under the burst is over PerProcessPeakRatio times it,
// plus ratioTolerance, the largest standing for them all. The sum
// alone is loose (archive 85.4.4 budgets a supervisor parent that does not
// exist before P-23), so footprint that grows with contention is what the
// ratio is there to catch; footprint that grows everywhere is RES-28's.
func (b Burst) footprintProblems(budgetMiB, tolerance, ratioTolerance float64) []string {
	var out []string
	sum := 0.0
	for _, m := range b.PairPeakMiB {
		if m <= 0 {
			out = append(out, "a pair's peak footprint was not measured")
			break
		}
		sum += m
	}
	if len(b.PairPeakMiB) != b.Ways {
		out = append(out, fmt.Sprintf("%d of %d pairs measured", len(b.PairPeakMiB), b.Ways))
	}
	if OverBudget(sum, budgetMiB, tolerance) {
		out = append(out, fmt.Sprintf("summed peak footprint %.1f MiB over budget %.1f MiB (+%.0f%%)", sum, budgetMiB, tolerance*100))
	}
	largest := b.LargestMiB()
	switch {
	case b.OneWayPeakMiB <= 0:
		out = append(out, "the 1-way per-process peak was not measured, so the per-process rule cannot be judged")
	case OverBudget(largest, PerProcessPeakRatio*b.OneWayPeakMiB, ratioTolerance):
		median, _ := Median(b.PairPeakMiB)
		out = append(out, fmt.Sprintf("largest per-process peak %.2f MiB is %.3fx the 1-way figure, %.2f MiB, over %.2fx (+%.0f%%; burst median %.2f MiB)",
			largest, largest/b.OneWayPeakMiB, b.OneWayPeakMiB, PerProcessPeakRatio, ratioTolerance*100, median))
	}
	return out
}

// footprintLine is the line of /usr/bin/time -l's report on darwin that
// carries a process's peak memory footprint, in bytes.
const footprintLine = "peak memory footprint"

// ParseFootprint reads the peak memory footprint out of /usr/bin/time -l's
// report. It is the darwin figure to sum across concurrent processes, because
// it leaves out shared clean pages such as the binary's text (archive
// 85.3.13). A report without the line is not a zero: ok is false.
func ParseFootprint(report string) (bytes int64, ok bool) {
	sc := bufio.NewScanner(strings.NewReader(report))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 4 && strings.Join(f[1:], " ") == footprintLine {
			n, err := strconv.ParseInt(f[0], 10, 64)
			return n, err == nil
		}
	}
	return 0, false
}

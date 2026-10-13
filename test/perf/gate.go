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
// figure for one pair run alone, the median of OneWaySamples runs, which is
// the 1-way figure the per-process rule holds every pair in the burst to.
type Burst struct {
	Ways                                int
	Declarations, Terminals, Executions int
	CallCoverage, PostCoverage          int
	Seqs                                []int64
	Spilled, LockTimeouts               int
	PairPeakMiB                         []float64
	OneWayPeakMiB                       float64
}

// PerProcessPeakRatio is RES-30's per-process rule as the spec states it
// (archive 85.4.4, 85.6, the RES-30 row): the per-process peak at 64-way is
// at most 1.1 times the 1-way figure. It is judged on every process, so the
// largest of the burst decides, against the median of the solo runs, the
// p50 the per-process budgets are written in.
//
// The rule holds with little room, and that is measured, not assumed. On an
// Apple M3 under load about 4 to 5 (2026-10-13), with the Go runtime's
// default of one P per core, solo pairs had a median peak of 3.58 to 3.63 MiB
// and the largest of 128 burst processes reached 4.02 to 4.11 MiB, 1.11 to
// 1.14 times it, in each of five runs (an earlier run reached 4.33 MiB, 1.21
// times), while the burst's median stayed within 1.04 to 1.06 times: the tail, not the typical process, grew with
// contention, and a median-against-median reading hid it. hook and post now
// run on one P (cmd/rashomon, oneProc): solo pairs 3.42 to 3.53 MiB, the
// burst median within 0.05 MiB of them, and the largest 1.03 to 1.09 times
// in fourteen runs of fourteen with OneWaySamples at 25. That is as little as
// 0.6 percent of room. A run over the ratio is a real red, never a flake to
// retry: K5 reverts the PR, and the ratio does not move without the spec
// owner.
//
// Both sides are measured on the same machine in the same run, so the runner
// tolerance, which covers one machine against another, does not apply.
const PerProcessPeakRatio = 1.1

// OneWaySamples is how many solo pairs the 1-way figure is the median of.
// With nine, the median itself moved 3.47 to 3.59 MiB from run to run, as
// much as the room the rule leaves, and the one red in fourteen runs measured
// on one P came from a low median rather than a high tail. More samples
// measure the same p50 more closely; they do not move the rule. Each pair
// costs about 20 ms.
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
// budget and its tolerance: a record of any kind that did not land in the
// ordered stream, a seq run that is not 1..n without a gap or a repeat, a
// spilled line or a lock timeout (each a hook that waited out its lock
// budget), and every footprint problem footprintProblems names. A burst of no
// pairs measured nothing and fails.
func (b Burst) Problems(budgetMiB, tolerance float64) []string {
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
	return append(out, b.footprintProblems(budgetMiB, tolerance)...)
}

// footprintProblems is the memory half of RES-30: a pair left unmeasured, a
// summed footprint over budget, a 1-way figure that was never measured, and
// any pair whose peak under the burst is over PerProcessPeakRatio times it,
// the largest standing for them all. The sum
// alone is loose (archive 85.4.4 budgets a supervisor parent that does not
// exist before P-23), so footprint that grows with contention is what the
// ratio is there to catch; footprint that grows everywhere is RES-28's.
func (b Burst) footprintProblems(budgetMiB, tolerance float64) []string {
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
	case largest > PerProcessPeakRatio*b.OneWayPeakMiB:
		median, _ := Median(b.PairPeakMiB)
		out = append(out, fmt.Sprintf("largest per-process peak %.2f MiB is %.3fx the 1-way figure, %.2f MiB, over %.1fx (burst median %.2f MiB)",
			largest, largest/b.OneWayPeakMiB, b.OneWayPeakMiB, PerProcessPeakRatio, median))
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

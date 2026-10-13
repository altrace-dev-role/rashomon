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

	"github.com/altrace-dev-role/rashomon/internal/budget"
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

// referencePlatform is the platform every entry row and RES-28 case was
// measured or specified on. Its own Platform.Entries and CaseBudgets stay
// empty because its rows already are its data; gating it against its own
// row is self-consistent, never a cross-platform guess.
const referencePlatform = "darwin/arm64"

// belowFloor names, per platform, entries the reaper cannot attribute a
// reading to there: the reaper's own floor on that runner is at or above the
// entry's own peak (peakrss.attributable), so no reading is possible, not
// merely unmeasured. TestRES28b_ reports these rather than gating them.
var belowFloor = map[string][]string{
	"linux/amd64": {"hook.bash_short", "post.bash_short"},
}

// entryBudget is the p50 TestRES28b_ gates e against on plat, and whether
// that is a real gate or a report-only comparison. A gate is real when plat
// has its own reading for e (overrides) or plat is the reference platform,
// whose row is that reading. Otherwise gating against the reference row
// would assume plat reads no higher, an assumption Linux's first run
// (archive 85.4.3) disproved for several entries it does have a floor for;
// why names the reason for a caller to log.
func entryBudget(plat string, e budget.Entry, overrides map[string]budget.RSS) (budgetP50 float64, gated bool, why string) {
	if o, ok := overrides[e.ID]; ok {
		return o.P50, true, ""
	}
	if plat == referencePlatform {
		return e.MaxRSSMiBP50, true, ""
	}
	for _, id := range belowFloor[plat] {
		if id == e.ID {
			return e.MaxRSSMiBP50, false, fmt.Sprintf(
				"this entry's own peak does not exceed the reaper's own floor on %s, so no reading here can be attributed to it yet", plat)
		}
	}
	return e.MaxRSSMiBP50, false, fmt.Sprintf(
		"no %s reading recorded yet; gating against %s's row would assume %s reads no higher, which no measurement here has shown", plat, referencePlatform, plat)
}

// caseBudget is the same decision as entryBudget, for a RES-28 case: its
// BudgetMiB is RES-28's own line, which the case's basis is darwin/arm64's
// measurement against that line, not a cross-platform assumption on its
// own merit.
func caseBudget(plat string, c budget.RSSCase, overrides map[string]float64) (budgetMiB float64, gated bool, why string) {
	if v, ok := overrides[c.ID]; ok {
		return v, true, ""
	}
	if plat == referencePlatform {
		return c.BudgetMiB, true, ""
	}
	return c.BudgetMiB, false, fmt.Sprintf(
		"no %s reading recorded yet; gating against RES-28's line, measured against %s, would assume %s reads no higher, which no measurement here has shown", plat, referencePlatform, plat)
}

// Burst is what a RES-30 run left behind: the records Ways concurrent
// hook-and-post pairs on one session landed in the ordered stream, by kind,
// with their seqs; the spill file's lines and the coverage records that gave
// up on a lock; each pair's peak footprint in MiB, the larger of its hook's
// and its post's, which run one after the other; and OneWayPeakMiB, the same
// figure for one pair run alone, the median of OneWaySamples runs, which is
// what the per-process rule compares the burst's median pair peak against.
type Burst struct {
	Ways                                int
	Declarations, Terminals, Executions int
	CallCoverage, PostCoverage          int
	Seqs                                []int64
	Spilled, LockTimeouts               int
	PairPeakMiB                         []float64
	OneWayPeakMiB                       float64
}

// PerProcessPeakRatio is RES-30's per-process rule (archive 85.4.4, 85.6):
// the per-process peak at 64-way is at most 1.1 times the 1-way figure.
//
// It is judged median against median. The 1-way figure is a p50, as the
// per-process budgets it comes from are, and the rule is what lets the summed
// budget be 64 times that p50: it says the typical process does not grow when
// sixty-four contend. The burst's largest process is a different statistic,
// the maximum of 128, and is reported beside the verdict, not gated: measured
// on an Apple M3 (2026-10-13), solo pair peaks ran 3.50 to 3.74 MiB with a
// median of 3.69, the 64-way median was about 3.70, and the top four or five
// of 128 processes reached 4.03 to 4.17 MiB, about 1.13 times the 1-way
// median, which a max-against-median rule would fail on every run.
//
// Both sides are measured on the same machine in the same run, so the runner
// tolerance, which covers one machine against another, does not apply.
const PerProcessPeakRatio = 1.1

// OneWaySamples is how many solo pairs the 1-way figure is the median of.
const OneWaySamples = 9

// LargestMiB is the largest per-process peak in the burst: the largest pair
// peak, each already the larger of its hook's and its post's. Zero with none.
// It is reported beside the per-process verdict, which judges the median.
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
// summed footprint over budget, a 1-way figure that was never measured, and a
// median pair peak under the burst over PerProcessPeakRatio times it. The sum
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
	median, _ := Median(b.PairPeakMiB)
	switch {
	case b.OneWayPeakMiB <= 0:
		out = append(out, "the 1-way per-process peak was not measured, so the per-process rule cannot be judged")
	case median > PerProcessPeakRatio*b.OneWayPeakMiB:
		out = append(out, fmt.Sprintf("median per-process peak %.2f MiB is over %.1fx the 1-way figure, %.2f MiB (largest %.2f MiB)",
			median, PerProcessPeakRatio, b.OneWayPeakMiB, b.LargestMiB()))
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

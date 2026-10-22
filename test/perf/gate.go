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

// ColdAllocConsistent reports a problem with an entry's cold_alloc_bytes_max
// (P-12) against its own alloc_bytes_max row, or "" if there is none. A
// negative cold figure is always a problem; a measured one (nonzero) that
// reads below the entry's warm, steady-state figure is too, because a fresh
// process's one call pays everything the warmed benchmark measures plus the
// one-time cost the benchmark's own warm-up call exists to exclude -- cold
// can never be cheaper than warm for the same entry. An unmeasured cold
// figure (0, recap.* and probe.* today) is not compared with warm.
func ColdAllocConsistent(coldAllocBytesMax, allocBytesMax int64) string {
	if coldAllocBytesMax < 0 {
		return fmt.Sprintf("cold_alloc_bytes_max %d is negative", coldAllocBytesMax)
	}
	if coldAllocBytesMax > 0 && coldAllocBytesMax < allocBytesMax {
		return fmt.Sprintf("cold_alloc_bytes_max %d is below its warm alloc_bytes_max %d", coldAllocBytesMax, allocBytesMax)
	}
	return ""
}

// PerProcessColdAllocBytes is a hook-entry process's total allocation
// budget, the figure neither field alone is (P-12): e's cold_alloc_bytes_max
// covers the one hook/post call the process makes, and InitBudgetBytes
// (RES-2) covers everything the Go runtime and this binary's own package
// inits allocate before that call runs. Returns 0, meaning "not yet
// measured", for an entry whose cold figure is 0 -- never InitBudgetBytes
// alone, which would misread as a real per-process figure.
func PerProcessColdAllocBytes(e budget.Entry) int64 {
	if e.ColdAllocBytesMax <= 0 {
		return 0
	}
	return e.ColdAllocBytesMax + InitBudgetBytes
}

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

// referencePlatform is the platform every entry row and RES-28 case was
// measured or specified on. Its own Platform.Entries and CaseBudgets stay
// empty because its rows already are its data; gating it against its own
// row is self-consistent, never a cross-platform guess.
const referencePlatform = "darwin/arm64"

// entryBudget is the p50 TestRES28b_ gates e against on plat, once the
// reaper has already attributed a reading to e there (a caller reaches this
// only after that; whether the reaper's own floor rules a reading out at
// all is a fact about one measurement attempt, from peakrss.ErrBelowFloor,
// never a guess made in advance -- the fixed per-platform belowFloor list
// this replaced reported two entries as unmeasurable on a runner where they
// had in fact read above the floor, archive 85.4.4).
//
// Three outcomes: gated (plat has its own reading for e, an override, or
// plat is the reference platform, whose row is that reading); report-only
// (plat has never measured anything of its own, so gating against the
// reference row would assume plat reads no higher, an assumption Linux's
// first run, archive 85.4.3, disproved for several entries); or neither --
// plat has measured SOME of its own entries (len(overrides) > 0) but not
// this one, which is a measurement debt, not a new platform, and fails
// closed with why naming it.
func entryBudget(plat string, e budget.Entry, overrides map[string]budget.RSS) (budgetP50 float64, gated, reportOnly bool, why string) {
	if o, ok := overrides[e.ID]; ok {
		return o.P50, true, false, ""
	}
	if plat == referencePlatform {
		return e.MaxRSSMiBP50, true, false, ""
	}
	if len(overrides) == 0 {
		return e.MaxRSSMiBP50, false, true, fmt.Sprintf(
			"no %s reading recorded yet; gating against %s's row would assume %s reads no higher, which no measurement here has shown", plat, referencePlatform, plat)
	}
	return e.MaxRSSMiBP50, false, false, fmt.Sprintf(
		"%s has a measured baseline on %s, but no row of its own for %s: measure it and commit an override (or, if the reaper could never attribute a reading to it there, let that report-only instead)", plat, plat, e.ID)
}

// caseBudget is the same decision as entryBudget, for a RES-28 case: its
// BudgetMiB is RES-28's own line, which the case's basis is darwin/arm64's
// measurement against that line, not a cross-platform assumption on its
// own merit.
func caseBudget(plat string, c budget.RSSCase, overrides map[string]float64) (budgetMiB float64, gated, reportOnly bool, why string) {
	if v, ok := overrides[c.ID]; ok {
		return v, true, false, ""
	}
	if plat == referencePlatform {
		return c.BudgetMiB, true, false, ""
	}
	if len(overrides) == 0 {
		return c.BudgetMiB, false, true, fmt.Sprintf(
			"no %s reading recorded yet; gating against RES-28's line, measured against %s, would assume %s reads no higher, which no measurement here has shown", plat, referencePlatform, plat)
	}
	return c.BudgetMiB, false, false, fmt.Sprintf(
		"%s has a measured baseline on %s, but no row of its own for case %s: measure it and commit an override", plat, plat, c.ID)
}

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
	// FlushFailed counts spilled terminals carrying ReasonFlushFailed
	// (REC-34): a group-commit flush that failed under the burst's load,
	// not a lock that timed out. Spilled already fails the burst on either
	// cause; this names which one, so a real flush failure under 64-way
	// load does not read as an ordinary lock timeout.
	FlushFailed   int
	PairPeakMiB   []float64
	OneWayPeakMiB float64
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
// budget), a flush_failed marker (a group-commit flush that failed, not a
// lock that timed out), and every footprint problem footprintProblems names.
// A burst of no pairs measured nothing and fails.
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
	if b.FlushFailed > 0 {
		out = append(out, fmt.Sprintf("%d flush_failed markers, want none: a hook's group-commit flush failed under the burst's load", b.FlushFailed))
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

// LineHolds reports an RSS case whose recorded measurement is missing or
// sits over its line. A line holds on the reference machine with no
// tolerance (RES-28): the platform tolerance is for other machines and
// run-to-run noise, so a case that holds only through it is restated from
// the measurement, never padded.
func LineHolds(id string, measuredMiB, lineMiB float64) error {
	if measuredMiB <= 0 {
		return fmt.Errorf("rss case %s: no measured figure beside its line", id)
	}
	if measuredMiB > lineMiB {
		return fmt.Errorf("rss case %s: measured %.2f MiB is over its line %.2f MiB; restate the slope from the measurement", id, measuredMiB, lineMiB)
	}
	return nil
}

// landed names the PRs this tree carries: their rss cases are switched on,
// and no budget basis may still read "until" one of them. gates_from stays
// in budgets.json as the record of which PR switched each case on.
var landed = map[string]bool{"P-08": true, "P-10": true}

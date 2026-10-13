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
// up on a lock; and each pair's peak footprint in MiB, the larger of its hook's
// and its post's, which run one after the other.
type Burst struct {
	Ways                                int
	Declarations, Terminals, Executions int
	CallCoverage, PostCoverage          int
	Seqs                                []int64
	Spilled, LockTimeouts               int
	PairPeakMiB                         []float64
}

// Problems lists every way the burst missed RES-30 against a summed-footprint
// budget and its tolerance: a record of any kind that did not land in the
// ordered stream, a seq run that is not 1..n without a gap or a repeat, a
// spilled line or a lock timeout (each a hook that waited out its lock
// budget), a pair left unmeasured, and a summed footprint over budget. A burst
// of no pairs measured nothing and fails.
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

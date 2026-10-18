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

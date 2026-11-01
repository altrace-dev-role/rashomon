package perf

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

func TestRES28c_OverBudgetHoldsTheLine(t *testing.T) {
	for _, c := range []struct {
		measured, budget, tol float64
		want                  bool
	}{
		{6.5, 6.5, 0.10, false}, {7.15, 6.5, 0.10, false}, {7.16, 6.5, 0.10, true},
		{7.4, 6.5, 0.15, false}, {7.5, 6.5, 0.15, true},
		{0, 0, 0.10, true}, {1, -1, 0.10, true},
	} {
		if got := OverBudget(c.measured, c.budget, c.tol); got != c.want {
			t.Errorf("OverBudget(%v, %v, %v) = %v, want %v", c.measured, c.budget, c.tol, got, c.want)
		}
	}
}

func TestRES28d_ToleranceFollowsTheDate(t *testing.T) {
	before := time.Date(2026, 11, 5, 23, 0, 0, 0, time.UTC)
	on := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	if got := Tolerance(0.15, "2026-11-06", 0.10, before); got != 0.15 {
		t.Errorf("before the date: %v", got)
	}
	if got := Tolerance(0.15, "2026-11-06", 0.10, on); got != 0.10 {
		t.Errorf("on the date: %v", got)
	}
	if got := Tolerance(0.15, "", 0.10, before); got != 0.10 {
		t.Errorf("no date: %v, want the tighter", got)
	}
	if got := Tolerance(0.15, "someday", 0.10, before); got != 0.10 {
		t.Errorf("bad date: %v, want the tighter", got)
	}
}

func TestRES28g_ToleranceNoteNamesTheSwitch(t *testing.T) {
	after := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	// 7.3 over a 6.5 budget: inside 15%, over 10%.
	if got := ToleranceNote(7.3, 6.5, 0.15, "2026-11-06", 0.10, after); got != "tolerance tightened per budgets.json tolerance_until 2026-11-06 (15% to 10%)" {
		t.Errorf("after the date, a value the old tolerance admitted: %q", got)
	}
	if got := ToleranceNote(7.3, 6.5, 0.15, "2026-11-06", 0.10, before); got != "" {
		t.Errorf("before the date there is nothing to explain: %q", got)
	}
	if got := ToleranceNote(8.0, 6.5, 0.15, "2026-11-06", 0.10, after); got != "" {
		t.Errorf("a value over both tolerances is a plain regression: %q", got)
	}
	if got := ToleranceNote(7.3, 6.5, 0.10, "", 0.10, after); got != "" {
		t.Errorf("no schedule, no note: %q", got)
	}
}

func TestRES28e_MedianOfNine(t *testing.T) {
	m, ok := Median([]float64{9, 1, 8, 2, 7, 3, 6, 4, 5})
	if !ok || m != 5 {
		t.Errorf("median = %v, %v", m, ok)
	}
	if m, ok := Median([]float64{4, 1, 3, 2}); !ok || m != 2.5 {
		t.Errorf("even median = %v, %v", m, ok)
	}
	if _, ok := Median(nil); ok {
		t.Error("median of nothing reported ok")
	}
}

func TestRES2b_ParseInitTraceSumsAndFailsClosed(t *testing.T) {
	trace := "init internal/bytealg @0.008 ms, 0.001 ms clock, 0 bytes, 0 allocs\n" +
		"init runtime @0.017 ms, 0.016 ms clock, 0 bytes, 0 allocs\n" +
		"init errors @0.11 ms, 0.003 ms clock, 160 bytes, 2 allocs\n" +
		"something else\n" +
		"init regexp/syntax @0.5 ms, 0.1 ms clock, 61824 bytes, 53 allocs\n"
	got, err := ParseInitTrace(trace)
	if err != nil {
		t.Fatal(err)
	}
	if got.Packages != 4 || got.Bytes != 61984 || got.Allocs != 55 || got.GCLines != 0 {
		t.Errorf("parsed %+v", got)
	}
	withGC, err := ParseInitTrace(trace + "gc 1 @0.6 ms 0%: 0.010+0.2+0.004 ms clock, 0.10+0/0.3/0+0.04 ms cpu, 4->4->0 MB, 4 MB goal, 0 MB stacks, 0 MB globals, 8 P\n")
	if err != nil || withGC.GCLines != 1 {
		t.Errorf("gc line not counted: %+v, %v", withGC, err)
	}
	if _, err := ParseInitTrace("init runtime @0.017 ms\n"); err == nil {
		t.Error("a truncated init line parsed as a small init")
	}
	// Ten fields with "bytes," in place: the allocs word is missing, and the
	// parser must refuse the line rather than index past it.
	if _, err := ParseInitTrace("init errors @0.11 ms, 0.003 ms clock, 160 bytes, 2\n"); err == nil {
		t.Error("an init line without its allocs word parsed")
	}
}

// TestRES28l_SeedAndWorkDone: a row is measured on the store its budget
// names (a recap row on the session its own benchmark reads, probe end on a
// started session), and a run counts only when it left the evidence of its
// work: a record for hook and post, one clean coverage record of its own
// phase for probe, and the line naming the session for recap. A row the
// harness has no check for fails rather than passing unchecked.
func TestRES28l_SeedAndWorkDone(t *testing.T) {
	entry := func(bench string, command ...string) budget.Entry {
		return budget.Entry{ID: strings.Join(command, "."), Command: command, Benchmark: bench}
	}
	for _, c := range []struct {
		e    budget.Entry
		want Seed
		bad  bool
	}{
		{entry("BenchmarkRecap/100", "recap"), Seed{Calls: 100}, false},
		{entry("BenchmarkRecapPending/10", "recap", "pending"), Seed{Calls: 10}, false},
		{entry("", "probe", "end"), Seed{ProbeStart: true}, false},
		{entry("", "probe", "start"), Seed{}, false},
		{entry("BenchmarkRunHook/bash_short", "hook"), Seed{}, false},
		{entry("", "recap"), Seed{}, true},
		{entry("BenchmarkRecap/many", "recap"), Seed{}, true},
		{entry(""), Seed{}, true},
	} {
		got, err := SeedFor(c.e)
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("SeedFor(%v %q) = %+v, %v; want %+v, error %v", c.e.Command, c.e.Benchmark, got, err, c.want, c.bad)
		}
	}

	const session = "s-1"
	cov := func(phase, reason string) []byte {
		r := "null"
		if reason != "" {
			r = `"` + reason + `"`
		}
		return []byte(`{"type":"coverage","session_id":"` + session + `","phase":"` + phase + `","reason":` + r + `}`)
	}
	line := []byte(`{"systemMessage":"※ rashomon: 2 recorded failures\n  → rashomon report --session ` + session + `"}` + "\n")
	for _, c := range []struct {
		name    string
		command []string
		added   Added
		ok      bool
	}{
		{"hook recorded", []string{"hook"}, Added{Records: 1}, true},
		{"hook wrote nothing", []string{"hook"}, Added{}, false},
		{"post recorded", []string{"post"}, Added{Records: 2}, true},
		{"probe end clean", []string{"probe", "end"}, Added{Coverage: [][]byte{cov("end", "")}}, true},
		{"probe on the absent-entry path", []string{"probe", "end"}, Added{Coverage: [][]byte{cov("end", "hook_entry_absent")}}, false},
		{"probe wrote the other phase", []string{"probe", "start"}, Added{Coverage: [][]byte{cov("end", "")}}, false},
		{"probe wrote nothing", []string{"probe", "start"}, Added{}, false},
		{"probe wrote two", []string{"probe", "start"}, Added{Coverage: [][]byte{cov("start", ""), cov("start", "")}}, false},
		{"probe wrote garbage", []string{"probe", "start"}, Added{Coverage: [][]byte{[]byte("{")}}, false},
		{"recap spoke", []string{"recap"}, Added{Stdout: line}, true},
		{"recap silent", []string{"recap", "pending"}, Added{}, false},
		{"recap about another session", []string{"recap"}, Added{Stdout: []byte(`{"systemMessage":"rashomon report --session other"}`)}, false},
		{"no check for the entry", []string{"event"}, Added{Records: 1}, false},
		{"no command", nil, Added{Records: 1}, false},
	} {
		if err := WorkDone(c.command, session, c.added); (err == nil) != c.ok {
			t.Errorf("%s: WorkDone = %v, want ok %v", c.name, err, c.ok)
		}
	}
}

// TestRES28m_UngatedNamesEveryGateThatDidNotRun: a perf run that skipped a
// gate it owed, or ran none (a mistyped -run, a platform missing from the
// file), is a failure when the run is required to be whole; cases gating
// from a PR this tree does not carry are not owed.
func TestRES28m_UngatedNamesEveryGateThatDidNotRun(t *testing.T) {
	b := budget.Budgets{
		Entries:  []budget.Entry{{ID: "hook.a"}, {ID: "post.b"}},
		RSSCases: []budget.RSSCase{{ID: "hook.400B", GatesFrom: "P-08"}, {ID: "hook.1MiB", GatesFrom: "P-11"}},
	}
	all := map[string]bool{EntryGate("hook.a"): true, EntryGate("post.b"): true, CaseGate("hook.400B"): true, InitGate: true}
	if got := Ungated(b, all); len(got) != 0 {
		t.Errorf("every owed gate ran, Ungated = %q", got)
	}
	if got := Ungated(b, nil); strings.Join(got, "|") != "case hook.400B|entry hook.a|entry post.b|init" {
		t.Errorf("nothing ran: Ungated = %q", got)
	}
	delete(all, EntryGate("post.b"))
	if got := Ungated(b, all); strings.Join(got, "|") != "entry post.b" {
		t.Errorf("one entry skipped: Ungated = %q", got)
	}
}

// TestRES28q_EntryBudgetGatesOnlyWithAReadingOfItsOwn: the reference
// platform's row always gates (it is that platform's own measurement); any
// other platform gates only with its own override. With no override: a
// platform that has measured nothing of its own yet reports (the reader
// never gated anything there, so nothing to compare against); a platform
// that HAS measured some of its own entries, but not this one, fails closed
// instead of silently passing report-only (RES-28s) -- that is a measurement
// debt, not a brand-new platform. Whether the reaper could attribute a
// reading at all (the old fixed belowFloor list) is decided from the actual
// measurement attempt, upstream of this function, never guessed here.
func TestRES28q_EntryBudgetGatesOnlyWithAReadingOfItsOwn(t *testing.T) {
	e := budget.Entry{ID: "hook.bash_short", MaxRSSMiBP50: 5.88}
	if _, gated, reportOnly, why := entryBudget(referencePlatform, e, nil); !gated || reportOnly || why != "" {
		t.Errorf("reference platform, no override: gated=%v reportOnly=%v why=%q, want gated with no reason", gated, reportOnly, why)
	}
	overrides := map[string]budget.RSS{"hook.bash_short": {P50: 9}}
	if got, gated, reportOnly, why := entryBudget("linux/amd64", e, overrides); !gated || reportOnly || got != 9 || why != "" {
		t.Errorf("own override: got=%v gated=%v reportOnly=%v why=%q, want 9 gated with no reason", got, gated, reportOnly, why)
	}
	other := budget.Entry{ID: "hook.edit_50k", MaxRSSMiBP50: 6.45}
	if _, gated, reportOnly, why := entryBudget("linux/amd64", other, nil); gated || !reportOnly || !strings.Contains(why, "no linux/amd64 reading recorded") {
		t.Errorf("never measured on this platform, no override: gated=%v reportOnly=%v why=%q, want report-only naming no reading", gated, reportOnly, why)
	}
	// linux/amd64 HAS a baseline (overrides is non-empty, for hook.bash_short)
	// but not for other: a measurement debt, which must fail, not pass
	// report-only.
	if _, gated, reportOnly, why := entryBudget("linux/amd64", other, overrides); gated || reportOnly || !strings.Contains(why, "measured baseline") {
		t.Errorf("baseline exists but not for this entry: gated=%v reportOnly=%v why=%q, want a hard failure naming the baseline", gated, reportOnly, why)
	}
}

// TestRES28r_CaseBudgetGatesOnlyWithAReadingOfItsOwn: the same decision as
// entryBudget, for a RES-28 case.
func TestRES28r_CaseBudgetGatesOnlyWithAReadingOfItsOwn(t *testing.T) {
	c := budget.RSSCase{ID: "hook.400B", BudgetMiB: 6.5}
	if got, gated, reportOnly, why := caseBudget(referencePlatform, c, nil); !gated || reportOnly || got != 6.5 || why != "" {
		t.Errorf("reference platform, no override: got=%v gated=%v reportOnly=%v why=%q", got, gated, reportOnly, why)
	}
	overrides := map[string]float64{"hook.400B": 7.64}
	if got, gated, reportOnly, why := caseBudget("linux/amd64", c, overrides); !gated || reportOnly || got != 7.64 || why != "" {
		t.Errorf("own override: got=%v gated=%v reportOnly=%v why=%q, want 7.64 gated", got, gated, reportOnly, why)
	}
	if _, gated, reportOnly, why := caseBudget("linux/amd64", c, nil); gated || !reportOnly || !strings.Contains(why, "no linux/amd64 reading recorded") {
		t.Errorf("no baseline at all: gated=%v reportOnly=%v why=%q, want report-only", gated, reportOnly, why)
	}
	other := budget.RSSCase{ID: "hook.40KB", BudgetMiB: 6.58}
	if _, gated, reportOnly, why := caseBudget("linux/amd64", other, overrides); gated || reportOnly || !strings.Contains(why, "measured baseline") {
		t.Errorf("baseline exists but not for this case: gated=%v reportOnly=%v why=%q, want a hard failure naming the baseline", gated, reportOnly, why)
	}
}

// TestRES36n_UngatedOwesTheColdGateOnlyForMeasuredEntries: an entry with a
// measured cold_alloc_bytes_max (P-12) owes ColdGate(id) beside its
// EntryGate(id), so TestColdAllocBudget skipping it -- a -run pattern
// edited to stop selecting it, or a future entry that gains a cold figure
// with no gate run for it -- fails a required run; an entry whose cold
// figure is still 0 (not yet measured) owes no cold gate at all.
func TestRES36n_UngatedOwesTheColdGateOnlyForMeasuredEntries(t *testing.T) {
	b := budget.Budgets{
		Entries: []budget.Entry{{ID: "hook.a", ColdAllocBytesMax: 100}, {ID: "post.b"}},
	}
	if got := Ungated(b, nil); strings.Join(got, "|") != "cold hook.a|entry hook.a|entry post.b|init" {
		t.Errorf("nothing ran: Ungated = %q", got)
	}
	ran := map[string]bool{InitGate: true, EntryGate("hook.a"): true, EntryGate("post.b"): true}
	if got := Ungated(b, ran); strings.Join(got, "|") != "cold hook.a" {
		t.Errorf("the cold gate alone skipped: Ungated = %q", got)
	}
	ran[ColdGate("hook.a")] = true
	if got := Ungated(b, ran); len(got) != 0 {
		t.Errorf("every owed gate ran, Ungated = %q", got)
	}
}

// healthyBurst is a RES-30 run in which every pair landed every record.
func healthyBurst(ways int) Burst {
	b := Burst{Ways: ways, Declarations: ways, Terminals: ways, Executions: ways, CallCoverage: ways, PostCoverage: ways}
	for i := 1; i <= 3*ways; i++ {
		b.Seqs = append(b.Seqs, int64(i))
	}
	for i := 0; i < ways; i++ {
		b.PairPeakMiB = append(b.PairPeakMiB, 5)
	}
	b.OneWayPeakMiB = 5
	return b
}

// setPairs gives every pair the same peak.
func (b *Burst) setPairs(mib float64) {
	for i := range b.PairPeakMiB {
		b.PairPeakMiB[i] = mib
	}
}

// TestRES30_BurstVerdictCountsEveryLoss: the verdict the 64-way test reaches
// fails on every kind of loss and on the footprint sum, and passes only the
// burst in which nothing was lost (RES-30).
func TestRES30_BurstVerdictCountsEveryLoss(t *testing.T) {
	const ways, budget, tol = 4, 30.0, 0.10 // a healthy burst sums 20 MiB
	for _, c := range []struct {
		name  string
		spoil func(*Burst)
		fails bool
	}{
		{"healthy", func(*Burst) {}, false},
		{"a declaration lost", func(b *Burst) { b.Declarations-- }, true},
		{"a terminal lost", func(b *Burst) { b.Terminals-- }, true},
		{"an execution lost", func(b *Burst) { b.Executions-- }, true},
		{"a call coverage record lost", func(b *Burst) { b.CallCoverage-- }, true},
		{"a post coverage record lost", func(b *Burst) { b.PostCoverage-- }, true},
		{"a record spilled", func(b *Burst) { b.Spilled = 1 }, true},
		{"a lock timeout", func(b *Burst) { b.LockTimeouts = 1 }, true},
		{"a flush_failed marker", func(b *Burst) { b.FlushFailed = 1 }, true},
		{"a seq skipped", func(b *Burst) { b.Seqs[len(b.Seqs)-1]++ }, true},
		{"a seq repeated", func(b *Burst) { b.Seqs[1] = b.Seqs[0] }, true},
		{"a record without a seq", func(b *Burst) { b.Seqs = b.Seqs[:len(b.Seqs)-1] }, true},
		{"a pair unmeasured", func(b *Burst) { b.PairPeakMiB = b.PairPeakMiB[1:] }, true},
		{"a pair measured as nothing", func(b *Burst) { b.PairPeakMiB[0] = 0 }, true},
		// The sum cases raise the 1-way figure with the pair, so that only
		// the sum is judged.
		{"the sum at budget plus tolerance", func(b *Burst) {
			b.PairPeakMiB[0] += budget*(1+tol) - 20
			b.OneWayPeakMiB = b.PairPeakMiB[0]
		}, false},
		{"the sum over it", func(b *Burst) {
			b.PairPeakMiB[0] += budget*(1+tol) - 20 + 0.01
			b.OneWayPeakMiB = b.PairPeakMiB[0]
		}, true},
		{"no pairs at all", func(b *Burst) { *b = Burst{} }, true},
		// The per-process rule, restated from measurement (RES-30, ruling
		// 2026-10-13): the largest pair peak under the burst, which stands for
		// every process, is at most PerProcessPeakRatio (1.25) times the
		// median solo pair. One process in the tail over it fails the burst
		// however typical the rest; the median is not what is judged.
		{"every process at the ratio", func(b *Burst) { b.setPairs(PerProcessPeakRatio * b.OneWayPeakMiB) }, false},
		{"every process over it", func(b *Burst) { b.setPairs(PerProcessPeakRatio*b.OneWayPeakMiB + 0.01) }, true},
		{"one process at 1.21x, the largest the default measured", func(b *Burst) { b.PairPeakMiB[2] = 1.21 * b.OneWayPeakMiB }, false},
		{"one process at the ratio, the rest within", func(b *Burst) { b.PairPeakMiB[2] = PerProcessPeakRatio * b.OneWayPeakMiB }, false},
		{"one process over it, the median within", func(b *Burst) { b.PairPeakMiB[2] = PerProcessPeakRatio*b.OneWayPeakMiB + 0.01 }, true},
		{"the last process over it", func(b *Burst) { b.PairPeakMiB[len(b.PairPeakMiB)-1] = 2 * b.OneWayPeakMiB }, true},
		{"the 1-way figure unmeasured", func(b *Burst) { b.OneWayPeakMiB = 0 }, true},
	} {
		b := healthyBurst(ways)
		c.spoil(&b)
		if got := len(b.Problems(budget, tol, 0)) > 0; got != c.fails {
			t.Errorf("%s: fails = %v, want %v (%q)", c.name, got, c.fails, b.Problems(budget, tol, 0))
		}
	}
}

// TestRES30_RatioToleranceIsTheRunnersOnly: the per-process ratio takes the
// tolerance it is handed and no other. The harness hands it none on the
// reference machine, where the ratio was measured, and the platform's runner
// tolerance on a CI runner, where it has not been (macos-latest, ruling
// 2026-10-13); the summed footprint's tolerance never loosens it.
func TestRES30_RatioToleranceIsTheRunnersOnly(t *testing.T) {
	const ways, budget, tol = 4, 30.0, 0.10
	over := func(ratioTol float64) []string {
		b := healthyBurst(ways)
		b.PairPeakMiB[1] = PerProcessPeakRatio*b.OneWayPeakMiB + 0.01
		return b.Problems(budget, tol, ratioTol)
	}
	if got := over(0); len(got) == 0 {
		t.Error("a process over the ratio passed with no ratio tolerance: the sum's tolerance loosened it")
	}
	if got := over(0.15); len(got) != 0 {
		t.Errorf("a process 0.01 MiB over the ratio failed under a 15%% runner tolerance: %q", got)
	}
	b := healthyBurst(ways)
	b.PairPeakMiB[1] = PerProcessPeakRatio * 1.15 * b.OneWayPeakMiB * 1.001
	if got := b.Problems(budget, tol, 0.15); len(got) == 0 {
		t.Error("a process over the ratio plus the runner tolerance passed")
	}
}

// TestRES30_RatioToleranceOnlyOnARunner: the harness picks the ratio's
// tolerance by where it runs. On GitHub Actions, where macos-latest's ratio
// has not been measured, it is the platform's runner tolerance; anywhere else,
// the reference machine included, it is none, so the 1.25x rule holds as
// measured and a regression to 1.44x (1.25 x 1.15) cannot hide in it (RES-30,
// ruling 2026-10-13). Only "true", which is what Actions sets, is a runner.
func TestRES30_RatioToleranceOnlyOnARunner(t *testing.T) {
	const platform = 0.15
	env := func(v string, set bool) func(string) string {
		return func(k string) string {
			if k == "GITHUB_ACTIONS" && set {
				return v
			}
			return ""
		}
	}
	for _, c := range []struct {
		value string
		set   bool
		want  float64
	}{
		{"true", true, platform},
		{"", false, 0}, {"", true, 0}, {"false", true, 0}, {"1", true, 0}, {"TRUE", true, 0},
	} {
		if got := RatioTolerance(platform, env(c.value, c.set)); got != c.want {
			t.Errorf("RatioTolerance with GITHUB_ACTIONS=%q (set %v) = %v, want %v", c.value, c.set, got, c.want)
		}
	}
}

// TestRES30_BudgetsRowStatesTheGate: budgets.json's parallel row carries the
// per-process rule's basis, the measured figures it was restated from, and
// it names the same bound and sample count the gate uses, so the two cannot
// drift apart unseen (RES-30, ruling 2026-10-13).
func TestRES30_BudgetsRowStatesTheGate(t *testing.T) {
	table, err := budget.Table()
	if err != nil {
		t.Fatal(err)
	}
	row := table.Parallel
	for _, want := range []string{
		fmt.Sprintf("largest pair peak at most %.2fx", PerProcessPeakRatio),
		fmt.Sprintf("median of %d solo pairs", OneWaySamples),
		"GOMAXPROCS unchanged",
		"macos-latest unmeasured",
	} {
		if !strings.Contains(row.Basis, want) {
			t.Errorf("budgets.json parallel.basis does not say %q:\n%s", want, row.Basis)
		}
	}
}

// TestRES30_LargestIsTheLargestPairPeak: the largest process, which the
// per-process rule judges, is the largest pair peak, each the larger of a
// pair's hook and post, so the largest of every process (RES-30).
func TestRES30_LargestIsTheLargestPairPeak(t *testing.T) {
	b := Burst{PairPeakMiB: []float64{4.1, 4.3, 3.9}}
	if got := b.LargestMiB(); got != 4.3 {
		t.Errorf("LargestMiB() = %v, want 4.3", got)
	}
	if got := (Burst{}).LargestMiB(); got != 0 {
		t.Errorf("LargestMiB() of no pairs = %v, want 0", got)
	}
}

// TestRES30_ParseFootprintReadsTimeL: the darwin footprint comes from
// /usr/bin/time -l's report; a report without the line is not a zero.
func TestRES30_ParseFootprintReadsTimeL(t *testing.T) {
	report := "        0.01 real         0.00 user         0.00 sys\n" +
		"             6029312  maximum resident set size\n" +
		"             2622016  peak memory footprint\n"
	if got, ok := ParseFootprint(report); !ok || got != 2622016 {
		t.Errorf("ParseFootprint = %d, %v; want 2622016, true", got, ok)
	}
	if _, ok := ParseFootprint("             6029312  maximum resident set size\n"); ok {
		t.Error("a report with no footprint line parsed as one")
	}
}

// TestCOLDPLATFORM1_ColdBudgetIsThePlatformsOwnFigureElseTheTopLevelOne: a
// cold entry is gated against its platform's own cold_alloc_bytes_max
// (platforms.<plat>.cold_alloc_bytes_max, by entry id, as the RSS rows'
// entries are) where the platform has one, and against the entry's single
// top-level figure everywhere else -- another entry of the same platform
// with no figure of its own, and a platform with none at all.
func TestCOLDPLATFORM1_ColdBudgetIsThePlatformsOwnFigureElseTheTopLevelOne(t *testing.T) {
	e := budget.Entry{ID: "post.bash_short", ColdAllocBytesMax: 146672}
	own := map[string]int64{"post.bash_short": 162744}
	if got := ColdBudget(e, own); got != 162744 {
		t.Errorf("own figure: ColdBudget = %d, want 162744", got)
	}
	if got := ColdBudget(e, nil); got != 146672 {
		t.Errorf("platform with no figure: ColdBudget = %d, want the top-level 146672", got)
	}
	other := budget.Entry{ID: "hook.bash_short", ColdAllocBytesMax: 171136}
	if got := ColdBudget(other, own); got != 171136 {
		t.Errorf("entry the platform names no figure for: ColdBudget = %d, want the top-level 171136", got)
	}
	if got := ColdBudget(e, map[string]int64{"post.edit_314k": 1}); got != 146672 {
		t.Errorf("another entry's figure leaked: ColdBudget = %d, want 146672", got)
	}
}

// TestCOLDPLATFORM1_PlatformColdFiguresNameMeasuredEntriesAndHoldTheWarmRow:
// every per-platform cold figure in budgets.json names an entry that has a
// cold gate (a nonzero top-level figure, or the override is dead text), and
// is itself consistent with that entry's warm row (ColdAllocConsistent).
func TestCOLDPLATFORM1_PlatformColdFiguresNameMeasuredEntriesAndHoldTheWarmRow(t *testing.T) {
	b := loadBudgets(t)
	byID := map[string]budget.Entry{}
	for _, e := range b.Entries {
		byID[e.ID] = e
	}
	for plat, p := range b.Platforms {
		for id, v := range p.ColdAllocBytesMax {
			e, ok := byID[id]
			if !ok || e.ColdAllocBytesMax <= 0 {
				t.Errorf("%s: cold_alloc_bytes_max names %q, which is no entry with a cold gate", plat, id)
				continue
			}
			if v <= 0 {
				t.Errorf("%s: cold_alloc_bytes_max for %s is %d; an override is a measured figure, never 0", plat, id, v)
			}
			if why := ColdAllocConsistent(v, e.AllocBytesMax); why != "" {
				t.Errorf("%s: %s: %s", plat, id, why)
			}
		}
	}
}

package perf

import (
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
// from a later PR are not owed.
func TestRES28m_UngatedNamesEveryGateThatDidNotRun(t *testing.T) {
	b := budget.Budgets{
		Entries:  []budget.Entry{{ID: "hook.a"}, {ID: "post.b"}},
		RSSCases: []budget.RSSCase{{ID: "hook.400B", GatesFrom: "P-08"}, {ID: "hook.1MiB", GatesFrom: "P-10"}},
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

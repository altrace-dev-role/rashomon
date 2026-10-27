package verify

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"reflect"
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Seqs below are the builder's counter: with no terminal, call i has
// declaration 2i-1 and execution 2i (TestFixture_RunOfPairsEveryDeclarationWithItsExecution).

// statesAt is the gate state at the point at of the run calls build.
func statesAt(t *testing.T, at int64, calls ...call) []GateState {
	t.Helper()
	return statesOf(t, runOf(t, calls...), at)
}

// statesOf is classify then gateStates over r, with the v0 constants checked
// on every row any test of this file builds: Source learned, Trust builtin,
// Required, no partial runs, every RunRef's Scope unknown and every
// AgentCount's AgentType nil (TestVER10).
func statesOf(t *testing.T, r *store.Run, at int64) []GateState {
	t.Helper()
	runs, writes, unnamed, _ := classify(r)
	got := gateStates(runs, writes, unnamed, at)
	for _, g := range got {
		if g.Source != "learned" || g.Trust != "builtin" || !g.Required || g.PartialRunsSince != 0 {
			t.Fatalf("%s: source %q trust %q required %t partial %d, want learned, builtin, true, 0", g.Gate, g.Source, g.Trust, g.Required, g.PartialRunsSince)
		}
		for _, ref := range []*RunRef{g.LastRun, g.LastGreen} {
			if ref != nil && ref.Scope != "unknown" {
				t.Fatalf("%s: run %d scope %q, want unknown (nothing in the record states a scope yet)", g.Gate, ref.Seq, ref.Scope)
			}
		}
		for _, a := range g.WritersSinceGreen {
			if a.AgentType != nil {
				t.Fatalf("%s: writer %v agent type %q, want nil (classify's write carries none)", g.Gate, a.AgentID, *a.AgentType)
			}
		}
	}
	return got
}

// one is the only row of states, failing the test otherwise.
func one(t *testing.T, states []GateState) GateState {
	t.Helper()
	if len(states) != 1 {
		t.Fatalf("%d rows, want one: %+v", len(states), states)
	}
	return states[0]
}

func wantState(t *testing.T, g GateState, s State, r Reason) {
	t.Helper()
	if g.State != s || g.Reason != r {
		t.Fatalf("%s: %s %s, want %s %s", g.Gate, g.State, g.Reason, s, r)
	}
}

func withOutput(c call, kind string) call {
	c.gateOutput = &store.GateOutput{Kind: kind, Parser: "go"}
	return c
}

func maskedCall(seq int64) call {
	c := gateCall(seq, "go-test", store.ExecOK)
	c.class, c.masked = shape.VerbPackage, shape.MaskedTest // a stage after the runner keeps go's base class
	return c
}

func unnamedWrite(seq int64) call {
	return call{seq: seq, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true}
}

func seqOr0(r *RunRef) int64 {
	if r == nil {
		return 0
	}
	return r.Seq
}

func TestVER10_GreenAfterTheLastRelevantWrite(t *testing.T) {
	g := one(t, statesAt(t, 4, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)))
	wantState(t, g, Green, OK)
	if seqOr0(g.LastGreen) != 3 || seqOr0(g.LastRun) != 3 || g.WritesSinceGreen != 0 || g.LastWrite != nil {
		t.Fatalf("LastGreen %d LastRun %d WritesSinceGreen %d LastWrite %v, want 3, 3, 0, nil: the edit precedes the green", seqOr0(g.LastGreen), seqOr0(g.LastRun), g.WritesSinceGreen, g.LastWrite)
	}
	if g.WritersSinceGreen == nil {
		t.Fatal("WritersSinceGreen is nil with no writers, want made empty so it encodes as []")
	}
}

func TestVER11_SourceWriteAfterGreenIsStale(t *testing.T) {
	g := one(t, statesAt(t, 4, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go")))
	wantState(t, g, Stale, WritesAfterGreen)
	if g.WritesSinceGreen != 1 || g.LastWrite == nil || g.LastWrite.Seq != 3 || str(g.LastWrite.Path) != "pkg/a.go" {
		t.Fatalf("WritesSinceGreen %d LastWrite %+v, want 1 and pkg/a.go at 3", g.WritesSinceGreen, g.LastWrite)
	}
	wantState(t, one(t, statesAt(t, 4, gateCall(1, "go-test", store.ExecOK), edit(2, "README.md"))), Green, OK)
}

func TestVER11b_RelevanceFollowsPathClass(t *testing.T) {
	for _, tc := range []struct {
		path  string
		stale bool
	}{
		{"go.mod", true},
		{"pkg/a_test.go", true},
		{".github/ci.yaml", false},
		{"vendor/x.go", false},
		{"docs/a.md", false},
	} {
		g := one(t, statesAt(t, 4, gateCall(1, "go-test", store.ExecOK), edit(2, tc.path)))
		if got := g.State == Stale; got != tc.stale || Relevant(ClassOf(tc.path)) != tc.stale {
			t.Errorf("%s (class %s): %s, stale %t, want stale %t", tc.path, ClassOf(tc.path), g.State, got, tc.stale)
		}
	}
}

func TestVER12_RedByExitAndByOutput(t *testing.T) {
	wantState(t, one(t, statesAt(t, 2, gateCall(1, "go-test", store.ExecFailed))), Red, FailedExit)
	wantState(t, one(t, statesAt(t, 2, withOutput(gateCall(1, "go-test", store.ExecOK), "summary_fail"))), Red, FailedOutput)
}

func TestVER12b_EachRedTermAlone(t *testing.T) {
	nilCode := gateCall(1, "go-test", store.ExecFailed)
	nilCode.exit = 0 // a failure with no code: ExitCode nil
	interrupted := gateCall(1, "go-test", store.ExecInterrupted)
	interrupted.exit = 130
	for _, tc := range []struct {
		name string
		c    call
		want Reason
	}{
		{"failed with a nil code", nilCode, FailedExit},
		{"interrupted with 130", interrupted, FailedExit},
		{"summary_fail alone", withOutput(gateCall(1, "go-test", store.ExecOK), "summary_fail"), FailedOutput},
		{"fail_line alone", withOutput(gateCall(1, "go-test", store.ExecOK), "fail_line"), FailedOutput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantState(t, one(t, statesAt(t, 2, tc.c)), Red, tc.want)
		})
	}
	g := one(t, statesAt(t, 4, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/a.go")))
	wantState(t, g, Red, RedThenWrites)
	if g.WritesSinceGreen != 1 {
		t.Fatalf("WritesSinceGreen %d, want 1", g.WritesSinceGreen)
	}
	// A write before the red, after a green: counted, and not RedThenWrites.
	g = one(t, statesAt(t, 6, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), gateCall(3, "go-test", store.ExecFailed)))
	wantState(t, g, Red, FailedExit)
	if g.WritesSinceGreen != 1 {
		t.Fatalf("WritesSinceGreen %d, want 1: the write after the green", g.WritesSinceGreen)
	}
}

func TestVER13_MaskedIsInconclusiveNeverGreen(t *testing.T) {
	wantState(t, one(t, statesAt(t, 2, maskedCall(1))), MaskedInconclusive, MaskedNoSuccessLine)
	hiddenBuild := call{seq: 1, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-release", kind: shape.RunnerBuild, masked: shape.MaskedBuild, outcome: store.ExecOK, loc: "d:repo"}
	wantState(t, one(t, statesAt(t, 2, hiddenBuild)), MaskedInconclusive, MaskedNoSuccessLine)
	wantState(t, one(t, statesAt(t, 2, gateCall(1, "go-test", store.ExecOK))), Green, OK)
}

func TestVER13b_MaskedReasonsByOutput(t *testing.T) {
	for _, tc := range []struct {
		kind string // "" is no gate_output
		want Reason
	}{
		{"summary_pass", MaskedWithSuccessLine},
		{"", MaskedNoSuccessLine},
		{"no_summary", MaskedNoSuccessLine},
		{"empty_run", MaskedNoSuccessLine},
		{"unattributable", MaskedUnattributable},
	} {
		c := maskedCall(1)
		if tc.kind != "" {
			c = withOutput(c, tc.kind)
		}
		g := one(t, statesAt(t, 2, c))
		if g.State != MaskedInconclusive || g.Reason != tc.want {
			t.Errorf("output %q: %s %s, want %s %s", tc.kind, g.State, g.Reason, MaskedInconclusive, tc.want)
		}
	}
}

func TestVER14_BackgroundedAndNoFamily(t *testing.T) {
	bg := gateCall(1, "go-test", store.ExecOK)
	bg.bg = true
	wantState(t, one(t, statesAt(t, 2, bg)), Backgrounded, BackgroundLaunch)

	got := gateStates(nil, nil, nil, 10)
	b, err := json.Marshal(got)
	if err != nil || got == nil || string(b) != "[]" {
		t.Fatalf("no family: %v encodes as %s (err %v), want a made empty slice encoding as []", got, b, err)
	}
}

func TestVER15_ThePointCutsTheFuture(t *testing.T) {
	calls := []call{gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), gateCall(3, "go-test", store.ExecOK)}
	wantState(t, one(t, statesAt(t, 4, calls...)), Stale, WritesAfterGreen)
	wantState(t, one(t, statesAt(t, 6, calls...)), Green, OK)
}

func unread(seq int64) call {
	c := gateCall(seq, "go-test", store.ExecOK)
	reason := "budget"
	c.gateOutput = &store.GateOutput{Kind: "unread", Parser: "go", Reason: &reason}
	return c
}

func TestVER15b_UnreadOutputIsUnknown(t *testing.T) {
	wantState(t, one(t, statesAt(t, 2, unread(1))), Unknown, OutcomeUnknown)
}

// An unread run with a write after it is Unknown, never Stale: Stale would
// assert a pass nobody read (D28). The write is still counted.
func TestVER15c_UnreadThenWriteIsUnknownNotStale(t *testing.T) {
	g := one(t, statesAt(t, 4, unread(1), edit(2, "pkg/a.go")))
	wantState(t, g, Unknown, OutcomeUnknown)
	if g.WritesSinceGreen != 1 || g.LastGreen != nil {
		t.Fatalf("WritesSinceGreen %d LastGreen %v, want 1 and nil: an unread run is never the reference green", g.WritesSinceGreen, g.LastGreen)
	}
}

func TestVER15d_InterruptedWithoutACodeIsUnknown(t *testing.T) {
	stopped := gateCall(3, "go-test", store.ExecInterrupted) // exit 0: ExitCode nil
	g := one(t, statesAt(t, 6, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), stopped))
	wantState(t, g, Unknown, OutcomeUnknown)
	if seqOr0(g.LastGreen) != 1 || g.WritesSinceGreen != 1 {
		t.Fatalf("LastGreen %d WritesSinceGreen %d, want 1 and 1: the interrupted run is not the reference green", seqOr0(g.LastGreen), g.WritesSinceGreen)
	}
	coded := gateCall(1, "go-test", store.ExecInterrupted)
	coded.exit = 130
	wantState(t, one(t, statesAt(t, 2, coded)), Red, FailedExit)
}

// The table's order is pinned here: a reviewer who wants Red for a masked run
// with fail output changes the spec, not this test.
func TestVER16_MaskedBeforeFailed(t *testing.T) {
	wantState(t, one(t, statesAt(t, 2, withOutput(maskedCall(1), "summary_fail"))), MaskedInconclusive, MaskedUnattributable)
}

func TestVER17_WritersAreCapped(t *testing.T) {
	for _, tc := range []struct {
		agents, omitted int
	}{{9, 1}, {8, 0}} {
		calls := []call{gateCall(1, "go-test", store.ExecOK)}
		for i := 1; i <= tc.agents; i++ {
			c := edit(int64(i+1), "pkg/f"+strconv.Itoa(i)+".go")
			c.agent = "a" + strconv.Itoa(i)
			calls = append(calls, c)
		}
		n := int64(len(calls))
		calls = append(calls, unnamedWrite(n+1))
		at := 2 * (n + 1)
		g := one(t, statesAt(t, at, calls...))
		kept := min(tc.agents, MaxWriters)
		if len(g.WritersSinceGreen) != kept || g.WritersOmitted != tc.omitted || g.UnknownTarget != 1 || g.WritesSinceGreen != tc.agents || g.LastWrite == nil || g.LastWrite.Seq != 2*n-1 {
			t.Fatalf("%d agents: writers %d omitted %d unknown %d writes %d last write %+v, want %d, %d, 1, %d, %d", tc.agents,
				len(g.WritersSinceGreen), g.WritersOmitted, g.UnknownTarget, g.WritesSinceGreen, g.LastWrite, kept, tc.omitted, tc.agents, 2*n-1)
		}
		for i, a := range g.WritersSinceGreen {
			if str(a.AgentID) != "a"+strconv.Itoa(i+1) || a.Writes != 1 {
				t.Fatalf("writer %d is %s with %d writes, want a%d with 1: writers in the order of their first write", i, str(a.AgentID), a.Writes, i+1)
			}
		}
	}
}

// encodedUnderShuffles is gateStates' JSON for one order of its inputs, then
// checked byte-identical over 100 shuffles of runs, writes and unnamed.
func encodedUnderShuffles(t *testing.T, r *store.Run, at int64) []byte {
	t.Helper()
	runs, writes, unnamed, _ := classify(r)
	want, err := json.Marshal(gateStates(runs, writes, unnamed, at))
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(runs), func(i, j int) { runs[i], runs[j] = runs[j], runs[i] })
		rng.Shuffle(len(writes), func(i, j int) { writes[i], writes[j] = writes[j], writes[i] })
		rng.Shuffle(len(unnamed), func(i, j int) { unnamed[i], unnamed[j] = unnamed[j], unnamed[i] })
		got, err := json.Marshal(gateStates(runs, writes, unnamed, at))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("shuffle %d: %s, want %s", i, got, want)
		}
	}
	return want
}

func TestVER18_SortedAndOrderIndependent(t *testing.T) {
	build := call{seq: 3, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-release", kind: shape.RunnerBuild, masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo"}
	sub := edit(4, "pkg/b.go")
	sub.agent = "a1"
	// The last write names two paths in one call: LastWrite is the greater.
	r := runOf(t, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), build, sub, unnamedWrite(5), gateCall(6, "go-test", store.ExecFailed), bashWrite(7, "pkg/d.go", "pkg/c.go"))
	encodedUnderShuffles(t, r, 14)
	got := statesOf(t, r, 14)
	if len(got) != 2 || got[0].Gate != "learned:build:r:make-relea@d:repo" || got[1].Gate != "learned:test:r:go-test@d:repo" {
		t.Fatalf("rows %+v, want the build gate then the test gate", got)
	}
	if w := got[1].LastWrite; w == nil || w.Seq != 13 || str(w.Path) != "pkg/d.go" {
		t.Fatalf("LastWrite %+v, want pkg/d.go at 13", w)
	}
}

func TestVER18b_OneRunnerInTwoLocationsIsTwoGates(t *testing.T) {
	other := gateCall(2, "go-test", store.ExecOK)
	other.loc = "d:other"
	got := statesAt(t, 4, gateCall(1, "go-test", store.ExecOK), other)
	if len(got) != 2 || got[0].Gate == got[1].Gate || got[0].Family != "r:go-test@d:other" || got[1].Family != "r:go-test@d:repo" {
		t.Fatalf("rows %+v, want two families of one runner with two Gate names", got)
	}
	// Two runners whose digests share their first 12 bytes at one location
	// share a Gate name; Family orders them, whatever the input order.
	r := runOf(t, gateCall(1, "go-test-unit-b", store.ExecOK), gateCall(2, "go-test-unit-a", store.ExecFailed))
	b := encodedUnderShuffles(t, r, 4)
	var rows []GateState
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Gate != rows[1].Gate || rows[0].Family != "r:go-test-unit-a@d:repo" {
		t.Fatalf("rows %+v, want one Gate name ordered by Family", rows)
	}
}

func TestVER19_SpilledExecutionStillCredits(t *testing.T) {
	spilled := gateCall(2, "go-test", store.ExecOK)
	spilled.spilled, spilled.term = true, store.OutcomeOK
	g := one(t, statesAt(t, 3, edit(1, "pkg/a.go"), spilled))
	wantState(t, g, Green, OK)
	if seqOr0(g.LastGreen) != 3 {
		t.Fatalf("LastGreen %d, want 3, the declaration's Seq", seqOr0(g.LastGreen))
	}
}

// A lost declaration pairs with nothing, so it is no run here. 1.4 turns it
// into Recording Incomplete.
func TestVER20_LostDeclarationIsNoRow(t *testing.T) {
	lost := gateCall(1, "go-test", store.ExecOK)
	lost.lostDeclaration, lost.term = true, store.OutcomeError
	vet := gateCall(2, "go-vet", store.ExecOK)
	g := one(t, statesAt(t, 10, lost, vet))
	wantState(t, g, Green, OK)
	if g.Family != "r:go-vet@d:repo" {
		t.Fatalf("family %q, want the sibling's", g.Family)
	}
}

func TestVER21_NoFamilyRunsNeitherStaleNorCredit(t *testing.T) {
	hidden := call{seq: 3, tool: "Bash", prog: "go", class: shape.VerbPackage, masked: shape.MaskedTest, outcome: store.ExecOK, loc: "d:repo"} // two hidden runners: no digest
	nowhere := gateCall(4, "go-test", store.ExecOK)
	nowhere.loc = ""
	g := one(t, statesAt(t, 8, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), hidden, nowhere))
	wantState(t, g, Stale, WritesAfterGreen)
	if seqOr0(g.LastRun) != 1 {
		t.Fatalf("LastRun %d, want 1: a run with no family credits none", seqOr0(g.LastRun))
	}
	wantState(t, one(t, statesAt(t, 6, gateCall(1, "go-test", store.ExecOK), hidden, nowhere)), Green, OK)
}

func TestVER22_GateOutputFieldCount(t *testing.T) {
	if n := reflect.TypeFor[store.GateOutput]().NumField(); n != 3 {
		t.Fatalf("store.GateOutput has %d fields, want 3: runOfSession's copy of a call's gateOutput and sameGateOutput compare three; update both", n)
	}
}

func TestVER23_ReferenceRunWithoutAGreen(t *testing.T) {
	g := one(t, statesAt(t, 6, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/a.go"), edit(3, "pkg/b.go")))
	if g.WritesSinceGreen != 2 || g.LastGreen != nil {
		t.Fatalf("WritesSinceGreen %d LastGreen %v, want 2 and nil", g.WritesSinceGreen, g.LastGreen)
	}
	// With a green before the red, the green is the reference.
	g = one(t, statesAt(t, 8, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), gateCall(3, "go-test", store.ExecFailed), edit(4, "pkg/b.go")))
	if g.WritesSinceGreen != 2 || seqOr0(g.LastGreen) != 1 {
		t.Fatalf("WritesSinceGreen %d LastGreen %d, want 2 and 1", g.WritesSinceGreen, seqOr0(g.LastGreen))
	}
}

// A run is at the point when its outcome is (D31).
func TestVER24_ARunExistsAtItsExecution(t *testing.T) {
	if got := statesAt(t, 1, gateCall(1, "go-test", store.ExecOK)); len(got) != 0 {
		t.Fatalf("at the declaration: %+v, want no row: the run is in flight", got)
	}
	wantState(t, one(t, statesAt(t, 2, gateCall(1, "go-test", store.ExecOK))), Green, OK)
	spilled := gateCall(1, "go-test", store.ExecOK)
	spilled.spilled, spilled.term = true, store.OutcomeOK
	wantState(t, one(t, statesAt(t, 1, spilled)), Green, OK)
}

// A write declared while a gate run is in flight counts against it (D31):
// decl test 5, decl edit 6, exec edit 7, exec test 8, set by hand since the
// builder does not interleave calls.
func TestVER24b_AWriteDuringTheRunStales(t *testing.T) {
	for _, tc := range []struct {
		editDecl int64
		want     State
		writes   int
	}{{6, Stale, 1}, {4, Green, 0}} {
		r := runOf(t, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"))
		r.Declarations[0].Seq, r.Executions[0].Seq = 5, ptr(int64(8))
		r.Declarations[1].Seq, r.Executions[1].Seq = tc.editDecl, ptr(int64(7))
		g := one(t, statesOf(t, r, 8))
		if g.State != tc.want || g.WritesSinceGreen != tc.writes {
			t.Fatalf("edit declared at %d: %s with %d writes, want %s with %d", tc.editDecl, g.State, g.WritesSinceGreen, tc.want, tc.writes)
		}
	}
}

// An unnamed write is counted and changes no state (D30).
func TestVER25_UnnamedWriteAfterGreenIsCounted(t *testing.T) {
	g := one(t, statesAt(t, 4, gateCall(1, "go-test", store.ExecOK), unnamedWrite(2)))
	wantState(t, g, Green, OK)
	if g.UnknownTarget != 1 || g.WritesSinceGreen != 0 || g.LastWrite != nil {
		t.Fatalf("UnknownTarget %d WritesSinceGreen %d LastWrite %v, want 1, 0, nil", g.UnknownTarget, g.WritesSinceGreen, g.LastWrite)
	}
	wantState(t, one(t, statesAt(t, 6, gateCall(1, "go-test", store.ExecOK), unnamedWrite(2), edit(3, "pkg/a.go"))), Stale, WritesAfterGreen)
}

// BenchmarkGateStates is the gate state at one point of a run with two
// families and fifty writes: go test and go vet run green every ten edits,
// the edits split between the main agent and four subagents, and the point
// is the run's end. An evaluation computes it once per point, so this is
// that cost.
func BenchmarkGateStates(b *testing.B) {
	// A top-level benchmark, not a b.Run: the name with a sub-benchmark's
	// suffix reads as a secret to internal/secretshape's entropy test, which
	// TestH113 runs over test/perf/budgets.json.
	var calls []call
	seq := int64(0)
	next := func() int64 { seq++; return seq }
	for i := range 50 {
		if i%10 == 0 {
			calls = append(calls, gateCall(next(), "go-test", store.ExecOK), gateCall(next(), "go-vet", store.ExecOK))
		}
		c := edit(next(), "pkg/f"+strconv.Itoa(i)+".go")
		if i%2 == 1 {
			c.agent = "a" + strconv.Itoa(i%5)
		}
		calls = append(calls, c)
	}
	runs, writes, unnamed, _ := classify(runOf(b, calls...))
	at := 2 * seq
	if got := gateStates(runs, writes, unnamed, at); len(got) != 2 || got[0].WritesSinceGreen != 10 {
		b.Fatalf("the fixture gives %+v, want two families with ten writes since green", got)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		gateStates(runs, writes, unnamed, at)
	}
}

package verify

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"reflect"
	"slices"
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
	got := gateStates(runs, writes, unnamed, at, nil)
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

func TestVER11c_RelevanceFollowsPathClass(t *testing.T) {
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

// TestVER13c_EmptyRunIsAbsentNeverGreen: a run whose output says no test ran
// is Absent with reason empty_run (2.2.4 step 5), after the backgrounded and
// masked rows and before every other (D46), and it is no green: LastGreen
// stays the last real one and writes count from it (D47). An ok run with
// no_summary stays green: the spec's "visible ok ... else green"; only
// empty_run moves. Seqs: call i declares at 2i-1.
func TestVER13c_EmptyRunIsAbsentNeverGreen(t *testing.T) {
	empty := func(seq int64, outcome string) call {
		return withOutput(gateCall(seq, "go-test", outcome), outEmptyRun)
	}
	pytest5 := empty(1, store.ExecFailed)
	pytest5.exit = 5
	for _, c := range []struct {
		name      string
		at        int64
		calls     []call
		state     State
		reason    Reason
		lastRun   int64
		lastGreen int64 // 0 is none
		writes    int
		lastWrite int64 // 0 is none
	}{
		{"(a) an ok empty run", 2, []call{empty(1, store.ExecOK)}, Absent, EmptyRun, 1, 0, 0, 0},
		{"(b) green, a write, an empty run", 6, []call{gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), empty(3, store.ExecOK)},
			Absent, EmptyRun, 5, 1, 1, 3},
		{"(c) an empty run, then a write", 4, []call{empty(1, store.ExecOK), edit(2, "pkg/a.go")}, Absent, EmptyRun, 1, 0, 1, 3},
		{"(d) a masked empty run", 2, []call{withOutput(maskedCall(1), outEmptyRun)}, MaskedInconclusive, MaskedNoSuccessLine, 1, 0, 0, 0},
		{"(e) an ok run with no output", 2, []call{gateCall(1, "go-test", store.ExecOK)}, Green, OK, 1, 1, 0, 0},
		{"(f) an ok run with no summary", 2, []call{withOutput(gateCall(1, "go-test", store.ExecOK), outNoSummary)}, Green, OK, 1, 1, 0, 0},
		{"(g) pytest's exit 5", 2, []call{pytest5}, Absent, EmptyRun, 1, 0, 0, 0},
		{"(h) a failed run with a failing summary", 2, []call{withOutput(gateCall(1, "go-test", store.ExecFailed), outSummaryFail)},
			Red, FailedExit, 1, 0, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := one(t, statesAt(t, c.at, c.calls...))
			wantState(t, g, c.state, c.reason)
			if g.LastRun == nil || g.LastRun.Seq != c.lastRun {
				t.Errorf("LastRun %+v, want the run at %d", g.LastRun, c.lastRun)
			}
			if got := refSeq(g.LastGreen); got != c.lastGreen {
				t.Errorf("LastGreen at %d, want %d (0 is none)", got, c.lastGreen)
			}
			if g.WritesSinceGreen != c.writes {
				t.Errorf("WritesSinceGreen %d, want %d", g.WritesSinceGreen, c.writes)
			}
			if got := int64(0); g.LastWrite != nil && g.LastWrite.Seq != c.lastWrite || g.LastWrite == nil && c.lastWrite != got {
				t.Errorf("LastWrite %+v, want the write at %d (0 is none)", g.LastWrite, c.lastWrite)
			}
		})
	}
	if snap := evalAt(runOf(t, empty(1, store.ExecOK)), 2); snap.RecordGreen || snap.Gates[0].State != Absent {
		t.Errorf("an empty run: RecordGreen %t, gate %s, want false and absent", snap.RecordGreen, snap.Gates[0].State)
	}
}

// refSeq is r's Seq, 0 where r is nil.
func refSeq(r *RunRef) int64 {
	if r == nil {
		return 0
	}
	return r.Seq
}

// TestVER13d_EmptyRunReachesTheLedger: through EvaluateWithObligations, the
// Absent rows card 1.9 pinned on obligations() directly (TestPG5b): an empty
// run with no write after it owes nothing, no row of any kind; with one,
// gate_after_write is open on the write. Absent now has a run, so
// exit_unmasked is met on it where a row is owed: the table reads an empty
// run only where no pipe hid its status (D47). The watched row's
// red is the table's Red and its green the table's green (D47): an empty run
// after a red leaves the row open (TestPG7b's red with no green), and
// pytest's exit 5 alone opens none.
func TestVER13d_EmptyRunReachesTheLedger(t *testing.T) {
	empty := func(seq int64, outcome string) call {
		return withOutput(gateCall(seq, "go-test", outcome), outEmptyRun)
	}
	if obs := obAt(runOf(t, empty(1, store.ExecOK)), 2); len(obs) != 0 {
		t.Errorf("an empty run with no write owes %+v, want nothing", obs)
	}
	r := runOf(t, empty(1, store.ExecOK), edit(2, "pkg/a.go"))
	obs := obAt(r, 4)
	wantRow(t, rowOf(t, obs, GateAfterWrite), ObligationOpen, declSeq(r, 1))
	wantRow(t, rowOf(t, obs, ExitUnmasked), ObligationMet, declSeq(r, 0))

	r = runOf(t, gateCall(1, "go-test", store.ExecFailed), empty(2, store.ExecOK))
	wantRow(t, rowOf(t, obAt(r, 4), WatchedRedThenGreen), ObligationOpen, declSeq(r, 0))

	pytest5 := empty(1, store.ExecFailed)
	pytest5.exit = 5
	noRow(t, obAt(runOf(t, pytest5), 2), WatchedRedThenGreen)
}

// TestVER13e_StateOrderAndVocabularyUnchanged: the empty-run row adds no
// state and no reason; Order and empty_run's state stand as they were.
func TestVER13e_StateOrderAndVocabularyUnchanged(t *testing.T) {
	if want := []State{Red, MaskedInconclusive, Backgrounded, Stale, Absent, Unknown, Green}; !slices.Equal(Order, want) || reasonState()[EmptyRun] != Absent {
		t.Fatalf("Order %v and empty_run's state %s, want %v and absent", Order, reasonState()[EmptyRun], want)
	}
}

// TestVER13f_GreenIsTheNegationOfTheRowsAboveStale is green()'s doc as a
// test, over every run the rows above Stale read: backgrounded or not,
// masked by test, build or not at all, each outcome (an interrupt with a code
// and one without), no output or each of the seven kinds, and landed in
// order or spilled. green(r) holds exactly when no row before Stale matches a
// family whose last run is r, so a row added above Stale without its term in
// green() fails here (D47). The enumeration varies f.last only; counted stays
// empty, so a row above Stale that reads f.counted or any other fam field
// needs its own test.
func TestVER13f_GreenIsTheNegationOfTheRowsAboveStale(t *testing.T) {
	stale := slices.IndexFunc(gateTable, func(r gateRow) bool { return r.state == Stale })
	if stale < 1 {
		t.Fatalf("the Stale row is at %d, want a row after the first", stale)
	}
	one := 1
	type outcome struct {
		name string
		exit *int
	}
	n := 0
	for _, bg := range []bool{false, true} {
		for _, m := range []*string{nil, ptr(shape.MaskedTest), ptr(shape.MaskedBuild)} {
			for _, o := range []outcome{{store.ExecOK, nil}, {store.ExecFailed, nil}, {store.ExecInterrupted, &one}, {store.ExecInterrupted, nil}, {"", nil}} {
				for _, k := range []string{"", outSummaryPass, outSummaryFail, outFailLine, outEmptyRun, outNoSummary, "unattributable", outUnread} {
					for _, execSeq := range []*int64{ptr(int64(2)), nil} {
						r := gateRun{Seq: 1, ExecSeq: execSeq, Backgrounded: bg, Masked: m, Outcome: o.name, ExitCode: o.exit}
						if k != "" {
							r.Output = &store.GateOutput{Kind: k}
						}
						f := fam{last: &r}
						above := slices.ContainsFunc(gateTable[:stale], func(row gateRow) bool { return row.when(&f) })
						if green(&r) == above {
							t.Errorf("bg %t masked %v outcome %q exit %v output %q landed %t: green %t, a row above Stale matches %t",
								bg, m, o.name, o.exit, k, execSeq != nil, green(&r), above)
						}
						n++
					}
				}
			}
		}
	}
	if n != 480 {
		t.Fatalf("%d runs, want 480", n)
	}
}

func TestVER14_BackgroundedAndNoFamily(t *testing.T) {
	bg := gateCall(1, "go-test", store.ExecOK)
	bg.bg = true
	wantState(t, one(t, statesAt(t, 2, bg)), Backgrounded, BackgroundLaunch)

	got := gateStates(nil, nil, nil, 10, nil)
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

// encodedUnderShuffles is gateStates' JSON for one order of its inputs, with
// dgs' rows joined (withDeclared) where any is given, then checked
// byte-identical over 100 shuffles of runs, writes, unnamed and the declared
// gates' candidate runs.
func encodedUnderShuffles(t *testing.T, r *store.Run, at int64, dgs ...declaredGate) []byte {
	t.Helper()
	runs, writes, unnamed, _ := classify(r)
	cands := lineRuns(r, pickExecutions(r))
	rows := func() []GateState {
		g := gateStates(runs, writes, unnamed, at, nil)
		if len(dgs) > 0 {
			g = withDeclared(g, dgs, cands, runs, writes, unnamed, at, nil)
		}
		return g
	}
	want, err := json.Marshal(rows())
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(runs), func(i, j int) { runs[i], runs[j] = runs[j], runs[i] })
		rng.Shuffle(len(writes), func(i, j int) { writes[i], writes[j] = writes[j], writes[i] })
		rng.Shuffle(len(unnamed), func(i, j int) { unnamed[i], unnamed[j] = unnamed[j], unnamed[i] })
		rng.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
		got, err := json.Marshal(rows())
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
	// A declared gate over the test runner absorbs its family, and one no run
	// matched reads Absent with the writes counted: the same bytes whatever
	// the order (PRD PG3).
	unit := declaredGate{id: "spec:unit", kind: kindTest, source: sourceDeclared, trust: "trusted", required: true, runners: map[string]bool{"r:go-test": true}, location: "d:repo"}
	lint := declaredGate{id: "spec:lint", kind: "lint", source: sourceDeclared, trust: "trusted", required: true, lines: map[string]bool{"no-such-line": true}, location: "d:repo"}
	var rows []GateState
	if err := json.Unmarshal(encodedUnderShuffles(t, r, 14, unit, lint), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Gate != "learned:build:r:make-relea@d:repo" || rows[1].Gate != "spec:lint" || rows[1].State != Absent ||
		rows[1].WritesSinceGreen != 4 || rows[2].Gate != "spec:unit" || rows[2].Family != familyDeclared || rows[2].State != Red {
		t.Fatalf("rows %+v, want the build gate, spec:lint absent with four writes, spec:unit red", rows)
	}
}

func TestVER18c_OneRunnerInTwoLocationsIsTwoGates(t *testing.T) {
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
	// A declared gate bound to one of the two locations takes that family's
	// run alone; the other family keeps its learned row.
	r = runOf(t, gateCall(1, "go-test", store.ExecOK), other)
	web := declaredGate{id: "spec:web", kind: kindTest, source: sourceDeclared, trust: "trusted", required: true, runners: map[string]bool{"r:go-test": true}, location: "d:other"}
	rows = nil
	if err := json.Unmarshal(encodedUnderShuffles(t, r, 4, web), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Family != "r:go-test@d:repo" || rows[1].Gate != "spec:web" || seqOr0(rows[1].LastRun) != 3 {
		t.Fatalf("rows %+v, want the root's learned row, then spec:web from the other location's run at 3", rows)
	}
}

// A spilled run is cited by its declaration's Seq, and credits nothing: when
// its outcome landed is not known (D33, which replaced D31's fallback here).
func TestVER19_SpilledRunIsCitedAtItsDeclaration(t *testing.T) {
	g := one(t, statesAt(t, 3, edit(1, "pkg/a.go"), spilledGate(2, store.ExecOK)))
	wantState(t, g, Unknown, OrderUnknown)
	if seqOr0(g.LastRun) != 3 || g.LastGreen != nil {
		t.Fatalf("LastRun %d LastGreen %v, want 3, the declaration's Seq, and nil", seqOr0(g.LastRun), g.LastGreen)
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
	// A spilled run exists from its declaration, the earliest its outcome can
	// have landed, and is Unknown there (D33).
	wantState(t, one(t, statesAt(t, 1, spilledGate(1, store.ExecOK))), Unknown, OrderUnknown)
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

// spilledGate is a gate run whose execution and terminal went to the spill
// file: its outcome is recorded, its order is not.
func spilledGate(seq int64, outcome string) call {
	c := gateCall(seq, "go-test", outcome)
	c.spilled, c.term = true, store.OutcomeOK
	return c
}

// A spilled last run is Unknown, OrderUnknown, from its declaration on, and
// never the reference green (D33): its outcome is known, when it landed is not.
func TestVER26_SpilledLastRunIsOrderUnknown(t *testing.T) {
	calls := []call{gateCall(1, "go-test", store.ExecOK), spilledGate(2, store.ExecOK)}
	for _, at := range []int64{3, 10} {
		g := one(t, statesAt(t, at, calls...))
		wantState(t, g, Unknown, OrderUnknown)
		if seqOr0(g.LastRun) != 3 || seqOr0(g.LastGreen) != 1 {
			t.Fatalf("at %d: LastRun %d LastGreen %d, want 3 and 1", at, seqOr0(g.LastRun), seqOr0(g.LastGreen))
		}
	}
	// Before its declaration the run does not exist, and the green stands.
	wantState(t, one(t, statesAt(t, 2, calls...)), Green, OK)
	// The green before it stays the reference: an edit between them counts.
	g := one(t, statesAt(t, 10, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), spilledGate(3, store.ExecOK)))
	wantState(t, g, Unknown, OrderUnknown)
	if g.WritesSinceGreen != 1 || seqOr0(g.LastGreen) != 1 {
		t.Fatalf("WritesSinceGreen %d LastGreen %d, want 1 and 1: the spilled run is not the reference", g.WritesSinceGreen, seqOr0(g.LastGreen))
	}
	// A write after it is counted and the state stays Unknown.
	g = one(t, statesAt(t, 10, gateCall(1, "go-test", store.ExecOK), spilledGate(2, store.ExecOK), edit(3, "pkg/a.go")))
	wantState(t, g, Unknown, OrderUnknown)
	if g.WritesSinceGreen != 1 {
		t.Fatalf("WritesSinceGreen %d, want 1", g.WritesSinceGreen)
	}
	// A spilled red is Red: an unknown order never hides a red (state.go, Order).
	wantState(t, one(t, statesAt(t, 10, gateCall(1, "go-test", store.ExecOK), spilledGate(2, store.ExecFailed))), Red, FailedExit)
}

// A spilled run that is not the last changes nothing.
func TestVER26b_SpilledEarlierRunLeavesTheLastRunsState(t *testing.T) {
	g := one(t, statesAt(t, 3, spilledGate(1, store.ExecOK), gateCall(2, "go-test", store.ExecOK)))
	wantState(t, g, Green, OK)
	if seqOr0(g.LastRun) != 2 || seqOr0(g.LastGreen) != 2 {
		t.Fatalf("LastRun %d LastGreen %d, want 2 and 2", seqOr0(g.LastRun), seqOr0(g.LastGreen))
	}
}

// The Gate's kind is the family's recorded kind wherever in the record a run
// records one, so the name is the same at every point (D33).
func TestVER27_GateKindIsTheFamilysRecordedKind(t *testing.T) {
	// Both class package, so no run is evidence for the inference: the
	// schema 5 run infers custom (classify, inferKinds).
	old := call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbPackage, runner: "go-test", masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo", schema: 5}
	recorded := call{seq: 2, tool: "Bash", prog: "go", class: shape.VerbPackage, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo"}
	r := runOf(t, old, recorded)
	for _, at := range []int64{2, 4} {
		if g := one(t, statesOf(t, r, at)); g.Gate != "learned:test:r:go-test@d:repo" || g.Kind != "test" {
			t.Fatalf("at %d: Gate %q Kind %q, want learned:test:r:go-test@d:repo and test", at, g.Gate, g.Kind)
		}
	}
	// With no recorded kind in the record, the last run's inferred one.
	if g := one(t, statesAt(t, 2, old)); g.Gate != "learned:custom:r:go-test@d:repo" {
		t.Fatalf("Gate %q, want learned:custom:r:go-test@d:repo", g.Gate)
	}
}

// Two recorded kinds (a runner reclassified between versions): the last
// recorded names the gate.
func TestVER27b_TheLastRecordedKindWins(t *testing.T) {
	asBuild := call{seq: 2, tool: "Bash", prog: "go", class: shape.VerbPackage, runner: "go-test", kind: shape.RunnerBuild, masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo"}
	old := call{seq: 3, tool: "Bash", prog: "go", class: shape.VerbPackage, runner: "go-test", masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo", schema: 5}
	g := one(t, statesAt(t, 6, gateCall(1, "go-test", store.ExecOK), asBuild, old))
	if g.Gate != "learned:build:r:go-test@d:repo" {
		t.Fatalf("Gate %q, want learned:build:r:go-test@d:repo: test recorded first, build last, the last run inferred", g.Gate)
	}
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
	if got := gateStates(runs, writes, unnamed, at, nil); len(got) != 2 || got[0].WritesSinceGreen != 10 {
		b.Fatalf("the fixture gives %+v, want two families with ten writes since green", got)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		gateStates(runs, writes, unnamed, at, nil)
	}
}

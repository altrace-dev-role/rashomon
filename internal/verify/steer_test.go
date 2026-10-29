package verify

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// stEvent is the event every regression-location test raises it at: the
// failed runner call (card 2.3 R3).
const stEvent = string(steer.EventPostToolUseFailure)

// minuteClock stamps every declaration of r at its call's seq in minutes,
// so a test reads "how long ago" off the seqs it wrote: a green at call 2
// and a red at call 8 are six minutes apart. runOf leaves RecordedAtMS 0.
func minuteClock(t *testing.T, r *store.Run) {
	t.Helper()
	const t0 = 1_800_000_000_000
	for i := range r.Declarations {
		n, err := strconv.ParseInt(strings.TrimPrefix(r.Declarations[i].ToolUseID, "t"), 10, 64)
		if err != nil {
			t.Fatalf("minuteClock: tool_use_id %q is not runOf's", r.Declarations[i].ToolUseID)
		}
		r.Declarations[i].RecordedAtMS = t0 + n*60_000
	}
}

// callPoint is the per-call point of call seq n: the seq its execution
// landed at, as LandExecution returns it to the hook (card 2.3 R2), the
// empty kind (D55), prompt p1, the main agent.
func callPoint(t *testing.T, r *store.Run, n int64) Point {
	t.Helper()
	id := "t" + strconv.FormatInt(n, 10)
	for _, x := range r.Executions {
		if x.ToolUseID == id && x.Seq != nil {
			return Point{Seq: *x.Seq, SessionID: "s1", PromptID: strp("p1")}
		}
	}
	t.Fatalf("callPoint: call %d has no execution in the ordered stream", n)
	return Point{}
}

// facts is the producer's output at call n's point, for call n's id.
func facts(t *testing.T, r *store.Run, n int64, pt Point) []Finding {
	t.Helper()
	var c classified
	snap := evaluate(r, nil, nil, pt, time.Time{}, &c)
	return regressionLocations(snap, c.runs, c.writes, pt, stEvent, "t"+strconv.FormatInt(n, 10))
}

// viewAt is SteerAt at call n's own point, as the hook calls it.
func viewAt(t *testing.T, r *store.Run, n int64) SteerView {
	t.Helper()
	return SteerAt(r, nil, nil, callPoint(t, r, n), time.Time{}, stEvent, "t"+strconv.FormatInt(n, 10))
}

// gateOf is the one gate of r at pt.
func gateOf(t *testing.T, r *store.Run, pt Point) string {
	t.Helper()
	snap := Evaluate(r, nil, nil, pt, time.Time{})
	if len(snap.Gates) != 1 {
		t.Fatalf("premise: %d gates at seq %d, want 1", len(snap.Gates), pt.Seq)
	}
	return snap.Gates[0].Gate
}

// callDecl is call n's declaration seq, the seq a RunRef cites (declSeq
// reads by index).
func callDecl(t *testing.T, r *store.Run, n int64) int64 {
	t.Helper()
	id := "t" + strconv.FormatInt(n, 10)
	for _, d := range r.Declarations {
		if d.ToolUseID == id {
			return d.Seq
		}
	}
	t.Fatalf("callDecl: no declaration for call %d", n)
	return 0
}

// redirect is gate call c with its own line writing out.txt: the write
// shares the run's declaration seq (A7), so only strict bounds leave it out.
func redirect(c call) call {
	c.paths = []string{"out.txt"}
	return c
}

// TestSTR43b_ProducerNamesDistinctPathsBetweenTheRuns: the finding names
// the distinct paths written strictly between the last green and this red,
// first write first, and counts every relevant write there, named or not.
// A write before the green, one after the red, a ci path, and each run's
// own redirect are not between; a path outside the slot charset is counted
// as omitted, never named.
func TestSTR43b_ProducerNamesDistinctPathsBetweenTheRuns(t *testing.T) {
	r := runOf(t,
		edit(1, "internal/before.go"),
		redirect(gateCall(2, "go", store.ExecOK)),
		edit(3, "internal/a.go"),
		edit(4, "internal/a.go"),
		edit(5, ".github/workflows/ci.yml"),
		call{seq: 6, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true},
		edit(7, "internal/a b.go"),
		redirect(gateCall(8, "go", store.ExecFailed)),
		edit(9, "internal/after.go"),
	)
	minuteClock(t, r)
	for i := range r.Declarations {
		r.Declarations[i].Project = strp("/repo")
	}
	pt := callPoint(t, r, 8)
	gate := gateOf(t, r, pt)

	got := facts(t, r, 8, pt)
	if len(got) != 1 || got[0].Gate == nil {
		t.Fatalf("regressionLocations = %+v, want one gate finding", got)
	}
	f, g := got[0], got[0].Gate
	if f.Pattern != pattern.RegressionLocation || f.Class != Absence || f.Event != stEvent ||
		f.DedupeKey != "regression-location|"+gate+"|p1" {
		t.Errorf("finding = pattern %q, class %q, event %q, key %q", f.Pattern, f.Class, f.Event, f.DedupeKey)
	}
	if !slices.Equal(g.Paths, []PathSlot{"internal/a.go"}) || g.PathsOmitted != 1 || g.WritesSince != 4 {
		t.Errorf("paths %q, omitted %d, writes since %d; want [internal/a.go], 1 (the path outside the charset), 4 "+
			"(a.go twice, the unknown target, the bad path; not the ci path, not either redirect)",
			g.Paths, g.PathsOmitted, g.WritesSince)
	}
	if g.GateID != SlotID(gate) || g.Program != "go" || g.State != Red || g.Reason != FailedExit ||
		intOr0(g.LastGreenSeq) != callDecl(t, r, 2) || intOr0(g.LastRunSeq) != callDecl(t, r, 8) ||
		g.ExitCode == nil || *g.ExitCode != 1 {
		t.Errorf("gate fact = %+v", *g)
	}

	view := viewAt(t, r, 8)
	if len(view.Findings) != 1 || view.Dropped != 0 || view.ProjectRoot != "/repo" {
		t.Fatalf("SteerAt = %d findings, %d dropped, root %q; want 1, 0, /repo", len(view.Findings), view.Dropped, view.ProjectRoot)
	}
	sf := view.Findings[0]
	want := "rashomon record: gate 'go' exited 0 earlier in this session (6 min ago) and exited 1 on this run. " +
		"Files written between the two runs: 'internal/a.go' and 2 more."
	if sf.Message.Text() != want || sf.Key != gate || sf.AfterSeq != pt.Seq ||
		!slices.Equal(sf.Evidence, []int64{callDecl(t, r, 2), callDecl(t, r, 8)}) {
		t.Errorf("steer finding: text %q, key %q, after %d, evidence %v; want %q, %q, %d, the two run seqs",
			sf.Message.Text(), sf.Key, sf.AfterSeq, sf.Evidence, want, gate, pt.Seq)
	}
}

// TestSTR43c_NoWritesBetweenIsNoFinding: a red after a green with no
// relevant write between is not a located regression; a ci path alone is
// not a relevant write. The control, an edit between, is one.
func TestSTR43c_NoWritesBetweenIsNoFinding(t *testing.T) {
	cases := []struct {
		name  string
		calls []call
		want  int
	}{
		{"control: an edit between", []call{gateCall(1, "go", store.ExecOK), edit(2, "internal/a.go"), gateCall(3, "go", store.ExecFailed)}, 1},
		{"nothing between", []call{gateCall(1, "go", store.ExecOK), gateCall(3, "go", store.ExecFailed)}, 0},
		{"a ci path only", []call{gateCall(1, "go", store.ExecOK), edit(2, ".github/workflows/ci.yml"), gateCall(3, "go", store.ExecFailed)}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, tc.calls...)
			minuteClock(t, r)
			if got := facts(t, r, 3, callPoint(t, r, 3)); len(got) != tc.want {
				t.Errorf("%d findings, want %d", len(got), tc.want)
			}
		})
	}
}

// TestSTR43d_NoEarlierGreenIsNoFinding: a red with no green before it in
// the session has no "exited 0 earlier" to state.
func TestSTR43d_NoEarlierGreenIsNoFinding(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first string
		want  int
	}{
		{"control: green first", store.ExecOK, 1},
		{"red first", store.ExecFailed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, gateCall(1, "go", tc.first), edit(2, "internal/a.go"), gateCall(3, "go", store.ExecFailed))
			minuteClock(t, r)
			if got := facts(t, r, 3, callPoint(t, r, 3)); len(got) != tc.want {
				t.Errorf("%d findings, want %d", len(got), tc.want)
			}
		})
	}
}

// TestSTR43e_OtherAgentsAreCounted: OtherAgents counts the writes between
// whose agent is not the point's, the main agent (nil) differing from any
// subagent.
func TestSTR43e_OtherAgentsAreCounted(t *testing.T) {
	sub := edit(2, "internal/a.go")
	sub.agent = "a1"
	r := runOf(t, gateCall(1, "go", store.ExecOK), sub, edit(3, "internal/b.go"), edit(4, "internal/c.go"), gateCall(5, "go", store.ExecFailed))
	minuteClock(t, r)
	main := callPoint(t, r, 5)
	inSub := main
	inSub.AgentID = strp("a1")
	for _, tc := range []struct {
		name string
		pt   Point
		want int
	}{
		{"from the main agent", main, 1},
		{"from subagent a1", inSub, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := facts(t, r, 5, tc.pt)
			if len(got) != 1 || got[0].Gate.OtherAgents != tc.want {
				t.Fatalf("findings %+v, want one with OtherAgents %d", got, tc.want)
			}
		})
	}
}

// TestSTR43f_DegradedRowWhenNoPathIsNamed: writes between with no path
// named (a Bash write with no known target) render the degraded row with
// their count; the control, a named path, renders the full row.
func TestSTR43f_DegradedRowWhenNoPathIsNamed(t *testing.T) {
	unnamed := call{seq: 2, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true}
	for _, tc := range []struct {
		name     string
		between  call
		template string
		text     string
	}{
		{"control: a named path", edit(2, "internal/a.go"), "regression-location/v1",
			"rashomon record: gate 'go' exited 0 earlier in this session (2 min ago) and exited 1 on this run. " +
				"Files written between the two runs: 'internal/a.go'."},
		{"no path named", unnamed, "regression-location-degraded/v1",
			"rashomon record: gate 'go' exited 0 earlier in this session (2 min ago) and exited 1 on this run. " +
				"1 file writes were recorded between the two runs."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, gateCall(1, "go", store.ExecOK), tc.between, gateCall(3, "go", store.ExecFailed))
			minuteClock(t, r)
			view := viewAt(t, r, 3)
			if len(view.Findings) != 1 {
				t.Fatalf("%d findings, %d dropped; want 1, 0", len(view.Findings), view.Dropped)
			}
			m := view.Findings[0].Message
			if m.Template() != tc.template || m.Text() != tc.text {
				t.Errorf("message %q, %q; want %q, %q", m.Template(), m.Text(), tc.template, tc.text)
			}
		})
	}
}

// TestSTR43g_MinutesOverTheSlotRangeRefuses: a span over the minutes slot's
// range (10,080, one week) refuses the render, an honest refusal over a
// clamped number: the finding stays, with the zero Message, so Route logs
// render_refused. At the bound the row renders.
func TestSTR43g_MinutesOverTheSlotRangeRefuses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		minutes int64
		renders bool
	}{
		{"control: at the bound", 10_080, true},
		{"one over", 10_081, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, gateCall(1, "go", store.ExecOK), edit(2, "internal/a.go"), gateCall(3, "go", store.ExecFailed))
			minuteClock(t, r)
			r.Declarations[len(r.Declarations)-1].RecordedAtMS = r.Declarations[0].RecordedAtMS + tc.minutes*60_000
			view := viewAt(t, r, 3)
			if len(view.Findings) != 1 {
				t.Fatalf("%d findings, %d dropped; want the finding kept either way", len(view.Findings), view.Dropped)
			}
			text := view.Findings[0].Message.Text()
			if (text != "") != tc.renders {
				t.Errorf("text %q; want rendered %v", text, tc.renders)
			}
			if tc.renders && !strings.Contains(text, "(10080 min ago)") {
				t.Errorf("text %q, want it to say 10080 min ago", text)
			}
		})
	}
}

// TestSTR43j_AllNamedPathsSecretShapedFallToDegraded: when the renderer
// leaves out every named path (deny list or entropy test), the full row is
// refused with ErrNoPathsLeft and the degraded row renders the count
// instead (A10).
func TestSTR43j_AllNamedPathsSecretShapedFallToDegraded(t *testing.T) {
	for _, tc := range []struct {
		name, path, template string
	}{
		{"control: a plain path", "internal/a.go", "regression-location/v1"},
		{"a key-shaped path", "AKIAIOSFODNN7EXAMPLE.txt", "regression-location-degraded/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, gateCall(1, "go", store.ExecOK), edit(2, tc.path), gateCall(3, "go", store.ExecFailed))
			minuteClock(t, r)
			view := viewAt(t, r, 3)
			if len(view.Findings) != 1 {
				t.Fatalf("%d findings, %d dropped; want 1, 0", len(view.Findings), view.Dropped)
			}
			m := view.Findings[0].Message
			if m.Template() != tc.template {
				t.Errorf("template %q, want %q (text %q)", m.Template(), tc.template, m.Text())
			}
			if strings.Contains(m.Text(), "AKIA") {
				t.Errorf("text %q names the key-shaped path", m.Text())
			}
		})
	}
}

// TestSTR43k_AConcurrentWriteAfterTheRedIsNoCandidate: a write declared
// after the red's declaration and before its execution landed makes the
// reason red_then_writes, which the candidate excludes, so nothing reaches
// the mapper and nothing is dropped (A8, D67).
func TestSTR43k_AConcurrentWriteAfterTheRedIsNoCandidate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		concurrent bool
		want       int
	}{
		{"control: the write lands after the red's execution", false, 1},
		{"the write lands before it", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, gateCall(1, "go", store.ExecOK), edit(2, "internal/a.go"), gateCall(3, "go", store.ExecFailed), edit(4, "internal/b.go"))
			minuteClock(t, r)
			if tc.concurrent {
				// The red's execution lands after the late edit's two records.
				for i := range r.Executions {
					if r.Executions[i].ToolUseID == "t3" {
						late := callDecl(t, r, 4) + 2
						r.Executions[i].Seq = &late
					}
				}
			}
			pt := callPoint(t, r, 3)
			if tc.concurrent {
				if s := Evaluate(r, nil, nil, pt, time.Time{}); len(s.Gates) != 1 || s.Gates[0].Reason != RedThenWrites {
					t.Fatalf("premise: gates %+v, want one with reason red_then_writes", s.Gates)
				}
			}
			view := SteerAt(r, nil, nil, pt, time.Time{}, stEvent, "t3")
			if len(view.Findings) != tc.want || view.Dropped != 0 {
				t.Errorf("%d findings, %d dropped; want %d, 0", len(view.Findings), view.Dropped, tc.want)
			}
		})
	}
}

// TestST_SteerAtDropsAnUnmappableFact: a fact ToSteer refuses is a contract
// break between the producer and the mapper, counted in Dropped and never
// passed on; the good fact beside it still is.
func TestST_SteerAtDropsAnUnmappableFact(t *testing.T) {
	r := runOf(t, gateCall(1, "go", store.ExecOK), edit(2, "internal/a.go"), gateCall(3, "go", store.ExecFailed))
	minuteClock(t, r)
	pt := callPoint(t, r, 3)
	good := facts(t, r, 3, pt)
	if len(good) != 1 {
		t.Fatalf("premise: %d facts, want 1", len(good))
	}
	bad := good[0]
	g := *bad.Gate
	g.GateID = "not a slot id"
	bad.Gate = &g
	got, dropped := mapFacts(r, []Finding{bad, good[0]}, pt)
	if len(got) != 1 || dropped != 1 || got[0].Message.Text() == "" {
		t.Errorf("mapFacts = %d findings, %d dropped; want the good one rendered and 1 dropped", len(got), dropped)
	}
}

// TestST_ProgramFallsBackToTheKind: a program outside the fact's charset,
// or none at all, leaves Program empty, and the note names the gate by its
// kind instead of the finding being dropped by the mapper.
func TestST_ProgramFallsBackToTheKind(t *testing.T) {
	for _, tc := range []struct {
		name, prog, word string
	}{
		{"control: go", "go", "'go'"},
		{"outside the charset", "GoTest", "'test'"},
		{"no program", "", "'test'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			green, red := gateCall(1, "go", store.ExecOK), gateCall(3, "go", store.ExecFailed)
			green.prog, red.prog = tc.prog, tc.prog
			r := runOf(t, green, edit(2, "internal/a.go"), red)
			minuteClock(t, r)
			view := viewAt(t, r, 3)
			if len(view.Findings) != 1 || view.Dropped != 0 {
				t.Fatalf("%d findings, %d dropped; want 1, 0", len(view.Findings), view.Dropped)
			}
			if text := view.Findings[0].Message.Text(); !strings.Contains(text, "gate "+tc.word+" exited 0") {
				t.Errorf("text %q, want the gate named %s", text, tc.word)
			}
		})
	}
}

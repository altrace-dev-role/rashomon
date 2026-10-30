package verify

import (
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// snEvent is the event stale-evidence is raised at: the post of the write.
const snEvent = string(steer.EventPostToolUse)

// snNow is the judge's clock, an hour after minuteClock's t0, inside the
// index's thirty days.
var snNow = time.UnixMilli(1_800_000_000_000 + 3_600_000)

// snIndex is a project index in which each family was seen in sessions, the
// last at minuteClock's t0: two make it required (RequiredSessions).
func snIndex(sessions int, families ...string) *ProjectIndex {
	idx := &ProjectIndex{ProjectKey: "/repo"}
	for _, f := range families {
		idx.Entries = append(idx.Entries, IndexEntry{Family: f, Kind: shape.RunnerTest, Sessions: sessions,
			LastSession: "s0", FirstSeen: 1_800_000_000_000, LastSeen: 1_800_000_000_000})
	}
	slices.SortFunc(idx.Entries, entryOrder)
	return idx
}

// snRun is r with minuteClock's times and /repo as every call's project.
func snRun(t *testing.T, calls ...call) *store.Run {
	t.Helper()
	r := runOf(t, calls...)
	minuteClock(t, r)
	for i := range r.Declarations {
		r.Declarations[i].Project = strp("/repo")
	}
	return r
}

// staleAt is SteerAt at call n's own point at its post, under idx.
func staleAt(t *testing.T, r *store.Run, n int64, idx *ProjectIndex) SteerView {
	t.Helper()
	return SteerAt(r, idx, nil, callPoint(t, r, n), snNow, snEvent, "t"+strconv.FormatInt(n, 10))
}

// staleFindings is the view's stale-evidence candidates.
func staleFindings(v SteerView) []steer.Finding {
	var out []steer.Finding
	for _, f := range v.Findings {
		if f.Pattern == pattern.StaleEvidence {
			out = append(out, f)
		}
	}
	return out
}

// TestSN1_TheWriteAfterGreenRaisesOnce (card STALE-NUDGE1 R1): under an index
// that requires the family, the first edit after a green run raises one
// nudge naming the gate's program, how long ago it was green and the file
// the call wrote, keyed by the gate; a second edit raises none, because the
// write that left the green behind was the first one.
func TestSN1_TheWriteAfterGreenRaisesOnce(t *testing.T) {
	r := snRun(t, gateCall(2, "rt", store.ExecOK), edit(3, "internal/a.go"), edit(4, "internal/b.go"))
	idx := snIndex(2, "rt@d:repo")
	pt := callPoint(t, r, 3)
	gate := gateOf(t, r, pt)

	got := staleFindings(staleAt(t, r, 3, idx))
	if len(got) != 1 {
		t.Fatalf("the first edit after the green: %d stale-evidence findings, want 1", len(got))
	}
	sf := got[0]
	want := "rashomon record: gate 'go' exited 0 earlier in this session (1 min ago); this call then wrote 'internal/a.go'. " +
		"No later run of the gate is recorded."
	if sf.Message.Text() != want || sf.Key != gate || sf.MaxLevel != steer.LevelNudge || sf.AfterSeq != pt.Seq ||
		!slices.Equal(sf.Evidence, []int64{callDecl(t, r, 2)}) {
		t.Errorf("finding: text %q, key %q, level %v, after %d, evidence %v; want %q, %q, nudge, %d, the green run's seq",
			sf.Message.Text(), sf.Key, sf.MaxLevel, sf.AfterSeq, sf.Evidence, want, gate, pt.Seq)
	}
	if got := staleFindings(staleAt(t, r, 4, idx)); len(got) != 0 {
		t.Errorf("the second edit: %d stale-evidence findings, want 0 (the gate was left behind by the first)", len(got))
	}
}

// TestSN1b_ARerunClearsIt: a green rerun raises nothing at its own post, and
// the first edit after it raises again, the one after that nothing.
func TestSN1b_ARerunClearsIt(t *testing.T) {
	r := snRun(t, gateCall(2, "rt", store.ExecOK), edit(3, "internal/a.go"), gateCall(4, "rt", store.ExecOK),
		edit(5, "internal/b.go"), edit(6, "internal/c.go"))
	idx := snIndex(2, "rt@d:repo")
	for n, want := range map[int64]int{3: 1, 4: 0, 5: 1, 6: 0} {
		if got := staleFindings(staleAt(t, r, n, idx)); len(got) != want {
			t.Errorf("call %d: %d stale-evidence findings, want %d", n, len(got), want)
		}
	}
}

// TestSN1c_AdvisoryGatesStaySilent (D38; D164's departure): a gate the index
// does not require raises nothing, and neither does any gate while no
// family is established, with no index, an index whose families were seen
// once, or a learned index that requires another family; the gate the index
// requires raises.
func TestSN1c_AdvisoryGatesStaySilent(t *testing.T) {
	r := snRun(t, gateCall(2, "rt", store.ExecOK), edit(3, "internal/a.go"))
	for _, c := range []struct {
		name string
		idx  *ProjectIndex
		want int
	}{
		{"no index", nil, 0},
		{"seen once: no family established", snIndex(1, "rt@d:repo"), 0},
		{"another family required: this gate advisory", snIndex(2, "rv@d:repo"), 0},
		{"this family required", snIndex(2, "rt@d:repo"), 1},
	} {
		if got := staleFindings(staleAt(t, r, 3, c.idx)); len(got) != c.want {
			t.Errorf("%s: %d stale-evidence findings, want %d", c.name, len(got), c.want)
		}
	}
}

// TestSN1d_ForeignMoveIsNamedNotBlamed (INV-2): a subagent's edit declared
// between this call's declaration and its post is not this call's. The
// call that left the green behind names only its own file, and the
// subagent's call, which wrote after it, is not told it left the green
// behind.
func TestSN1d_ForeignMoveIsNamedNotBlamed(t *testing.T) {
	sub := edit(4, "internal/b.go")
	sub.agent = "sub1"
	r := snRun(t, gateCall(2, "rt", store.ExecOK), edit(3, "internal/a.go"), sub)
	// Interleave the two posts as parallel calls land: t4's declaration
	// before t3's execution.
	d3, d4 := declIndex(t, r, "t3"), declIndex(t, r, "t4")
	e3, e4 := execIndex(t, r, "t3"), execIndex(t, r, "t4")
	seqs := []int64{r.Declarations[d3].Seq, *r.Executions[e3].Seq, r.Declarations[d4].Seq, *r.Executions[e4].Seq}
	r.Declarations[d4].Seq = seqs[1]
	r.Executions[e3].Seq = &seqs[2]
	idx := snIndex(2, "rt@d:repo")

	got := staleFindings(staleAt(t, r, 3, idx))
	if len(got) != 1 {
		t.Fatalf("this call: %d stale-evidence findings, want 1", len(got))
	}
	want := "rashomon record: gate 'go' exited 0 earlier in this session (1 min ago); this call then wrote 'internal/a.go'. " +
		"No later run of the gate is recorded."
	if got[0].Message.Text() != want {
		t.Errorf("text %q, want %q (the subagent's file is not this call's)", got[0].Message.Text(), want)
	}
	if got := staleFindings(staleAt(t, r, 4, idx)); len(got) != 0 {
		t.Errorf("the subagent's call: %d stale-evidence findings, want 0 (its write came after the one that left the green behind)", len(got))
	}
}

// TestSN1e_PatternIsFrozen: stale-evidence is the eighteenth frozen id,
// after the spec's sixteen and task-preflight (merge order), with priority 78
// under regression-location and
// no kinds; the mapper takes it at a PostToolUse as a nudge keyed by the gate
// and refuses it at any other event.
func TestSN1e_PatternIsFrozen(t *testing.T) {
	all := pattern.All()
	if len(all) != 18 || all[17] != pattern.StaleEvidence || pattern.Kinds(pattern.StaleEvidence) != nil {
		t.Fatalf("All() = %v, Kinds = %v; want stale-evidence eighteenth, with no kinds", all, pattern.Kinds(pattern.StaleEvidence))
	}
	if p, ok := pattern.Priority(pattern.StaleEvidence); !ok || p != 78 {
		t.Errorf("Priority = (%d, %v), want (78, true)", p, ok)
	}
	green := int64(1)
	f := Finding{Pattern: pattern.StaleEvidence, Class: Absence, Event: snEvent,
		Gate: &GateFact{GateID: "learned:test:rt@d:repo", Program: "go", State: Stale, Reason: WritesAfterGreen,
			LastGreenSeq: &green, LastRunSeq: &green, WritesSince: 1, Paths: []PathSlot{"internal/a.go"}}}
	at := Point{Seq: 4, SessionID: "s1", PromptID: strp("p1")}
	sf, err := ToSteer(f, at, "")
	if err != nil || sf.MaxLevel != steer.LevelNudge || sf.StopForm != steer.StopFormNone || sf.Key != "learned:test:rt@d:repo" {
		t.Errorf("ToSteer at PostToolUse = %+v, %v; want a nudge keyed by the gate", sf, err)
	}
	f.Event = string(steer.EventPostToolUseFailure)
	var me *MappingError
	if _, err := ToSteer(f, at, ""); !errors.As(err, &me) {
		t.Errorf("ToSteer at PostToolUseFailure = %v, want a MappingError", err)
	}
}

// declIndex is the index in r.Declarations of the call whose id is id.
func declIndex(t *testing.T, r *store.Run, id string) int {
	t.Helper()
	i := slices.IndexFunc(r.Declarations, func(d store.Declaration) bool { return d.ToolUseID == id })
	if i < 0 {
		t.Fatalf("no declaration %s", id)
	}
	return i
}

// execIndex is the index in r.Executions of the call whose id is id.
func execIndex(t *testing.T, r *store.Run, id string) int {
	t.Helper()
	i := slices.IndexFunc(r.Executions, func(e store.Execution) bool { return e.ToolUseID == id })
	if i < 0 || r.Executions[i].Seq == nil {
		t.Fatalf("no ordered execution %s", id)
	}
	return i
}

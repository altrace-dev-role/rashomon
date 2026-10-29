package verify

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Seqs below are the builder's counter: with no terminal, call i has
// declaration 2i-1 and execution 2i.

// evalAt is Evaluate over r at a Stop at seq, with no index, no spec and no
// clock.
func evalAt(r *store.Run, seq int64) Snapshot {
	return Evaluate(r, nil, nil, Point{Kind: PointStop, Seq: seq, SessionID: "s1"}, time.Time{})
}

func encode(t *testing.T, snap Snapshot) []byte {
	t.Helper()
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// prefixOf is the record as it stood at the point s, cut by declaration (D37):
// the declarations at or before s with every outcome of theirs wherever it
// landed, the orphan outcomes by byPoint, every coverage record (they carry no
// Seq), and Skipped and Ignored as they are. It is what recording and
// gateStates read at s.
func prefixOf(r *store.Run, s int64) *store.Run {
	p := &store.Run{Dir: r.Dir, Coverage: r.Coverage, Skipped: r.Skipped, Ignored: r.Ignored}
	declared, kept := map[string]bool{}, map[string]bool{}
	for _, d := range r.Declarations {
		declared[d.ToolUseID] = true
		if d.Seq <= s {
			kept[d.ToolUseID] = true
			p.Declarations = append(p.Declarations, d)
		}
	}
	keep := func(id string, seq *int64) bool { return kept[id] || !declared[id] && byPoint(seq, s) }
	for _, e := range r.Executions {
		if keep(e.ToolUseID, e.Seq) {
			p.Executions = append(p.Executions, e)
		}
	}
	for _, x := range r.Terminals {
		if keep(x.ToolUseID, x.Seq) {
			p.Terminals = append(p.Terminals, x)
		}
	}
	return p
}

// bySeq is the record cut by record Seq at s, as a live reader holds it at s:
// an outcome that lands after s is not written yet.
func bySeq(r *store.Run, s int64) *store.Run {
	p := &store.Run{Dir: r.Dir, Coverage: r.Coverage, Skipped: r.Skipped, Ignored: r.Ignored}
	for _, d := range r.Declarations {
		if d.Seq <= s {
			p.Declarations = append(p.Declarations, d)
		}
	}
	for _, e := range r.Executions {
		if byPoint(e.Seq, s) {
			p.Executions = append(p.Executions, e)
		}
	}
	for _, x := range r.Terminals {
		if byPoint(x.Seq, s) {
			p.Terminals = append(p.Terminals, x)
		}
	}
	return p
}

func wantGreenAt(t *testing.T, snap Snapshot, want int64) {
	t.Helper()
	if !snap.RecordGreen || snap.RecordGreenAtSeq == nil || *snap.RecordGreenAtSeq != want {
		t.Fatalf("RecordGreen %t at %v, want true at %d: %+v", snap.RecordGreen, snap.RecordGreenAtSeq, want, snap.Gates)
	}
}

func wantNoGreen(t *testing.T, snap Snapshot) {
	t.Helper()
	if snap.RecordGreen || snap.RecordGreenAtSeq != nil {
		t.Fatalf("RecordGreen %t at %v, want false and nil: %+v", snap.RecordGreen, snap.RecordGreenAtSeq, snap.Gates)
	}
}

func TestVER40_EvaluateFillsGatesAndRecordGreen(t *testing.T) {
	t.Run("one gate", func(t *testing.T) {
		snap := evalAt(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)), 4)
		if snap.Recording != Complete || len(snap.Gates) != 1 || snap.Gates[0].State != Green || snap.Spec != nil {
			t.Fatalf("recording %s, gates %+v, spec %+v: want complete, one green gate, no spec", snap.Recording, snap.Gates, snap.Spec)
		}
		// LastGreen is the green run's declaration (3), not its execution.
		wantGreenAt(t, snap, 3)
	})
	t.Run("two greens, the later", func(t *testing.T) {
		read := call{seq: 3, tool: "Read", class: shape.VerbRead, outcome: store.ExecOK}
		snap := evalAt(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), read, gateCall(4, "go-vet", store.ExecOK)), 8)
		if len(snap.Gates) != 2 {
			t.Fatalf("gates %+v, want two", snap.Gates)
		}
		wantGreenAt(t, snap, 7)
	})
	t.Run("a stale required gate", func(t *testing.T) {
		snap := evalAt(runOf(t, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go")), 4)
		if len(snap.Gates) != 1 || snap.Gates[0].State != Stale {
			t.Fatalf("gates %+v, want one stale", snap.Gates)
		}
		wantNoGreen(t, snap)
	})
	t.Run("a green gate with no green run", func(t *testing.T) {
		// A gate Evaluate never builds; it fails closed, as TurnSummary reads it.
		if green, seq := recordGreen([]GateState{{State: Green, Required: true}}, 0); green || seq != nil {
			t.Fatalf("recordGreen %t at %v, want false and nil", green, seq)
		}
	})
}

func TestVER41_IncompleteRecordingSilencesEverything(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	r.Skipped = 1
	r.Declarations[0].Project = ptr("p1")
	sp := &spec.Compiled{
		Sources: []spec.Source{{Path: "/etc/rashomon/spec.json", Digest: spec.DigestPrefix + "11", Trust: spec.Managed}, {Path: ".claude/rashomon/spec.json", Digest: spec.DigestPrefix + "22", Trust: spec.Trusted}},
		Digest:  spec.DigestPrefix + strings.Repeat("ab", 32),
		Trust:   spec.Trusted,
	}
	snap := Evaluate(r, nil, sp, Point{Kind: PointStop, Seq: 4, SessionID: "s1"}, time.Time{})
	if snap.Recording != Incomplete || !slices.Equal(snap.Notes, []string{"skipped_lines"}) {
		t.Fatalf("recording %s notes %q, want incomplete with skipped_lines alone", snap.Recording, snap.Notes)
	}
	if snap.Gates == nil || len(snap.Gates) != 0 {
		t.Fatalf("gates %#v, want empty and non-nil", snap.Gates)
	}
	wantNoGreen(t, snap)
	// What the turn is still travels: where it ran, and under which spec.
	if snap.ProjectRoot != "p1" {
		t.Fatalf("ProjectRoot %q, want p1", snap.ProjectRoot)
	}
	if snap.Spec == nil || snap.Spec.Digest != sp.Digest || snap.Spec.Path != ".claude/rashomon/spec.json" {
		t.Fatalf("spec %+v, want the repository spec's identity", snap.Spec)
	}
	if s := TurnSummary(snap, TurnCounts{Writes: 1, GateRuns: 1}); s.Row != RowUnknown || s.Sentence != sentenceRecordingIncomplete {
		t.Fatalf("row %q sentence %q, want unknown with the recording sentence", s.Row, s.Sentence)
	}
}

func TestVER42_SeqZeroCountsNothing(t *testing.T) {
	snap := evalAt(runOf(t, gateCall(1, "go-test", store.ExecOK)), 0)
	if snap.Recording != Complete || snap.Gates == nil || len(snap.Gates) != 0 {
		t.Fatalf("recording %s gates %#v, want complete with no gate", snap.Recording, snap.Gates)
	}
	wantNoGreen(t, snap)

	// A spilled orphan outcome cannot be placed, so it is at every point,
	// Seq 0 included (recording's byPoint).
	snap = evalAt(runOf(t, lost(edit(1, "pkg/a.go"))), 0)
	if snap.Recording != Incomplete || !slices.Equal(snap.Notes, []string{"outcome_without_declaration"}) || len(snap.Gates) != 0 {
		t.Fatalf("recording %s notes %q gates %+v, want incomplete with outcome_without_declaration and no gate", snap.Recording, snap.Notes, snap.Gates)
	}
	wantNoGreen(t, snap)
}

func TestVER43_TheValueLineRowsFollowTheOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		calls []call
		turn  TurnCounts
		want  Row
	}{
		{"verified", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)}, TurnCounts{Writes: 1, GateRuns: 1}, RowVerified},
		{"unverified", []call{gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go")}, TurnCounts{Writes: 1, GateRuns: 1}, RowUnverified},
		{"red", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed)}, TurnCounts{Writes: 1, GateRuns: 1}, RowRed},
		{"no gate", []call{edit(1, "pkg/a.go")}, TurnCounts{Writes: 1}, RowNoGate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The point is the builder's last Seq, 2i for the last call.
			snap := evalAt(runOf(t, tc.calls...), 2*int64(len(tc.calls)))
			if got := TurnSummary(snap, tc.turn).Row; got != tc.want {
				t.Fatalf("row %q, want %q: %+v", got, tc.want, snap.Gates)
			}
		})
	}
}

func TestVER44_ShuffledRecordOrderGivesIdenticalBytes(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "pkg/b.go"),
		gateCall(4, "go-vet", store.ExecOK), bashWrite(5, "pkg/c.go"))
	r.Declarations[0].Project, r.Declarations[2].Project = ptr("p1"), ptr("p2")
	r.Coverage = append(r.Coverage, verifiedAt(store.PhaseStart), verifiedAt(store.PhaseEnd))
	want := encode(t, evalAt(r, 10))
	if snap := evalAt(r, 10); snap.ProjectRoot != "p2" || len(snap.Gates) != 2 {
		t.Fatalf("ProjectRoot %q, %d gates, want p2 and two: the fixture must carry both", snap.ProjectRoot, len(snap.Gates))
	}
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(r.Declarations), func(i, j int) { r.Declarations[i], r.Declarations[j] = r.Declarations[j], r.Declarations[i] })
		rng.Shuffle(len(r.Executions), func(i, j int) { r.Executions[i], r.Executions[j] = r.Executions[j], r.Executions[i] })
		rng.Shuffle(len(r.Terminals), func(i, j int) { r.Terminals[i], r.Terminals[j] = r.Terminals[j], r.Terminals[i] })
		rng.Shuffle(len(r.Coverage), func(i, j int) { r.Coverage[i], r.Coverage[j] = r.Coverage[j], r.Coverage[i] })
		if got := encode(t, evalAt(r, 10)); !bytes.Equal(got, want) {
			t.Fatalf("shuffle %d:\n%s\nwant\n%s", i, got, want)
		}
	}
}

func TestVER45_EmptyListsMarshalAsEmptyNotNull(t *testing.T) {
	complete := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	incomplete := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	incomplete.Skipped = 1
	for name, r := range map[string]*store.Run{"complete": complete, "incomplete": incomplete} {
		b := encode(t, evalAt(r, 4))
		for _, key := range []string{"gates", "claims", "spec_verdicts", "notes", "unresolved"} {
			if !bytes.Contains(b, []byte(`"`+key+`":[`)) || bytes.Contains(b, []byte(`"`+key+`":null`)) {
				t.Errorf("%s: %q is not a list: %s", name, key, b)
			}
		}
	}
}

// The families gateCall's runners give at the builder's location.
const (
	testFamily = "r:go-test@d:repo"
	vetFamily  = "r:go-vet@d:repo"
)

// TestVER46_RequiredFollowsTheIndex: with an index that has learned a family,
// a gate is required when the index requires its family at now (Required);
// with no index, or one that requires nothing yet, every gate is (D38). A
// gate not required keeps its state, never blocks the record green, and its
// LastGreen is not the record's (D36). With only advisory gates, the row
// reads them capped: green is never verified, red is never hidden.
func TestVER46_RequiredFollowsTheIndex(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	nowMS := now.UnixMilli()
	// index holds go-test, last seen at testSeen in two sessions, and go-vet,
	// seen now in vetSessions.
	index := func(testSeen int64, vetSessions int) *ProjectIndex {
		return &ProjectIndex{ProjectKey: "k", Entries: []IndexEntry{
			{Family: testFamily, Kind: "test", Sessions: 2, LastSession: "s0", FirstSeen: testSeen, LastSeen: testSeen},
			{Family: vetFamily, Kind: "test", Sessions: vetSessions, LastSession: "s0", FirstSeen: nowMS, LastSeen: nowMS},
		}}
	}
	aged := nowMS - RequiredWindowMS - 1
	// Call i declares at 2i-1: a gate's LastGreen is its call's 2i-1.
	green := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), gateCall(3, "go-vet", store.ExecOK))
	vetStale := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-vet", store.ExecOK), edit(3, "pkg/a.go"), gateCall(4, "go-test", store.ExecOK))
	testStale := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "pkg/a.go"), gateCall(4, "go-vet", store.ExecOK))
	eval := func(r *store.Run, idx *ProjectIndex) Snapshot {
		return Evaluate(r, idx, nil, Point{Kind: PointStop, Seq: 2 * int64(len(r.Declarations)), SessionID: "s1"}, now)
	}
	wantRequired := func(t *testing.T, snap Snapshot, test, vet bool) {
		t.Helper()
		got := map[string]bool{}
		for _, g := range snap.Gates {
			got[g.Family] = g.Required
		}
		if len(snap.Gates) != 2 || len(got) != 2 || got[testFamily] != test || got[vetFamily] != vet {
			t.Fatalf("required %v over %+v, want %s %t and %s %t", got, snap.Gates, testFamily, test, vetFamily, vet)
		}
	}
	wantRow := func(t *testing.T, snap Snapshot, turn TurnCounts, row Row, sentence string) {
		t.Helper()
		if s := TurnSummary(snap, turn); s.Row != row || s.Sentence != sentence {
			t.Fatalf("row %q sentence %q, want %q and %q", s.Row, s.Sentence, row, sentence)
		}
	}

	t.Run("a: the index requires go-test alone", func(t *testing.T) {
		snap := eval(green, index(nowMS, 1))
		wantRequired(t, snap, true, false)
		wantGreenAt(t, snap, 3) // go-test's run; go-vet's at 5 is not the record's
	})
	t.Run("b: a stale gate not required does not block", func(t *testing.T) {
		snap := eval(vetStale, index(nowMS, 1))
		wantRequired(t, snap, true, false)
		wantGreenAt(t, snap, 7)
	})
	t.Run("c: a stale required gate blocks", func(t *testing.T) {
		snap := eval(testStale, index(nowMS, 1))
		wantNoGreen(t, snap)
		// The required set is not empty, so the row is the required path's,
		// not advisory: go-vet's green beside it changes nothing.
		wantRow(t, snap, TurnCounts{Writes: 1, GateRuns: 2}, RowUnverified, "")
	})
	t.Run("d: a family past the window no longer blocks", func(t *testing.T) {
		// go-vet is still required, so the index has learned: without it the
		// index would read as none (e1) and go-test would block again.
		snap := eval(testStale, index(aged, 2))
		wantRequired(t, snap, false, true)
		wantGreenAt(t, snap, 7)
	})
	t.Run("e1: an index that requires nothing reads as none", func(t *testing.T) {
		snap := eval(green, index(aged, 1))
		wantRequired(t, snap, true, true)
		wantGreenAt(t, snap, 5)
		wantRow(t, snap, TurnCounts{Writes: 1, GateRuns: 2}, RowVerified, "")
	})
	// go-test is established but did not run this turn: go-vet is advisory.
	t.Run("e2: an advisory green is unverified, never verified", func(t *testing.T) {
		snap := eval(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-vet", store.ExecOK)), index(nowMS, 1))
		if len(snap.Gates) != 1 || snap.Gates[0].Required || snap.Gates[0].State != Green {
			t.Fatalf("gates %+v, want go-vet alone, green and not required", snap.Gates)
		}
		wantNoGreen(t, snap)
		wantRow(t, snap, TurnCounts{Writes: 1, GateRuns: 1}, RowUnverified, sentenceAdvisoryOnly)
	})
	t.Run("e3: an advisory red is never hidden", func(t *testing.T) {
		snap := eval(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-vet", store.ExecFailed)), index(nowMS, 1))
		wantNoGreen(t, snap)
		wantRow(t, snap, TurnCounts{Writes: 1, GateRuns: 1}, RowRed, sentenceAdvisoryOnly)
	})
	t.Run("g: an advisory green with no write ran only", func(t *testing.T) {
		snap := eval(runOf(t, gateCall(1, "go-vet", store.ExecOK)), index(nowMS, 1))
		if len(snap.Gates) != 1 || snap.Gates[0].Required || snap.Gates[0].State != Green {
			t.Fatalf("gates %+v, want go-vet alone, green and not required", snap.Gates)
		}
		wantRow(t, snap, TurnCounts{GateRuns: 1}, RowRanOnly, sentenceAdvisoryOnly)
	})
	t.Run("f: no index, every gate is required", func(t *testing.T) {
		snap := eval(green, nil)
		wantRequired(t, snap, true, true)
		wantGreenAt(t, snap, 5)
		wantNoGreen(t, eval(vetStale, nil))
		wantNoGreen(t, eval(testStale, nil))
	})
}

func TestVER47_ProjectRootIsTheLastCountedDeclarations(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), edit(2, "pkg/b.go"))
	if got := evalAt(r, 4).ProjectRoot; got != "" {
		t.Fatalf("no project: ProjectRoot %q, want empty", got)
	}
	r.Declarations[0].Project, r.Declarations[1].Project = ptr("p1"), ptr("p2")
	if got := evalAt(r, 1).ProjectRoot; got != "p1" {
		t.Fatalf("at the first declaration: ProjectRoot %q, want p1", got)
	}
	if got := evalAt(r, 3).ProjectRoot; got != "p2" {
		t.Fatalf("at the second declaration: ProjectRoot %q, want p2", got)
	}
	// The nearest declaration that names a project, not the last one's own.
	r.Declarations[1].Project = nil
	if got := evalAt(r, 3).ProjectRoot; got != "p1" {
		t.Fatalf("p1 then none: ProjectRoot %q, want p1", got)
	}
}

// The plan's INV-11 property over the prefix by declaration (D37), at every
// Seq of the record, the in-flight ones included.
func TestRES14b_EvaluateAtAPointEqualsEvaluateOverThePrefix(t *testing.T) {
	full := runOf(t, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"), gateCall(3, "go-test", store.ExecFailed), edit(4, "pkg/b.go"))
	for s := int64(0); s <= 8; s++ {
		a, b := encode(t, evalAt(full, s)), encode(t, evalAt(prefixOf(full, s), s))
		if !bytes.Equal(a, b) {
			t.Fatalf("at %d the point cut differs from the prefix:\n%s\n%s", s, a, b)
		}
	}
}

// A gate's kind is read over the whole family record (kindOf, D33 item 3):
// a runner whose recorded kind lands after the point names its gate by that
// kind at the point, and by its inferred kind in the prefix. That token is
// the one difference INV-11 allows.
func TestRES14c_InvariantElevenModuloIdentity(t *testing.T) {
	v5 := call{seq: 1, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-ci", masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo", schema: 5}
	v6 := v5
	v6.seq, v6.kind, v6.schema = 2, shape.RunnerBuild, 0
	full := runOf(t, v5, v6)
	cut, prefix := evalAt(full, 2), evalAt(prefixOf(full, 2), 2)
	if len(cut.Gates) != 1 || len(prefix.Gates) != 1 {
		t.Fatalf("gates %+v and %+v, want one each", cut.Gates, prefix.Gates)
	}
	g, p := &cut.Gates[0], &prefix.Gates[0]
	if g.Family != p.Family || g.Kind != shape.RunnerBuild || p.Kind == g.Kind {
		t.Fatalf("family %q and %q, kind %q and %q: want one family, recorded build at the point, inferred otherwise in the prefix", g.Family, p.Family, g.Kind, p.Kind)
	}
	for _, x := range []*GateState{g, p} {
		x.Gate, x.Kind = strings.Replace(x.Gate, "learned:"+x.Kind+":", "learned:", 1), ""
	}
	if a, b := encode(t, cut), encode(t, prefix); !bytes.Equal(a, b) {
		t.Fatalf("beyond the kind token the point cut differs from the prefix:\n%s\n%s", a, b)
	}
	if a, b := encode(t, evalAt(full, 4)), encode(t, evalAt(prefixOf(full, 4), 4)); !bytes.Equal(a, b) {
		t.Fatalf("with the recorded kind in both, at 4:\n%s\n%s", a, b)
	}
}

// 1.4's existence rule (VER32b) makes a point with a call in flight complete
// on replay, where the outcome is in the record, and silent live, where it is
// not yet written. Stop points carry no call in flight.
func TestRES14d_AnInFlightPointIsSilentLiveAndCompleteOnReplay(t *testing.T) {
	full := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	live := evalAt(bySeq(full, 3), 3)
	if live.Recording != Incomplete || !slices.Equal(live.Notes, []string{"declaration_without_outcome"}) || live.Gates == nil || len(live.Gates) != 0 {
		t.Fatalf("live at 3: recording %s notes %q gates %#v, want incomplete, declaration_without_outcome, no gate", live.Recording, live.Notes, live.Gates)
	}
	if replay := evalAt(full, 3); replay.Recording != Complete {
		t.Fatalf("replay at 3: recording %s notes %q, want complete", replay.Recording, replay.Notes)
	}
}

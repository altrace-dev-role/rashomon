package verify

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The obligation ledger (PRD section 5, PF-PG5) is a view over what Evaluate
// computed: one row per thing a required gate owes at the point, its state,
// and the record seqs that back it. It stores nothing and adds no field to
// the Snapshot; replaying the record rebuilds it byte for byte (PG5c), and a
// row whose evidence is missing is unknown, never met (INV-2). The ledger at
// a point equals the ledger over the record's prefix at that point (INV-11),
// with Evaluate's one stated exception (RES14c) reaching it as row order
// only: a gate's label, which snap.Gates sorts by, can differ between the
// two, and no row carries the label, so ids, states and evidence cannot. Done is
// snap.RecordGreen, never "every row met" and never an empty ledger: in v0
// every evidence_bound row is unknown, and an incomplete record's ledger is
// empty (Q9).
//
// The mapping, per required gate (D36, D38: an advisory gate owes nothing),
// rows in snap.Gates order and per gate in ObligationKinds order:
//   - gate_after_write, from the gate's state (gateAfterWriteTable). An Absent
//     gate with no write owes nothing and has no row of any kind.
//   - watched_red_then_green (PF-PG7), from classify's runs and writes
//     (watchedRow): no row without a red run.
//   - exit_unmasked, from the state, where the gate has a last run and owes
//     a row (exitUnmaskedTable).
//   - evidence_bound, unknown with no evidence, where gate_after_write has a
//     row (Phase 5 binds the evidence to the tree digest; Q6).
//
// A cited pointer that is nil makes its row unknown with no evidence (Q2). A
// state outside States, a source outside snapshot.go's three, or a record not
// Complete makes every row of the gate unknown (INV-2, A1).
//
// The three v0 facts: the scope is the session at every point (Q1); the
// evidence_bound rows are unknown; and no spec_rule, platform_impact or
// proposed_check row is emitted (Phase 3 and 6). expected_evidence rows
// come from the task file (evidence.go, card 3.3).

// ObligationKind is what an obligation owes (PRD section 5).
type ObligationKind string

const (
	GateAfterWrite      ObligationKind = "gate_after_write"       // a gate ran green after the last relevant write
	WatchedRedThenGreen ObligationKind = "watched_red_then_green" // the family went red, then green after a write
	ExitUnmasked        ObligationKind = "exit_unmasked"          // the run's status was readable, not hidden by a pipe
	EvidenceBound       ObligationKind = "evidence_bound"         // Phase 5 fills it; v0 always unknown
	SpecRule            ObligationKind = "spec_rule"              // Phase 3
	PlatformImpact      ObligationKind = "platform_impact"        // Phase 6
	ExpectedEvidence    ObligationKind = "expected_evidence"      // Phase 3
	ProposedCheck       ObligationKind = "proposed_check"         // Phase 3
)

// ObligationKinds is the closed list in the PRD's order; v0 emits the first
// four.
var ObligationKinds = []ObligationKind{
	GateAfterWrite, WatchedRedThenGreen, ExitUnmasked, EvidenceBound,
	SpecRule, PlatformImpact, ExpectedEvidence, ProposedCheck,
}

// ObligationState is a row's state. Unknown never fires and never reads met
// (INV-2).
type ObligationState string

const (
	ObligationOpen    ObligationState = "open"
	ObligationMet     ObligationState = "met"
	ObligationUnmet   ObligationState = "unmet"
	ObligationUnknown ObligationState = "unknown"
)

// The scopes an obligation is owed in (PRD section 5).
const (
	ScopeSession = "session"
	ScopeSubtask = "subtask"
	ScopeCommit  = "commit"
)

// obligationScopes is the scope each point kind owes in: the session at every
// one in v0 (Q1). The window is the run (D40), and an id digests its scope,
// so a subtask or commit label over session-wide evidence would change
// meaning under the same id once those windows land.
func obligationScopes() map[string]string {
	obligationScopesOnce.Do(func() {
		obligationScopesOnce.v = map[string]string{
			PointStop: ScopeSession, PointSubagentStop: ScopeSession, PointSubtask: ScopeSession,
			PointCommit: ScopeSession, PointPush: ScopeSession, PointPR: ScopeSession, PointQuery: ScopeSession,
		}
	})
	return obligationScopesOnce.v
}

// obligationScopesOnce holds what obligationScopes returns, built on its first
// call and never at init (RES-2).
var obligationScopesOnce struct {
	sync.Once
	v map[string]string
}

// obligationSourceSpec is the source of a declared or ci_commands gate.
const obligationSourceSpec = "spec"

// Obligation is one ledger row. ID is a stable digest (ObligationID);
// Evidence is record seqs only, ascending, never null.
type Obligation struct {
	ID       string          `json:"id"`
	Scope    string          `json:"scope"`
	Kind     ObligationKind  `json:"kind"`
	Source   string          `json:"source"`
	State    ObligationState `json:"state"`
	Evidence []int64         `json:"evidence"`
}

// EvaluateWithObligations is Evaluate and the ledger over its snapshot, from
// one classification. Evaluate keeps its signature (Snapshot is frozen); the
// ledger is never nil.
func EvaluateWithObligations(run *store.Run, idx *ProjectIndex, sp *spec.Compiled, pt Point, now time.Time) (Snapshot, []Obligation) {
	var c classified
	snap := evaluate(run, idx, sp, pt, now, &c)
	ex := expected(snap, evidenceIn{sp: sp, wrote: WroteRelevant(run, pt.Seq)})
	scope := cmp.Or(obligationScopes()[snap.Point.Kind], ScopeSession)
	return snap, append(obligations(snap, c.runs, c.writes), expectedObligations(scope, ex)...)
}

// cite is the gate pointers a row's evidence reads.
type cite uint8

const (
	citeRun   cite = 1 << iota // LastRun
	citeGreen                  // LastGreen
	citeWrite                  // LastWrite
)

// obRow is one line of a kind's table: a gate in state in owes out, citing
// cites. A state no line names reads unknown with no evidence.
type obRow struct {
	in    State
	out   ObligationState
	cites cite
}

// gateAfterWriteTable: green after the last relevant write is met; red, or
// green with writes since, is unmet; a gate with a write and no run, or a run
// still in the background, is open; a hidden or unknown outcome is unknown.
var gateAfterWriteTable = []obRow{
	{Green, ObligationMet, citeGreen},
	{Red, ObligationUnmet, citeRun},
	{Stale, ObligationUnmet, citeRun | citeWrite},
	{Absent, ObligationOpen, citeWrite},
	{Backgrounded, ObligationOpen, citeRun},
	{MaskedInconclusive, ObligationUnknown, citeRun},
	{Unknown, ObligationUnknown, citeRun},
}

// exitUnmaskedTable: a status was read on Green, Red and Stale, and on an
// Absent gate's run, an empty run, which the state table reads only where no
// pipe hid it (D47); a pipe hid it on Masked; none was read yet on
// Backgrounded, and none on Unknown (output unread, an unended run or a
// spilled execution: Gate B F2).
var exitUnmaskedTable = []obRow{
	{Green, ObligationMet, citeRun},
	{Red, ObligationMet, citeRun},
	{Stale, ObligationMet, citeRun},
	{Absent, ObligationMet, citeRun},
	{MaskedInconclusive, ObligationUnmet, citeRun},
	{Backgrounded, ObligationUnknown, citeRun},
	{Unknown, ObligationUnknown, citeRun},
}

// ledger builds the rows. Every row's Evidence is a window of one backing
// array, made with room for the most a gate cites (two, three and one), and
// capped so an append to one never writes into the next; the array is never
// nil, so neither is a window, and an empty one encodes as [].
type ledger struct {
	rows     []Obligation
	evidence []int64
	scope    string
}

// obligations is the view: the ledger over snap, with classify's runs and
// writes, both in seq order, for the watched kind. It reads nothing else.
// Cost: one pass over the runs into a map per family with a red, O(R), then
// per required gate a table lookup and, for the watched kind, a binary search
// and a backward scan of the writes, O(G(log W + W)) at worst.
func obligations(snap Snapshot, runs []gateRun, writes []write) []Obligation {
	n := 0
	for i := range snap.Gates {
		if snap.Gates[i].Required {
			n++
		}
	}
	if n == 0 {
		return []Obligation{}
	}
	l := ledger{
		rows:     make([]Obligation, 0, 4*n),
		evidence: make([]int64, 0, 6*n),
		scope:    cmp.Or(obligationScopes()[snap.Point.Kind], ScopeSession),
	}
	reds := watches(runs, snap.Point.Seq)
	for i := range snap.Gates {
		g := &snap.Gates[i]
		if !g.Required {
			continue
		}
		source, known := obligationSource(g.Source)
		start := len(l.rows)
		owed := g.State != Absent || g.LastWrite != nil
		if owed {
			l.fromTable(g, source, GateAfterWrite, gateAfterWriteTable)
		}
		if w, ok := reds[g.Family]; ok {
			state, ev, n := watchedRow(w, writes)
			l.add(g, source, WatchedRedThenGreen, state, ev[:n]...)
		}
		if g.LastRun != nil && owed {
			l.fromTable(g, source, ExitUnmasked, exitUnmaskedTable)
		}
		if owed {
			l.add(g, source, EvidenceBound, ObligationUnknown)
		}
		if snap.Recording != Complete || Rank(g.State) < 0 || !known {
			for j := start; j < len(l.rows); j++ {
				l.rows[j].State = ObligationUnknown
			}
		}
	}
	return l.rows
}

// obligationSource is a gate's source as a row carries it, and whether it is
// one of the three GateState.Source names (snapshot.go); another is carried
// as given.
func obligationSource(s string) (string, bool) {
	switch s {
	case "declared", "ci_commands":
		return obligationSourceSpec, true
	case sourceLearned:
		return sourceLearned, true
	}
	return s, false
}

// fromTable adds g's row of kind by table.
func (l *ledger) fromTable(g *GateState, source string, kind ObligationKind, table []obRow) {
	row := obRow{out: ObligationUnknown}
	for _, r := range table {
		if r.in == g.State {
			row = r
			break
		}
	}
	var ev [3]int64
	n := 0
	for _, c := range [...]struct {
		bit cite
		seq *int64
	}{{citeRun, runSeq(g.LastRun)}, {citeGreen, runSeq(g.LastGreen)}, {citeWrite, writeSeq(g.LastWrite)}} {
		if row.cites&c.bit == 0 {
			continue
		}
		if c.seq == nil {
			l.add(g, source, kind, ObligationUnknown)
			return
		}
		ev[n] = *c.seq
		n++
	}
	l.add(g, source, kind, row.out, ev[:n]...)
}

func runSeq(r *RunRef) *int64 {
	if r == nil {
		return nil
	}
	return &r.Seq
}

func writeSeq(w *WriteRef) *int64 {
	if w == nil {
		return nil
	}
	return &w.Seq
}

// add appends g's row of kind, its evidence sorted ascending without repeats.
func (l *ledger) add(g *GateState, source string, kind ObligationKind, state ObligationState, evidence ...int64) {
	from := len(l.evidence)
	l.evidence = append(l.evidence, evidence...)
	slices.Sort(l.evidence[from:])
	l.evidence = l.evidence[:from+len(slices.Compact(l.evidence[from:]))]
	l.rows = append(l.rows, Obligation{
		ID: ObligationID(l.scope, kind, g.Family), Scope: l.scope, Kind: kind, Source: source, State: state,
		Evidence: l.evidence[from:len(l.evidence):len(l.evidence)],
	})
}

// ObligationID is "ob:" and the first 16 hex of sha256 over scope, kind and
// family joined by newlines: the family, not the gate's label, which can
// change between reads (D33 item 3). None of the three holds a newline.
// Exported for the rows another package builds (internal/platform).
func ObligationID(scope string, kind ObligationKind, family string) string {
	var a [128]byte
	b := append(append(append(append(append(a[:0], scope...), '\n'), kind...), '\n'), family...)
	sum := sha256.Sum256(b)
	var id [3 + 16]byte
	copy(id[:], "ob:")
	hex.Encode(id[3:], sum[:8])
	return string(id[:])
}

// watch is one family's red at the point: its last red run, and the first
// green run after it, nil where none has landed.
type watch struct {
	red, green *gateRun
}

// watches is every family with a red run at the point, by its Family name
// (stateOf's runner@location). A run counts once its outcome has landed at or
// before at, in declaration order, as gateStates reads them. A red is a run
// the state table would read Red (Q3): not backgrounded, not masked, not an
// empty run (D47), failed; a later red resets the green.
func watches(runs []gateRun, at int64) map[string]watch {
	var byKey map[familyKey]watch
	for i := range runs {
		r := &runs[i]
		k, ok := r.family()
		if s, known := landed(*r); !ok || !known || s > at {
			continue
		}
		switch w := byKey[k]; {
		case !r.Backgrounded && !masked(r) && !emptyRun(r) && failed(r):
			if byKey == nil {
				byKey = make(map[familyKey]watch)
			}
			byKey[k] = watch{red: r}
		case w.red != nil && w.green == nil && green(r):
			w.green = r
			byKey[k] = w
		}
	}
	if len(byKey) == 0 {
		return nil
	}
	out := make(map[string]watch, len(byKey))
	for k, w := range byKey {
		out[k.runner+"@"+k.location] = w
	}
	return out
}

// watchedRow is a family's watched state and evidence. Open with no green
// after the red; met when a relevant named write lies after the red landed
// and before the green was declared, citing the last such write; unmet, a
// pass by re-run, otherwise. A write declared while the red was in flight is
// not a response to it, so it never meets the row (Q4: the mirror of D31).
// The red is the last one, so a fix followed by a flake passed by re-run is
// unmet (Q5): stricter than the PRD's "was red, then green after a write" at
// some point, and closed on a late flake.
func watchedRow(w watch, writes []write) (ObligationState, [3]int64, int) {
	if w.green == nil {
		return ObligationOpen, [3]int64{w.red.Seq}, 1
	}
	lo, _ := landed(*w.red)
	hi, _ := slices.BinarySearchFunc(writes, w.green.Seq, func(x write, seq int64) int { return cmp.Compare(x.Seq, seq) })
	for i := hi - 1; i >= 0 && writes[i].Seq > lo; i-- {
		if x := &writes[i]; x.Path != nil && Relevant(x.Class) {
			return ObligationMet, [3]int64{w.red.Seq, x.Seq, w.green.Seq}, 3
		}
	}
	return ObligationUnmet, [3]int64{w.red.Seq, w.green.Seq}, 2
}

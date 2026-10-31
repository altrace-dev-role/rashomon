package verify

import (
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// Expected evidence (card 3.3, D117): the task file (3.2) states the demand,
// the evidence kinds the person expects; the spec (3.1) states the supply,
// which declared gate's run carries which kind (Gate.Supplies); the record is
// in between. One row per kind the task names, in spec.EvidenceKinds order.

// SourceTask is the Source of every expected_evidence row: the task file.
const SourceTask = "task"

// Expected is one expected_evidence row as the Stop clause reads it: the
// evidence kind, its state, the gate the state was read from ("" for a text
// kind or when no gate supplies the kind) and, when unmet, why, in the words
// the clause prints. An Obligation carries none of the three (D-A1).
type Expected struct {
	Kind   string
	State  ObligationState
	Gate   string
	Reason string
}

// The unmet reasons, as the done flag's clause prints them.
const (
	reasonNoGate       = "no gate declares it"
	reasonNoJudgment   = "the final message discloses no judgment"
	reasonNoMechanism  = "the task file states no mechanism"
	reasonTargetedTask = "a tool call targeted the task file"
)

// reasonNoGreen is the reason of a run kind whose supplying gate has no green
// run after the last relevant write.
func reasonNoGreen(gate string) string { return "no green run of " + gate + " since the last edit" }

// The kinds read from the final message and the task rather than a gate.
const (
	kindJudgment  = "judgment_disclosed"
	kindMechanism = "mechanism_stated"
)

// evidenceIn is what the rows read besides the snapshot: the compiled spec
// (its task and its gates' supplies), whether the point is the turn's Stop,
// whether no final message is held (unread) and, when one is, whether it
// discloses a judgment (both read at Stop only), and whether the window
// holds a relevant write.
type evidenceIn struct {
	sp        *spec.Compiled
	atStop    bool
	unread    bool
	disclosed bool
	wrote     bool
}

// expected is the rows for the task's kinds, never nil. No task, no rows. An
// incomplete recording reads every row unknown (INV-2, as the ledger does).
// Cost: O(K x G) with K at most five and G the spec's gates.
func expected(snap Snapshot, in evidenceIn) []Expected {
	if in.sp == nil || in.sp.Task == nil {
		return []Expected{}
	}
	out := make([]Expected, 0, len(spec.EvidenceKinds))
	for _, kind := range spec.EvidenceKinds {
		if !slices.Contains(in.sp.Task.ExpectedEvidence, kind) {
			continue
		}
		var e Expected
		switch kind {
		case kindJudgment:
			e = judgmentRow(in)
		case kindMechanism:
			e = mechanismRow(snap, in)
		default:
			e = runRow(snap, in, kind)
		}
		e.Kind = kind
		if snap.Recording != Complete {
			e = Expected{Kind: kind, State: ObligationUnknown}
		}
		out = append(out, e)
	}
	return out
}

// runRow reads a run kind through gate_after_write over each declared gate
// whose supplies carry it: met when any is met; otherwise the first such gate
// in the spec's order is the one read and named. No supplying gate is open
// before Stop and unmet at it; at Stop an open gate is unmet too, the turn
// having ended without the run (D-A2).
func runRow(snap Snapshot, in evidenceIn, kind string) Expected {
	first := Expected{State: ObligationOpen}
	found := false
	for _, d := range in.sp.Gates {
		if !slices.Contains(d.Gate.Supplies, kind) {
			continue
		}
		state := gateAfterWrite(snap, d.ID)
		if state == ObligationMet {
			return Expected{State: ObligationMet, Gate: d.ID}
		}
		if !found {
			first, found = Expected{State: state, Gate: d.ID}, true
		}
	}
	switch {
	case !found && in.atStop:
		return Expected{State: ObligationUnmet, Reason: reasonNoGate}
	case !found:
		return first
	case first.State == ObligationUnmet || first.State == ObligationOpen && in.atStop:
		first.State, first.Reason = ObligationUnmet, reasonNoGreen(first.Gate)
	}
	return first
}

// gateAfterWrite is the gate_after_write state of the snapshot's gate id: the
// ledger's own table (obligations.go). A gate the snapshot does not hold has
// no run and no write, which the table reads as Absent: open.
func gateAfterWrite(snap Snapshot, id string) ObligationState {
	state := Absent
	for i := range snap.Gates {
		if snap.Gates[i].Gate == id {
			state = snap.Gates[i].State
			break
		}
	}
	for _, r := range gateAfterWriteTable {
		if r.in == state {
			return r.out
		}
	}
	return ObligationUnknown
}

// judgmentRow: met when the final message discloses a judgment, unmet at Stop
// otherwise, open before Stop; unknown at a Stop with no message held, since
// a message not read is no judgment withheld (INV-2).
func judgmentRow(in evidenceIn) Expected {
	switch {
	case !in.atStop:
		return Expected{State: ObligationOpen}
	case in.unread:
		return Expected{State: ObligationUnknown}
	case in.disclosed:
		return Expected{State: ObligationMet}
	}
	return Expected{State: ObligationUnmet, Reason: reasonNoJudgment}
}

// mechanismRow: open before the first relevant write; then unmet while the
// task states no mechanism, unmet when a tool call targeted the task file
// (3.2's StoreTouched entry on it: a mechanism an agent may have written is
// not the person's, D-A3), and met otherwise. The entry is the declared
// write, outcome-blind as the ledger's writes are (D135), so the reason says
// the call targeted the file, never that it wrote it.
func mechanismRow(snap Snapshot, in evidenceIn) Expected {
	switch {
	case !in.wrote:
		return Expected{State: ObligationOpen}
	case in.sp.Task.Mechanism == "":
		return Expected{State: ObligationUnmet, Reason: reasonNoMechanism}
	case taskTargeted(snap):
		return Expected{State: ObligationUnmet, Reason: reasonTargetedTask}
	}
	return Expected{State: ObligationMet}
}

// taskTargeted reports a StoreTouched entry on the task file.
func taskTargeted(snap Snapshot) bool {
	for _, e := range snap.Integrity.Unresolved {
		if e.Check == StoreTouched && spec.IsTaskFile(e.Path) {
			return true
		}
	}
	return false
}

// UnmetExpected is the rows of ex whose expected_evidence row in obs, the
// ledger they were projected into, is unmet, in ex's order (EvidenceKinds'),
// reasons kept: what the done flag's clause names (Gate B 1, D-A1), its state
// read from the ledger (card LEDGER-WIRE1), its words from ex. An unknown row
// is not unmet (INV-2), and neither is a kind the ledger holds no row for.
// Cost: O(K x R) over at most five kinds and the ledger's expected rows.
func UnmetExpected(ex []Expected, obs []Obligation) []Expected {
	var out []Expected
	for _, e := range ex {
		for _, o := range obs {
			if o.Kind == ExpectedEvidence && o.State == ObligationUnmet && o.ID == ObligationID(o.Scope, ExpectedEvidence, e.Kind) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// expectedObligations is the ledger's rows for ex: the evidence kind is the
// row's family, so its ID is stable per kind. A met run row cites the
// supplying gate's last green run, read from snap at projection time (card
// 3.3b, D143); every other row, both text kinds included, cites nothing, as
// a run row's supplying gate has its own rows citing the write and the red.
func expectedObligations(scope string, snap Snapshot, ex []Expected) []Obligation {
	out := make([]Obligation, 0, len(ex))
	for _, e := range ex {
		out = append(out, Obligation{ID: ObligationID(scope, ExpectedEvidence, e.Kind), Scope: scope,
			Kind: ExpectedEvidence, Source: SourceTask, State: e.State, Evidence: expectedCites(snap, e)})
	}
	return out
}

// expectedCites is e's citation: the supplying gate's last green run when e
// is a met run row, else an empty list, never nil.
func expectedCites(snap Snapshot, e Expected) []int64 {
	if e.State != ObligationMet || e.Gate == "" {
		return []int64{}
	}
	for i := range snap.Gates {
		if g := &snap.Gates[i]; g.Gate == e.Gate && g.LastGreen != nil {
			return []int64{g.LastGreen.Seq}
		}
	}
	return []int64{}
}

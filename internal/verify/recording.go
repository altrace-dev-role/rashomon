package verify

import (
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// notes is the closed table of what recording says of an incomplete record,
// one note per condition, in the order it gives them.
var notes = [...]string{
	"skipped_lines",
	"declaration_without_outcome",
	"outcome_without_declaration",
	"spilled_outcome",
	"coverage_unverified",
	"coverage_absent",
}

// The conditions, by their note's index in notes.
const (
	noteSkipped = iota
	noteNoOutcome
	noteOrphan
	noteSpilled
	noteUnverified
	noteAbsent
)

// recording is whether the run's record is complete at the point at, so that
// classify and gateStates may speak for it (2.2.5, Silence), and if not, why:
// Incomplete with one note per condition that holds, in the order of notes,
// else Complete with nil notes. It is pure, and its answer does not depend on
// the order of the run's records: every condition is an existence test or a
// count. A record with a gap fails closed (INV-2).
//
//   - skipped_lines: the reader passed over a line it could not read
//     (store.Run.Skipped). An ignored record is not a gap: a non-critical one
//     an older reader may pass over.
//   - declaration_without_outcome: a declaration at or before at has neither
//     an execution nor a terminal in the record. Existence, not order: an
//     outcome that landed after at is recorded, and whether a run stands at
//     the point is gateStates' to say.
//   - outcome_without_declaration: an execution or a terminal in the record
//     by the point (byPoint) has no declaration, so what the call was is
//     lost, and a lost Edit is a write gateStates never sees (D35). A spilled
//     one cannot be placed, so it counts at every point. D35 names the two
//     shapes a lost declaration leaves, an ordered execution and a spilled
//     terminal; this one rule over both kinds is a superset of that, and
//     counts an ordered orphan terminal and a spilled orphan execution too,
//     which no writer leaves today: a record missing a declaration has a gap
//     whichever outcome shows it, and one predicate is simpler than two.
//   - spilled_outcome: an outcome of a declaration at or before at has no Seq
//     (#104): recorded, but the store lost when it landed (D33).
//   - coverage_unverified: no post record is verified, or one resolved its
//     hook entry or its probe to unknown, whatever the others say. A record is
//     verified only when its State says so and it names no reason, the rule
//     internal/report's rollup applies.
//   - coverage_absent: the run has no coverage record at all.
//
// Cost: one pass over the declarations into a map sized up front, one over
// the outcomes and one over the map and the coverage records,
// O(D + E + T + C); the map is the one allocation of a complete record.
func recording(run *store.Run, at int64) (Recording, []string) {
	var fault [len(notes)]bool
	fault[noteSkipped] = run.Skipped > 0

	ids := make(map[string]declState, len(run.Declarations))
	for _, d := range run.Declarations {
		s := ids[d.ToolUseID]
		s.due = s.due || d.Seq <= at
		ids[d.ToolUseID] = s
	}
	// answer marks the declaration answered by every outcome, wherever it
	// landed, so it stands on its own line: no condition may short-circuit
	// past it.
	for _, e := range run.Executions {
		declared := answer(ids, e.ToolUseID, e.Seq)
		if !declared && byPoint(e.Seq, at) {
			fault[noteOrphan] = true
		}
	}
	for _, t := range run.Terminals {
		declared := answer(ids, t.ToolUseID, t.Seq)
		if !declared && byPoint(t.Seq, at) {
			fault[noteOrphan] = true
		}
	}
	for _, s := range ids {
		fault[noteNoOutcome] = fault[noteNoOutcome] || s.due && !s.answered
		fault[noteSpilled] = fault[noteSpilled] || s.due && s.spilled
	}

	var verified, unknown bool
	for _, c := range run.Coverage {
		if c.Phase != store.PhasePost {
			continue
		}
		verified = verified || c.State == store.StateVerified && c.Reason == nil
		unknown = unknown || c.HookEntry == store.EntryUnknown || c.Probe == store.ProbeUnknown
	}
	fault[noteUnverified] = len(run.Coverage) > 0 && (!verified || unknown)
	fault[noteAbsent] = len(run.Coverage) == 0

	var buf [len(notes)]string
	out := buf[:0]
	for i, f := range fault {
		if f {
			out = append(out, notes[i])
		}
	}
	if len(out) == 0 {
		return Complete, nil
	}
	return Incomplete, slices.Clone(out)
}

// declState is what the record holds for one declared ToolUseID.
type declState struct {
	due      bool // a declaration of it is at or before the point
	answered bool // an execution or a terminal of it is in the record
	spilled  bool // one of those has no Seq
}

// answer marks id's declaration answered by an outcome at seq, and reports
// whether id is declared at all.
func answer(ids map[string]declState, id string, seq *int64) bool {
	s, ok := ids[id]
	if ok {
		s.answered, s.spilled = true, s.spilled || seq == nil
		ids[id] = s
	}
	return ok
}

// byPoint: an outcome at seq is in the record by the point at. A spilled one
// has no place, so it is at every point.
func byPoint(seq *int64, at int64) bool { return seq == nil || *seq <= at }

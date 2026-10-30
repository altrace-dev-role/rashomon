package verify

import (
	"path"
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// oneWayDoor reports the checks of PRD PF-IV3's closed list that VERIFY has
// a kind for (card P5B, 5.7): a test removed after a red run, a weakened
// gate run, an edited gate configuration and a write under .claude/rashomon.
// The list grows only by ruling. Only StoreTouched has a producer today; the
// other three raise from whatever entry their producers come to write
// (D139, IV-PRODUCERS1).
func oneWayDoor(k IntegrityKind) bool {
	switch k {
	case TestRemoved, GateWeakened, GateConfigEdited, StoreTouched:
		return true
	}
	return false
}

// integrityRaises is pattern.Integrity's producer at a post (card P5B, 5.7):
// each unresolved one-way-door entry this call made, its seq the seq of the
// declaration recorded under this call's tool_use_id, as a user-only fact.
// An entry an earlier call made was raised at that call, so no later call
// raises it again. A TestRemoved entry is raised only when a gate run failed
// before the edit (PF-IV3: "a test removed after a red run"). A store_touched
// entry outside .claude/rashomon is a write under the store root (D173): its
// fact says so and names the path relative to that root. An entry with no
// path is a call that named the root on a record that withholds its paths
// (D173b): its fact is StoreNamed, and its line says named. A path the
// fact slot refuses is counted in PathsOmitted, and the line renders
// without it.
func integrityRaises(run *store.Run, snap Snapshot, runs []gateRun, event, toolUseID string) []Finding {
	seq := callDeclSeq(run, toolUseID)
	if seq == 0 {
		return nil
	}
	var out []Finding
	for _, e := range snap.Integrity.Unresolved {
		if e.Seq != seq || !oneWayDoor(e.Check) || e.Check == TestRemoved && !failedBefore(runs, seq) {
			continue
		}
		fact := IntegrityFact{Kind: e.Check, Count: 1, Seq: e.Seq, Marker: MarkerID(e.Marker)}
		shown := e.Path
		if e.Check == StoreTouched && !inStore(e.Path) {
			fact.StoreRoot, fact.StoreNamed = true, e.Path == ""
			if rel, ok := under(path.Clean(e.Path), path.Clean(slashed(snap.Point.StoreRoot))); ok {
				shown = rel
			}
		}
		if p := PathSlot(shown); p.Valid() {
			fact.Paths = []PathSlot{p}
		} else if e.Path != "" {
			fact.PathsOmitted = 1
		}
		key := string(pattern.Integrity) + "|" + string(e.Check) + "|" + strconv.FormatInt(e.Seq, 10)
		out = append(out, Finding{Pattern: pattern.Integrity, Class: UserLabelled, Event: event, DedupeKey: key,
			Integrity: &fact, UserOnly: true})
	}
	return out
}

// callDeclSeq is the seq of the last declaration recorded under toolUseID,
// the capped id both records store, or 0 when there is none or no id.
func callDeclSeq(run *store.Run, toolUseID string) int64 {
	if run == nil || toolUseID == "" {
		return 0
	}
	for i := len(run.Declarations) - 1; i >= 0; i-- {
		if run.Declarations[i].ToolUseID == toolUseID {
			return run.Declarations[i].Seq
		}
	}
	return 0
}

// failedBefore reports a gate run declared before seq that is red as the
// gate states read one (failed: by its exit, or by its output). runs are in
// seq order.
func failedBefore(runs []gateRun, seq int64) bool {
	for i := range runs {
		if runs[i].Seq >= seq {
			break
		}
		if failed(&runs[i]) {
			return true
		}
	}
	return false
}

// renderIntegrity is an integrity fact's user line (archive 40.4.7; card
// P5B, 5.7). store-touched names its path when the slot takes it and falls
// to its no-path row when it does not, each in the store root's form for a
// write there (D173), and a fact from the store-root bit renders the root's
// named row (D173b); the other three doors are kind-only
// rows, since their facts carry no path (D139). Any other kind renders
// nothing, which Route logs as render_refused.
func renderIntegrity(f Finding) render.Message {
	var b any
	switch f.Integrity.Kind {
	case StoreTouched:
		if f.Integrity.StoreNamed {
			b = render.IntegrityStoreRootNamedBinding{}
			break
		}
		if len(f.Integrity.Paths) > 0 {
			var named any = render.IntegrityStoreTouchedBinding{Path: slot.Path(f.Integrity.Paths[0])}
			if f.Integrity.StoreRoot {
				named = render.IntegrityStoreRootTouchedBinding{Path: slot.Path(f.Integrity.Paths[0])}
			}
			if m, err := render.New(pattern.Integrity, named); err == nil {
				return m
			}
		}
		b = render.IntegrityStoreTouchedUnnamedBinding{}
		if f.Integrity.StoreRoot {
			b = render.IntegrityStoreRootTouchedUnnamedBinding{}
		}
	case TestRemoved:
		b = render.IntegrityTestRemovedBinding{}
	case GateWeakened:
		b = render.IntegrityGateWeakenedBinding{}
	case GateConfigEdited:
		b = render.IntegrityGateConfigEditedBinding{}
	default:
		return render.Message{}
	}
	m, _ := render.New(pattern.Integrity, b)
	return m
}

package verify

import (
	"cmp"
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The words a learned gate carries for v0: where it came from, how far it is
// trusted, and a run's scope, which nothing in the record states yet.
const (
	sourceLearned = "learned"
	trustBuiltin  = "builtin"
	scopeUnknown  = "unknown"
)

// The GateOutput kinds the table reads (record_v5.go, GateOutput.Kind).
const (
	outSummaryPass = "summary_pass"
	outSummaryFail = "summary_fail"
	outFailLine    = "fail_line"
	outNoSummary   = "no_summary"
	outEmptyRun    = "empty_run"
	outUnread      = "unread"
)

// gateStates is the state of every gate family present at the point at
// (2.2.4): one GateState per family with a run at or before at, sorted by
// Gate then Family, from what classify returned. It is pure: no clock, no
// store, no spec, and its output does not depend on the order of its inputs.
//
// A family is a runner at a location (gateRun.family); a run with no family
// credits none and is counted elsewhere (TurnCounts). A run is at the point
// when its outcome is: its execution's Seq at or before at, so a run in
// flight at the point is not yet a run there. A spilled execution has no Seq
// (#104): the run stands from its declaration, since its execution follows
// it, and as the last run it is Unknown, OrderUnknown, at every point, never
// a pass, since when its outcome landed is not known (D33).
//
// Per family, the last run (by declaration Seq) decides the state by the
// table below, first match wins. The reference run is the last green run, or
// the last run where none is green; a run is green when no row before Stale
// matches it, so a backgrounded, masked, failed, unread or spilled run is
// never one.
// Writes count strictly after the reference run's declaration, not its
// execution: a write declared while the run was in flight may be one the run
// never saw, and counting it stales the gate rather than crediting it (D31).
// A write counts when it names a relevant path (Relevant); every write counts
// against every family, since a write has no location. An unnamed write
// counts in UnknownTarget alone and changes no state (D30): the turn's row is
// where it fails closed (TurnCounts.UnnamedWriteSeq).
//
// The Gate's kind is the family's (kindOf), read over the whole record, so a
// gate keeps one name at every point; Family is the full key. AgentCount's
// AgentType is nil for v0: classify's write carries no agent type, though the
// declaration has one.
//
// Cost: one pass to bucket the runs, a sort per family, and one pass over the
// writes and the unnamed per family: O(R log R + F(W + U)).
func gateStates(runs []gateRun, writes []write, unnamed []int64, at int64) []GateState {
	byFamily := make(map[familyKey][]gateRun)
	for _, r := range runs {
		if k, ok := r.family(); ok {
			byFamily[k] = append(byFamily[k], r)
		}
	}
	out := make([]GateState, 0, len(byFamily))
	for k, fr := range byFamily {
		slices.SortFunc(fr, func(a, b gateRun) int { return cmp.Compare(a.Seq, b.Seq) })
		kind := kindOf(fr)
		fr = slices.DeleteFunc(fr, func(r gateRun) bool { s, _ := landed(r); return s > at })
		if len(fr) == 0 {
			continue
		}
		out = append(out, stateOf(k, kind, fr, writes, unnamed, at))
	}
	slices.SortFunc(out, func(a, b GateState) int {
		return cmp.Or(cmp.Compare(a.Gate, b.Gate), cmp.Compare(a.Family, b.Family))
	})
	return out
}

// landed is the Seq at which a run's outcome entered the record, and whether
// it is known: its execution's, or, where the execution was spilled, its
// declaration's, the earliest it can have been.
func landed(r gateRun) (seq int64, known bool) {
	if r.ExecSeq != nil {
		return *r.ExecSeq, true
	}
	return r.Seq, false
}

// kindOf is a family's kind, which names its gate: the last runner kind any
// of its runs records, in Seq order over the whole record, else the last
// run's inferred one. A runner whose recorded kind changed between versions
// takes the latest; the earlier one is not a second gate.
func kindOf(fr []gateRun) string {
	for i := len(fr) - 1; i >= 0; i-- {
		if fr[i].KindRecorded {
			return fr[i].Kind
		}
	}
	return fr[len(fr)-1].Kind
}

// fam is one family at the point, as the table reads it.
type fam struct {
	last    *gateRun
	green   *gateRun // the last green run; nil where none is
	ref     *gateRun // green, else last
	counted []write  // relevant named writes after ref and at or before at
}

// gateRow is one line of the table: when it matches, the state and its reason.
type gateRow struct {
	when   func(f *fam) bool
	state  State
	reason func(f *fam) Reason
}

// gateTable is the decision table of 2.2.4 as data, first match wins. The last
// row matches every family; the Unknown after the loop is the fail-closed
// answer for a table missing it, never reached.
var gateTable = []gateRow{
	{lastBackgrounded, Backgrounded, func(*fam) Reason { return BackgroundLaunch }},
	{lastMasked, MaskedInconclusive, maskedReason},
	{lastFailed, Red, redReason},
	{lastOutcomeNotKnown, Unknown, func(*fam) Reason { return OutcomeUnknown }},
	{lastOrderNotKnown, Unknown, func(*fam) Reason { return OrderUnknown }},
	{writtenSince, Stale, func(*fam) Reason { return WritesAfterGreen }},
	{func(*fam) bool { return true }, Green, func(*fam) Reason { return OK }},
}

func lastBackgrounded(f *fam) bool    { return f.last.Backgrounded }
func lastMasked(f *fam) bool          { return masked(f.last) }
func lastFailed(f *fam) bool          { return failed(f.last) }
func lastOutcomeNotKnown(f *fam) bool { return outcomeNotKnown(f.last) }
func lastOrderNotKnown(f *fam) bool   { return orderNotKnown(f.last) }
func writtenSince(f *fam) bool        { return len(f.counted) > 0 }

// masked: the line hid a runner's status, so its outcome is not the gate's.
func masked(r *gateRun) bool {
	return r.Masked != nil && (*r.Masked == shape.MaskedTest || *r.Masked == shape.MaskedBuild)
}

// failed: the run failed by its exit or said so in its output. An interrupt
// with a code is red; one without a code says nothing and falls through.
func failed(r *gateRun) bool {
	return failedByExit(r) || r.Output != nil && (r.Output.Kind == outSummaryFail || r.Output.Kind == outFailLine)
}

func failedByExit(r *gateRun) bool {
	return r.Outcome == store.ExecFailed ||
		r.Outcome == store.ExecInterrupted && r.ExitCode != nil && *r.ExitCode != 0
}

// outcomeNotKnown: a run no row above matched that did not end ok, or whose
// output was not read. Either way nobody saw it pass.
func outcomeNotKnown(r *gateRun) bool {
	return r.Outcome != store.ExecOK || r.Output != nil && r.Output.Kind == outUnread
}

// orderNotKnown: the run's execution was spilled, so when its outcome
// landed is not known. Below the rows of a known bad outcome, so it never
// hides a red.
func orderNotKnown(r *gateRun) bool {
	_, known := landed(*r)
	return !known
}

// green: no row before Stale matches the run.
func green(r *gateRun) bool {
	return !r.Backgrounded && !masked(r) && !failed(r) && !outcomeNotKnown(r) && !orderNotKnown(r)
}

// maskedReason is what the output said beside a hidden status.
func maskedReason(f *fam) Reason {
	if f.last.Output == nil {
		return MaskedNoSuccessLine
	}
	switch f.last.Output.Kind {
	case outSummaryPass:
		return MaskedWithSuccessLine
	case outNoSummary, outEmptyRun:
		return MaskedNoSuccessLine
	}
	return MaskedUnattributable
}

// redReason: writes after the red run first, then which source said fail.
func redReason(f *fam) Reason {
	for _, w := range f.counted {
		if w.Seq > f.last.Seq {
			return RedThenWrites
		}
	}
	if !failedByExit(f.last) {
		return FailedOutput
	}
	return FailedExit
}

// stateOf is one family's GateState; fr is its runs at the point, in Seq
// order, at least one, and kind is the family's.
func stateOf(k familyKey, kind string, fr []gateRun, writes []write, unnamed []int64, at int64) GateState {
	f := fam{last: &fr[len(fr)-1]}
	for i := len(fr) - 1; i >= 0; i-- {
		if green(&fr[i]) {
			f.green = &fr[i]
			break
		}
	}
	f.ref = cmp.Or(f.green, f.last)
	for _, w := range writes {
		if w.Path != nil && Relevant(w.Class) && w.Seq > f.ref.Seq && w.Seq <= at {
			f.counted = append(f.counted, w)
		}
	}
	g := GateState{
		Gate:             sourceLearned + ":" + kind + ":" + prefix12(k.runner) + "@" + prefix12(k.location),
		Kind:             kind,
		Family:           k.runner + "@" + k.location,
		Source:           sourceLearned,
		Trust:            trustBuiltin,
		Required:         true,
		State:            Unknown,
		Reason:           OutcomeUnknown,
		LastRun:          runRef(f.last),
		WritesSinceGreen: len(f.counted),
	}
	for _, r := range gateTable {
		if r.when(&f) {
			g.State, g.Reason = r.state, r.reason(&f)
			break
		}
	}
	if f.green != nil {
		g.LastGreen = runRef(f.green)
	}
	g.LastWrite = lastWrite(f.counted)
	g.WritersSinceGreen, g.WritersOmitted = writers(f.counted)
	for _, s := range unnamed {
		if s > f.ref.Seq && s <= at {
			g.UnknownTarget++
		}
	}
	return g
}

func prefix12(s string) string { return s[:min(len(s), 12)] }

// sameAgent: both the main agent (nil), or the same subagent.
func sameAgent(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func runRef(r *gateRun) *RunRef {
	return &RunRef{Seq: r.Seq, ToolUseID: r.ToolUseID, AgentID: r.AgentID, Outcome: r.Outcome,
		ExitCode: r.ExitCode, Masked: r.Masked, GateOutput: r.Output, Scope: scopeUnknown}
}

// lastWrite is the counted write with the highest Seq, the greatest path
// among one call's paths; nil with none.
func lastWrite(counted []write) *WriteRef {
	var last *write
	for i := range counted {
		w := &counted[i]
		if last == nil || w.Seq > last.Seq || w.Seq == last.Seq && *w.Path > *last.Path {
			last = w
		}
	}
	if last == nil {
		return nil
	}
	return &WriteRef{Seq: last.Seq, AgentID: last.AgentID, Path: last.Path, Tool: last.Tool}
}

// writers counts the counted writes per agent, in the order of each agent's
// first counted write, and keeps the first MaxWriters; omitted is the rest.
// The list is empty, never nil, so it encodes as [].
func writers(counted []write) (list []AgentCount, omitted int) {
	type agent struct {
		id    *string
		first int64
		n     int
	}
	var as []agent
	for _, w := range counted {
		i := slices.IndexFunc(as, func(a agent) bool { return sameAgent(a.id, w.AgentID) })
		if i < 0 {
			as = append(as, agent{id: w.AgentID, first: w.Seq})
			i = len(as) - 1
		}
		as[i].n++
		as[i].first = min(as[i].first, w.Seq)
	}
	slices.SortFunc(as, func(a, b agent) int { return cmp.Compare(a.first, b.first) })
	list = make([]AgentCount, 0, min(len(as), MaxWriters))
	for i, a := range as {
		if i == MaxWriters {
			return list, len(as) - MaxWriters
		}
		list = append(list, AgentCount{AgentID: a.id, Writes: a.n})
	}
	return list, 0
}

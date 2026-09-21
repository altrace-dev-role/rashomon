package digest

import (
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func (c *TurnCoverage) add(reason string) {
	for _, r := range c.Reasons {
		if r == reason {
			return
		}
	}
	c.Reasons = append(c.Reasons, reason)
}

// buildTurnCoverage is turn coverage's three-part rule, from the spec: the
// probe fired at start, the entry was present for the calls in this turn, and
// no gap intersects it. It returns the report.CoverageFacts rollup too, so
// the caller can read InstallID off it without a second pass over
// run.Coverage.
//
// run_not_closed is never added, by construction: report.RollupCoverage
// returns EndRecorded, and this function never once tests it against
// ReasonRunNotClosed the way report.build does. That omission IS H-84.
func buildTurnCoverage(run *store.Run, gaps []store.Gap, w turnWindow) (TurnCoverage, report.CoverageFacts) {
	global := report.RollupCoverage(run.Coverage)

	tc := TurnCoverage{
		Reasons:          []string{},
		StartRecorded:    global.StartRecorded,
		HookEntryAtStart: global.HookEntryAtStart,
	}

	// "the probe fired at start" -- session-wide, and the same fact for every
	// turn in the session: there is exactly one SessionStart per session and
	// it precedes every turn's own window.
	if !global.StartRecorded {
		tc.add(store.ReasonProbeAbsent)
	}

	// "the entry was present for the calls in this turn" -- windowed to this
	// turn's own span when one is known. A PreToolUse/PostToolUse coverage
	// record carries no tool_use_id or prompt_id of its own (RollupCoverage's
	// doc), so a time window is the only join available. Without one -- a
	// turn with no declarations of its own, including "no store" and "no
	// prompt found" -- the whole session's reasons are the best evidence
	// there is, under the same "degrade to the wider scope rather than assert
	// a boundary nobody wrote down" rule H-85's zero-vs-unknown split
	// depends on: a session that is demonstrably healthy elsewhere is
	// evidence a genuinely empty turn inside it is a real zero, and a
	// session that is not is exactly the "dead recorder" H-85 names.
	windowed := global
	if w.known {
		windowed = report.RollupCoverage(coverageInWindow(run.Coverage, w))
	}
	for _, r := range windowed.Reasons {
		tc.add(r)
	}

	// "no gap intersects it". Session-wide (any gap at all) under the same
	// fallback as above when the window is not known.
	for _, g := range gaps {
		if !w.known || gapIntersectsWindow(g, w) {
			tc.add(report.ReasonGap)
			break
		}
	}

	// Unterminated and dropped only count against the VERDICT once the run
	// has actually ended. While it is open -- the ordinary case at Stop -- a
	// hook invocation that has not yet written its own terminal, or a
	// declaration with no execution yet, is what an in-flight call looks
	// like on disk, and flipping the state on that would be run_not_closed
	// wearing another name. The lists themselves stay visible regardless --
	// see Declarations' doc -- only the coverage state is gated.
	if global.EndRecorded {
		if len(turnUnterminated(w, run.Terminals)) > 0 {
			tc.add(store.ReasonUnterminatedEntry)
		}
		if len(turnDropped(run, w)) > 0 {
			tc.add(store.ReasonLockTimeout)
		}
	}

	tc.State = store.StateVerified
	if len(tc.Reasons) > 0 {
		tc.State = store.StateUnverified
	}
	return tc, global
}

// coverageInWindow filters coverage records to those recorded inside w, by
// RECORDED TIME -- see turnWindow's doc for why that is the only join a
// coverage record supports.
func coverageInWindow(covs []store.Coverage, w turnWindow) []store.Coverage {
	out := make([]store.Coverage, 0, len(covs))
	for _, c := range covs {
		if w.contains(c.RecordedAtMS) {
			out = append(out, c)
		}
	}
	return out
}

// gapIntersectsWindow tests a gap's [FromUnixMS, ToUnixMS] against a turn's
// [startMS, endMS). w.known is assumed true; callers gate on it separately so
// the "no window, any gap counts" fallback stays visible at the call site.
func gapIntersectsWindow(g store.Gap, w turnWindow) bool {
	return g.FromUnixMS < w.endMS && g.ToUnixMS > w.startMS
}

// gapsInWindow is the gap records rendered on the digest itself: the ones
// that intersect the turn, under the same fallback as buildTurnCoverage --
// every session gap, when the window is not known.
func gapsInWindow(gaps []store.Gap, w turnWindow) []store.Gap {
	var out []store.Gap
	for _, g := range gaps {
		if !w.known || gapIntersectsWindow(g, w) {
			out = append(out, g)
		}
	}
	return out
}

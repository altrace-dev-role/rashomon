package budget

import "time"

// DeadlineEntry is the closed set of CLI entries the deadline table of CLAUDE.md
// 8.2.5 (archive 85.3.10) is keyed by: `hook`, `post`, the future `event`
// command's two rows, and the three recap/probe entries that already run
// today (`recap pending`, `recap` Stop, `probe start`, `probe end`). Each
// has one required phase that never waits on a soft deadline, and a soft
// deadline its optional phases check between steps.
type DeadlineEntry uint8

const (
	EntryHook DeadlineEntry = iota
	EntryPost
	// EntryEventPostToolBatch and EntryEventOther are the two rows the
	// unified `event` command (P-16) will dispatch between on
	// hook_event_name; neither has a production caller yet, but the table
	// names both and a Clock built for either must use the right row.
	EntryEventPostToolBatch
	EntryEventOther
	EntryRecapPending
	EntryRecapStop
	EntryProbeStart
	EntryProbeEnd
	deadlineEntryCount
)

// softBudgets is the table's "soft deadline for optional phases" column, in
// the table's own order. A row here with no matching row in CLAUDE.md
// 8.2.5 is a bug; TestRES26_SoftBudgetMatchesTheTable pins every value.
var softBudgets = [deadlineEntryCount]time.Duration{
	EntryHook:               50 * time.Millisecond,
	EntryPost:               150 * time.Millisecond,
	EntryEventPostToolBatch: 100 * time.Millisecond,
	EntryEventOther:         50 * time.Millisecond,
	EntryRecapPending:       200 * time.Millisecond,
	EntryRecapStop:          2 * time.Second,
	EntryProbeStart:         1200 * time.Millisecond,
	EntryProbeEnd:           1000 * time.Millisecond,
}

// SoftBudget is e's soft deadline for optional phases: NewClock(start,
// e.SoftBudget()) is the Clock every phase of e checks. An DeadlineEntry past the
// end has no budget, zero, which Expired reports as already past.
func (e DeadlineEntry) SoftBudget() time.Duration {
	if e >= deadlineEntryCount {
		return 0
	}
	return softBudgets[e]
}

// Entries lists every entry, in the table's order.
func Entries() []DeadlineEntry {
	out := make([]DeadlineEntry, 0, deadlineEntryCount)
	for e := DeadlineEntry(0); e < deadlineEntryCount; e++ {
		out = append(out, e)
	}
	return out
}

// probeStartFixed is the four ordered phases' own slice of `probe start`'s
// 1.2s total before CostScan takes what is left (CLAUDE.md 8.2.5,
// 85.3.10): labels, mining, eviction, compaction, briefing, in that order.
var probeStartFixed = map[Phase]time.Duration{
	Labels:   50 * time.Millisecond,
	Mining:   300 * time.Millisecond,
	Evict:    500 * time.Millisecond,
	Compact:  200 * time.Millisecond,
	Briefing: 150 * time.Millisecond,
}

// ProbeStartOrder is the five fixed-budget phases `probe start` runs, in
// order, before CostScan's leftover phase.
func ProbeStartOrder() []Phase {
	return []Phase{Labels, Mining, Evict, Compact, Briefing}
}

// ProbeStartFixedBudget is p's own slice of `probe start`'s total, zero for
// a phase outside the five ProbeStartOrder lists (including CostScan,
// which gets ProbeStartCostScanBudget instead).
func ProbeStartFixedBudget(p Phase) time.Duration {
	return probeStartFixed[p]
}

// probeStartCostScanCap is the most CostScan ever gets inside `probe
// start`, however little the five phases ahead of it spent.
const probeStartCostScanCap = 50 * time.Millisecond

// ProbeStartCostScanBudget is what `probe start`'s cost-finalisation phase
// gets after the five fixed phases ahead of it have spent spent of the
// entry's 1.2s total: whatever is left, capped at 50ms, never negative
// (CLAUDE.md 8.2.5: "then cost finalization of ended runs ... in whatever
// is left, at most 50 ms").
func ProbeStartCostScanBudget(spent time.Duration) time.Duration {
	left := EntryProbeStart.SoftBudget() - spent
	if left > probeStartCostScanCap {
		return probeStartCostScanCap
	}
	if left < 0 {
		return 0
	}
	return left
}

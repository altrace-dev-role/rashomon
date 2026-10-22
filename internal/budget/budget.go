// Package budget holds the deadline contract every hook phase answers to.
//
// A hook is a fresh process with a few milliseconds to live, and the optional
// phases inside it (VERIFY's evaluation, STEER's render, LEARN's lookup, the
// folds, the scans) each stand down at a deadline rather than finish late:
// a note that arrives after Claude Code has moved on is worse than no note.
// This package names the phases and the reason each writes when it stands
// down, so that every stand-down is a closed word a reader can count, never a
// free-text explanation. Technical spec 8.2.2 and RES-26; Archive 85.3.8 to
// 85.3.10.
//
// Contract only (P-07): the types and the closed lists. The deadlines
// themselves are wired by the PRs that own each phase.
package budget

import (
	"errors"
	"time"
)

// Clock is one hook's soft deadline, started when the process did.
//
// soft is the point past which an optional phase stands down; it is shorter
// than Claude Code's own hook timeout by the margin the record append needs,
// so the record is never what a late phase costs.
type Clock struct {
	start time.Time
	soft  time.Duration
}

// errZeroStart and errNonPositiveBudget are refused by NewClock: a wiring
// bug that leaves the start unset, or passes a budget of zero or less,
// must fail where it is made rather than read as every phase timing out
// (RES-26).
var (
	errZeroStart         = errors.New("budget: clock start is the zero time")
	errNonPositiveBudget = errors.New("budget: soft budget is zero or negative")
)

// NewClock starts the deadline at start with the given soft budget. It
// refuses a zero start or a non-positive budget, since either reads every
// subsequent Expired call as true -- a wiring bug surfacing as a
// stand-down.
func NewClock(start time.Time, soft time.Duration) (Clock, error) {
	if start.IsZero() {
		return Clock{}, errZeroStart
	}
	if soft <= 0 {
		return Clock{}, errNonPositiveBudget
	}
	return Clock{start: start, soft: soft}, nil
}

// Expired reports whether the soft deadline has passed at now.
func (c Clock) Expired(now time.Time) bool { return !now.Before(c.start.Add(c.soft)) }

// Left is the time until the soft deadline at now, zero once it has passed.
func (c Clock) Left(now time.Time) time.Duration {
	left := c.start.Add(c.soft).Sub(now)
	if left < 0 {
		return 0
	}
	return left
}

// Reason is the frozen stood-down reason a phase reports when its budget
// expires: the string Phase.Reason returns, recorded in turn_end.stood_down
// and read by MEASURE (8.2.2).
type Reason string

// Phase is an optional phase of a hook, each with its own stand-down reason.
// The order is the spec's (8.2.2) and is not meaningful beyond that. The zero
// Phase is none of them: it has no reason, which no writer may record, and
// Phases does not list it, so a phase never set cannot stand down as Verify.
type Phase uint8

const (
	phaseUnset Phase = iota
	Verify
	Steer
	LearnLookup
	Labels
	Mining
	Evict
	Compact
	CostScan
	Fold
	OutputScan
	Spec
	Briefing
	Join
	phaseCount
)

// reasons are the closed words a stand-down writes, one per Phase, in the
// spec's order. A reason is a coverage reason or a suppressed_by value
// elsewhere; the word is the contract, which is why it is a table and not a
// String() that someone could reword.
var reasons = [phaseCount]Reason{
	Verify:      "verify_budget",
	Steer:       "deadline",
	LearnLookup: "learn_budget",
	Labels:      "label_budget",
	Mining:      "mining_budget",
	Evict:       "evict_budget",
	Compact:     "compact_budget",
	CostScan:    "cost_scan_budget",
	Fold:        "digest_budget",
	OutputScan:  "output_scan_budget",
	Spec:        "spec_budget",
	Briefing:    "briefing_budget",
	Join:        "join_budget",
}

// Reason is the closed word the phase writes when it stands down at the
// deadline. An unknown phase has no word: the empty string, which no writer
// may record, so a Phase added without a reason fails the test below rather
// than inventing one.
func (p Phase) Reason() Reason {
	if p >= phaseCount {
		return ""
	}
	return reasons[p]
}

// Phases lists every phase, in the spec's order.
func Phases() []Phase {
	out := make([]Phase, 0, phaseCount-1)
	for p := phaseUnset + 1; p < phaseCount; p++ {
		out = append(out, p)
	}
	return out
}

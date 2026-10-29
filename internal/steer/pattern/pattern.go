// Package pattern is the frozen table of STEER pattern identifiers: which
// ones exist, what sub-kinds live under each, and the priority each takes
// when steer.Route orders competing findings. The technical spec's "The
// frozen `steer.PatternID` set" paragraph (Contracts and names, decided
// 2026-10-13) and 4.2.4 (the step 8 priority table).
//
// Contract only (P-07): the table and the lookups over it. Nothing here
// decides whether a pattern fires -- that is each pattern's own PR, landing
// against the Finding and Message contracts steer and steer/render freeze.
package pattern

import (
	"slices"
	"sync"
)

// ID names one of the sixteen frozen STEER patterns. The set is closed: the
// review that gates this package checks the constants below against the
// spec's list verbatim, so adding or renaming one is a contract change, not
// a local edit.
type ID string

// The frozen pattern IDs, in the order the frozen-set paragraph lists them
// -- the same order All returns. Kinds live under their id, never as ids
// (see the kinds table below); sequence-risk is flag-only in Wave B and its
// id is reserved now.
const (
	SubtaskGate        ID = "subtask-gate"
	RegressionLocation ID = "regression-location"
	ContinuityBriefing ID = "continuity-briefing"
	DeclaredFact       ID = "declared-fact"
	LearnedFact        ID = "learned-fact"
	PlatformRule       ID = "platform-rule"
	Integrity          ID = "integrity"
	IdenticalRetry     ID = "identical-retry"
	CredSpray          ID = "cred-spray"
	OrderingRule       ID = "ordering-rule"
	AgentInterference  ID = "agent-interference"
	SubagentClaim      ID = "subagent-claim"
	ModelSwitch        ID = "model-switch"
	// ClaimVsRecord's masked kind is never model-visible (the frozen-set
	// paragraph). This package's table cannot express that; steer.Finding.Check
	// enforces it, through userOnly, at the finding's Check rather than here.
	ClaimVsRecord ID = "claim-vs-record"
	MaskedStatus  ID = "masked-status"
	SequenceRisk  ID = "sequence-risk"
)

// All returns the frozen pattern IDs in the spec's order. Callers that need a stable iteration order -- rendering a legend,
// building an exhaustiveness test -- should use this rather than range over
// a map, which Go does not guarantee an order for.
func All() []ID {
	return []ID{
		SubtaskGate, RegressionLocation, ContinuityBriefing, DeclaredFact,
		LearnedFact, PlatformRule, Integrity, IdenticalRetry, CredSpray,
		OrderingRule, AgentInterference, SubagentClaim, ModelSwitch,
		ClaimVsRecord, MaskedStatus, SequenceRisk,
	}
}

// kinds holds the sub-kind vocabulary of the six ids the frozen-set
// paragraph gives a closed list ("Kinds live under their id, never as
// ids"). An id absent from this map has no sub-kind taxonomy: Kinds returns
// nil for it, and a Finding of that pattern leaves Kind empty. declared-fact
// is deliberately absent although the paragraph names it: its kind is the
// open `declared-fact/<class>`, not a closed list, so Kinds returns nil
// rather than a placeholder entry that would look like a closed set of one.
func kinds() map[ID][]string {
	kindsOnce.Do(func() {
		kindsOnce.v = map[ID][]string{
			ClaimVsRecord: {
				"red", "masked", "stale", "absent", "backgrounded",
				"silent-failure", "done-unverified",
			},
			PlatformRule: {"edit", "command", "pragma"},
			Integrity: {
				"skip-added", "test-deleted", "assert-drop", "frozen-path",
				"tests-only-green", "spec-edited", "steer-config", "store-touched",
				"suppression-added", "generated-edited", "gate-weakened",
				"gate-config-edited",
			},
			SubtaskGate:  {"commit", "push", "pr"},
			LearnedFact:  {"local", "pack"},
			SequenceRisk: {"secret-then-egress", "lockfile-unverified", "dep-unresolved"},
		}
	})
	return kindsOnce.v
}

// kindsOnce holds what kinds returns, built on its first call and never at
// init (RES-2).
var kindsOnce struct {
	sync.Once
	v map[ID][]string
}

// Kinds returns a copy of the closed sub-kind vocabulary under id, or nil
// if id has none. declared-fact takes an open "<class>" kind rather than a
// closed list -- callers must not read a nil result for it as "no kind is
// ever valid for this pattern"; it means the class name is caller-defined
// and this package does not enumerate it. Every other id with no entry
// (regression-location, continuity-briefing, identical-retry, cred-spray,
// ordering-rule, agent-interference, subagent-claim, model-switch,
// masked-status) has no sub-kind taxonomy: the frozen-set paragraph gives
// kinds to exactly seven ids, and silence for the rest is read here as
// "Kind is empty on a Finding of this pattern."
func Kinds(id ID) []string {
	return slices.Clone(kinds()[id])
}

// KindValid reports whether kind may stand under id on a Finding: one of the
// closed list for an id that has one, a non-empty class for declared-fact
// (the open `declared-fact/<class>`), and empty for every other id. The bare
// subtask-gate, a task completion (tech 2.2.9's row "subtask-gate,
// /commit, /push, /pr"), is the one id with a closed list whose kind may
// also be empty. Only frozen kinds cross into STEER (tech, Contracts and
// names: the verify-to-steer gap row).
func KindValid(id ID, kind string) bool {
	switch {
	case !Valid(id):
		return false
	case id == DeclaredFact:
		return classForm(kind)
	case kind == "":
		return kinds()[id] == nil || id == SubtaskGate
	}
	return slices.Contains(kinds()[id], kind)
}

// classForm reports a declared-fact class: ^[a-z0-9][a-z0-9._-]{0,63}$.
func classForm(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		lowerAlnum := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if !lowerAlnum && (i == 0 || c != '.' && c != '_' && c != '-') {
			return false
		}
	}
	return true
}

// Valid reports whether id is one of the sixteen frozen patterns.
func Valid(id ID) bool {
	for _, known := range All() {
		if known == id {
			return true
		}
	}
	return false
}

// priority is the step 8 ordering table (tech 4.2.4): ties break
// by lower after_seq, then steer id, at steer.Route -- this table only
// carries the base number. Every frozen id has one except the two that
// travel on their own channel (continuity-briefing and model-switch inject
// directly, never as competing candidates), because the number also orders
// flags for the one systemMessage per call: integrity ranks first (95, above
// subtask-gate: the evidence other notes rely on may be compromised) although
// it is flag-capped in Wave A, and sequence-risk carries 62 while it is
// flag-only in Wave B, so on an egress call its secret-then-egress flag beats
// identical-retry (40). TestSTR16_EveryCandidatePatternHasAPriority pins the
// split so a change to it is a reviewed decision, not a drift.
func priority() map[ID]int {
	priorityOnce.Do(func() {
		priorityOnce.v = map[ID]int{
			Integrity:          95,
			SubtaskGate:        90,
			ClaimVsRecord:      85,
			RegressionLocation: 80,
			DeclaredFact:       75,
			LearnedFact:        70,
			CredSpray:          65,
			SequenceRisk:       62,
			PlatformRule:       60,
			AgentInterference:  55,
			SubagentClaim:      50,
			MaskedStatus:       45,
			IdenticalRetry:     40,
			OrderingRule:       30,
		}
	})
	return priorityOnce.v
}

// priorityOnce holds what priority returns, built on its first call and never
// at init (RES-2).
var priorityOnce struct {
	sync.Once
	v map[ID]int
}

// Priority returns id's step 8 ordering value and true, or (0, false) for
// the two own-channel ids that never compete at steer.Route (tech 4.2.4).
func Priority(id ID) (int, bool) {
	p, ok := priority()[id]
	return p, ok
}

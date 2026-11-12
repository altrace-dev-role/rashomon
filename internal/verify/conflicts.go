package verify

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// Conflicts as unit-scoped obligations (W-M2.4, verify half). A conflict is
// a mismatch the graph engine raises about one unit: a boundary crossed, a
// decision contradicted, a signature changed under its callers, a protected
// path edited. This package never sees the engine's type (it imports nothing
// of internal/graph): the Stop's caller converts each conflict it holds into
// a ConflictSeen and sets StopInput.Conflicts, as it sets Unbound. With none
// set, nothing here runs and the Stop is unchanged.

// ConflictRow is one conflict as the ledger and the Stop line read it. Unit
// is the global unit id, the row's identity and Obligation.Unit; UnitPath is
// the unit's path as the brief prints it, filled by the caller from the unit
// node, for the Stop's line only and never identity (a path may hold ':', so
// no reader may parse Unit for it). RuleID is the rule that raised the
// conflict, "" for a kind no rule raises; Subject is what it is about.
type ConflictRow struct{ Kind, Unit, UnitPath, RuleID, Subject string }

// ConflictRowKinds are the conflict kinds that owe a ledger row and a Stop
// line, in the order rows and lines sort by. They are the engine's kind
// strings (cmd/rashomon pins that every engine kind is placed).
var ConflictRowKinds = []string{"boundary_cross", "decision_contradicted", "signature_mismatch", "protected_path"}

// WithheldConflictKinds are the kinds whose detector stays but whose finding
// is withheld in M2: no row, no line, no count on any surface (ruling
// 2026-11-11 16:23: no k in 2..6 reached zero wrong flags in the replay).
var WithheldConflictKinds = []string{"duplicate_abstraction"}

// ConflictSeen is a conflict the session was shown: Seqs are the PostToolUse
// seqs whose nudge named the row, ascending; ResolvedSeq is the seq of the
// edit after which the tree no longer showed it, 0 while it is present at the
// Stop. A Kind outside ConflictRowKinds (withheld, arity, unknown) yields
// nothing.
type ConflictSeen struct {
	Row         ConflictRow
	Seqs        []int64
	ResolvedSeq int64
}

// conflictGroup is the members of one (Kind, RuleID, Unit) group, in input
// order, and what the ledger and the view read of them.
type conflictGroup struct {
	row      ConflictRow // the first unresolved member's row, else the first member's
	seqs     []int64     // the union of the members' Seqs, ascending, deduped
	resolved int64       // the largest ResolvedSeq when every member is resolved, else 0
}

// unmet reports whether any member is still present at the Stop.
func (g conflictGroup) unmet() bool { return g.resolved == 0 }

// conflictGroups drops what owes no row (a Kind outside ConflictRowKinds, an
// empty Unit, a '\n' in Kind, RuleID or Unit), groups the rest by (Kind,
// RuleID, Unit), one group per key: several crosses of one rule in one unit
// are one row, and Kind is in the key because RuleID "" is shared by every
// rule-less kind. The groups come in ConflictRowKinds order, then RuleID,
// then Unit, whatever the input order. Cost: O(n log n).
func conflictGroups(in []ConflictSeen) []conflictGroup {
	type key struct{ kind, rule, unit string }
	type acc struct {
		g      conflictGroup
		open   bool
		maxRes int64
		k      key
	}
	var accs []*acc
	byKey := map[key]*acc{}
	for _, c := range in {
		r := c.Row
		if !slices.Contains(ConflictRowKinds, r.Kind) || r.Unit == "" ||
			strings.ContainsRune(r.Kind, '\n') || strings.ContainsRune(r.RuleID, '\n') || strings.ContainsRune(r.Unit, '\n') {
			continue
		}
		k := key{r.Kind, r.RuleID, r.Unit}
		a := byKey[k]
		if a == nil {
			a = &acc{k: k}
			a.g.row = r
			byKey[k] = a
			accs = append(accs, a)
		}
		a.g.seqs = append(a.g.seqs, c.Seqs...)
		if c.ResolvedSeq <= 0 {
			if !a.open {
				a.g.row = r
			}
			a.open = true
		} else {
			a.maxRes = max(a.maxRes, c.ResolvedSeq)
		}
	}
	slices.SortFunc(accs, func(x, y *acc) int {
		return cmp.Or(cmp.Compare(slices.Index(ConflictRowKinds, x.k.kind), slices.Index(ConflictRowKinds, y.k.kind)),
			cmp.Compare(x.k.rule, y.k.rule), cmp.Compare(x.k.unit, y.k.unit))
	})
	out := make([]conflictGroup, 0, len(accs))
	for _, a := range accs {
		slices.Sort(a.g.seqs)
		a.g.seqs = slices.Compact(a.g.seqs)
		if !a.open {
			a.g.resolved = a.maxRes
		}
		out = append(out, a.g)
	}
	return out
}

// obligation is the group's ledger row in scope: kind spec_rule, sourced by
// the rule id (else "conflict:"+Kind, so the ledger printer's single-space
// columns never print an empty one), unit the group's unit. Unmet cites the
// seqs that named it; met cites the edit that removed the last member.
func (g conflictGroup) obligation(scope string) Obligation {
	r := g.row
	o := Obligation{ID: ObligationID(scope, SpecRule, conflictFamily(r)), Scope: scope, Kind: SpecRule,
		Source: cmp.Or(r.RuleID, "conflict:"+r.Kind), State: ObligationUnmet, Unit: r.Unit}
	if g.unmet() {
		o.Evidence = append([]int64{}, g.seqs...)
		return o
	}
	o.State, o.Evidence = ObligationMet, []int64{g.resolved}
	return o
}

// conflictFamily is the family a conflict row's id digests:
// "c" + len(Kind) + ":" + Kind + len(RuleID) + ":" + RuleID + Unit.
// Injective for any RuleID charset (depguard rule names are arbitrary YAML
// keys): Kind and RuleID are length-prefixed, so the string reads back one
// way, Kind then RuleID then Unit as the rest. It never equals a verdict
// row's family: the only other spec_rule rows are specObligations', whose
// family is string(v.Rule) = "spec:" + a spec id, so it starts 's', and a
// conflict family always starts 'c'. No '\n' can enter (conflictGroups drops
// one), so ObligationID's separator stays unambiguous.
func conflictFamily(r ConflictRow) string {
	return "c" + strconv.Itoa(len(r.Kind)) + ":" + r.Kind + strconv.Itoa(len(r.RuleID)) + ":" + r.RuleID + r.Unit
}

// conflictObligations is the ledger rows of in in scope: one per group, in
// conflictGroups' order. nil for none.
func conflictObligations(scope string, in []ConflictSeen) []Obligation {
	var out []Obligation
	for _, g := range conflictGroups(in) {
		out = append(out, g.obligation(scope))
	}
	return out
}

// unmetConflictRows is the view's rows: each unmet group's row, in the
// ledger's order, paired one to one with the unmet conflict rows of the
// ledger built from the same groups. nil for none.
func unmetConflictRows(groups []conflictGroup) []ConflictRow {
	var out []ConflictRow
	for _, g := range groups {
		if g.unmet() {
			out = append(out, g.row)
		}
	}
	return out
}

// pointScope is the scope the ledger at snap's point owes in.
func pointScope(snap Snapshot) string {
	return cmp.Or(obligationScopes()[snap.Point.Kind], ScopeSession)
}

package main

import (
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// planPort is the post's plan-check port (W-M2.6 hook half): PlanFacts over
// the index under the call's cwd, which the hook package does not link. Only
// the facts that name a path a rule covers reach the hook: a boundary
// caution names units, and a decision about a symbol or a unit names no path.
func planPort() hook.PlanPort {
	return hook.PlanPort{
		Check: func(plan []byte, cwd string, sp *spec.Compiled) ([]hook.PlanItem, bool) {
			facts, ok := PlanFacts(plan, cwd, sp)
			if !ok {
				return nil, false
			}
			var items []hook.PlanItem
			for _, f := range facts {
				if f.Kind == PlanKindBoundary || f.Path == "" {
					continue
				}
				items = append(items, hook.PlanItem{Path: f.Path, RuleID: f.RuleID, Kind: f.Kind, Trust: f.Trust, Planned: f.Planned})
			}
			return items, len(items) > 0
		},
	}
}

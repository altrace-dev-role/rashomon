package steer

import "sync/atomic"

// Context is Route's second argument (tech 4.2.4). P-07 does not freeze
// it: the technical spec's gaps table (Contracts and names) closes
// `steer.Context` "before P-30 opens (D6)". This is an empty placeholder so
// Route's signature compiles; whoever lands P-30 adds its real fields.
type Context struct{}

// Route is the only path to Output (tech 4.2.4; tech 1141: "the only path
// to output"): it is where the step 8 priorities, tie-breaking by lower
// AfterSeq then steer id, dedup by Key, and every budget/suppression
// decision happen.
//
// Every finding passes Finding.Check before the router sees it: Route is
// admit then route, and TestDEL22f pins that body, so P-30 fills in route
// and cannot drop the check by forgetting it (P07N-14).
func Route(ctx *Context, findings []Finding) Output {
	return route(ctx, admit(findings))
}

// dropped counts findings admit has refused since process start. Route's
// body is pinned to admit then route with nothing else in it
// (TestDEL22f), so a dropped finding is counted here rather than by
// changing what Route returns.
var dropped atomic.Int64

// DroppedFindings is the number of findings admit has refused since
// process start: a producer sending a finding outside the contract. P-30
// wires this into the router's own telemetry.
func DroppedFindings() int64 { return dropped.Load() }

// admit keeps the findings that pass Check, in order, and drops the rest:
// a finding outside the contract says nothing rather than reach a hook's
// stdout (fail closed). Logging a dropped finding is P-30's, with the
// router's own telemetry; DroppedFindings is the count it reads from.
func admit(findings []Finding) []Finding {
	kept := make([]Finding, 0, len(findings))
	for _, f := range findings {
		if f.Check() == nil {
			kept = append(kept, f)
		} else {
			dropped.Add(1)
		}
	}
	return kept
}

// route is the router proper, P-30's. Contract only (P-07): this stub
// returns the empty Output and decides nothing, because doing any of that
// needs the record and the rules a later wave reads. Every candidate
// pattern's priority (TestSTR16_EveryCandidatePatternHasAPriority, in
// internal/steer/pattern) already exists for it to use once it is real.
func route(ctx *Context, findings []Finding) Output {
	return Output{}
}

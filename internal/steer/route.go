package steer

// Context is Route's second argument (tech 4.2.4). P-07 does not freeze
// it: the technical spec's gaps table (Contracts and names) closes
// `steer.Context` "before P-30 opens (D6)". This is an empty placeholder so
// Route's signature compiles; whoever lands P-30 adds its real fields.
type Context struct{}

// Route is the only path to Output (tech 4.2.4): it is where the step 8
// priorities, tie-breaking by lower AfterSeq then steer id, dedup by Key,
// and every budget/suppression decision happen.
//
// Contract only (P-07): this stub returns the empty Output and decides
// nothing, because doing any of that needs the record and the rules a
// later wave reads -- P-30 fills it in. Every candidate pattern's
// priority (TestSTR16_EveryCandidatePatternHasAPriority, in
// internal/steer/pattern) already exists for this function to use once
// it is real.
func Route(ctx *Context, findings []Finding) Output {
	return Output{}
}

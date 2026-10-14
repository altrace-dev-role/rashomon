package steer

// Pattern is what each pattern's PR implements: P-07's proposal for the gap
// row "The pattern interface, including `Revalidate` (4.2.6 step 2), and
// which evaluates a label" (tech, Contracts and names). K signs, L reviews
// or amends in P-07's review; nothing downstream builds on it until then.
//
// The label half is answered elsewhere: MEASURE's `label_rule` table (tech
// 5.2.4 B) evaluates every label (MSR 13, decided (a) 2026-10-13), and a
// steer line names its rule in Line.LabelRule, so a pattern has no Label
// method. Firing is each pattern's own code in its PR, producing Findings;
// rendering is render.New over the pattern's slot-binding struct (tech
// 4.2.7). This interface carries only what the router needs from a pattern
// at delivery time.
type Pattern interface {
	// ID is the pattern's frozen id (pattern.All; the frozen-set
	// paragraph of Contracts and names).
	ID() PatternID
	// Revalidate reports whether a candidate's premise still holds at
	// delivery: PostToolBatch reads the open candidates under the lock and
	// re-validates each, and a candidate whose premise no longer holds is
	// marked `stale` (tech 4.2.6 step 2; archive 40.3.7: "each pattern's
	// `Revalidate`"). The deferred claim-vs-record note is re-validated
	// the same way at the next UserPromptSubmit (tech 4.2.6 step 5). It
	// reads ctx, what the record shows now, and the candidate's facts,
	// never free text; it writes nothing; and it returns false on any
	// doubt, because a stale note dropped is the safe direction.
	// steer.Context is closed before P-30 opens (D6), so this signature
	// names the placeholder type.
	Revalidate(ctx *Context, f Finding) bool
}

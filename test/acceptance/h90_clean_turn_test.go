package acceptance

import "testing"

// H-90 -- a clean turn prints nothing.
//
// A line that is identical on almost every turn is trained out within a
// week, and a notification nobody reads is the third failure again (spec,
// Part 4). So a turn with none of the five triggers -- coverage verified, no
// recorded failure, no declaration without execution, no truncation, a
// known (non-Unknown) digest -- must produce nothing at all on stdout: not
// an empty systemMessage, not a friendly "all clear", nothing.
//
// Break: print a terse line anyway and the surface is ignorable within a
// week, by the argument this item rests on.
func TestH90_ACleanTurnPrintsNothing(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))

	line, ok := e.recapLine(stopPayload(testSession, "Ran git status as requested.", false))
	if ok {
		t.Errorf("recap printed a line on a clean turn: %q", line)
	}
}

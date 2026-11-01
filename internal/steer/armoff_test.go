package steer

import (
	"testing"
)

// TestMS1o_ArmOffDecidesAsSteer: arm off runs the steer arm's decision and
// withholds only its effect. Over one prompt, five calls under each arm on
// its own store: two notes on one call, then three calls of one note with a
// new key each, then a note on a key already given. Steer delivers the first
// three calls' winners and refuses the rest by budget. Off prints nothing,
// and each of its lines is steer's, but for arm_off where steer delivered:
// so the per-prompt budget counts a withheld note as given.
func TestMS1o_ArmOffDecidesAsSteer(t *testing.T) {
	calls := func() [][]Finding {
		return [][]Finding{
			{regressionNote(t, "go test", 5, "a", LevelNudge), regressionNote(t, "go vet", 6, "b", LevelNudge)},
			{regressionNote(t, "make", 7, "c", LevelNudge)},
			{regressionNote(t, "npm test", 8, "d", LevelNudge)},
			{regressionNote(t, "cargo test", 9, "e", LevelNudge)},
			{regressionNote(t, "go test", 10, "a", LevelNudge)},
		}
	}
	run := func(arm Arm) ([]Output, []Line) {
		ctx := routerContext(t)
		ctx.Arm = arm
		var outs []Output
		for _, fs := range calls() {
			outs = append(outs, Route(ctx, fs))
			ctx.NowUnixMS += 1000
		}
		return outs, steerLines(t, ctx)
	}
	steerOut, steerLog := run(ArmSteer)
	offOut, offLog := run(ArmOff)

	wantSteer := []struct {
		state LineState
		by    SuppressedBy
	}{
		{LineStateDelivered, ""}, {LineStateSuperseded, SuppressedByBudgetBatch},
		{LineStateDelivered, ""}, {LineStateDelivered, ""},
		{LineStateSuppressed, SuppressedByBudgetPrompt}, {LineStateSuppressed, SuppressedByBudgetPattern},
	}
	if len(steerLog) != len(wantSteer) || len(offLog) != len(wantSteer) {
		t.Fatalf("steer logged %d lines and off %d, want %d each", len(steerLog), len(offLog), len(wantSteer))
	}
	for i, w := range wantSteer {
		if by := byOf(steerLog[i]); steerLog[i].State != w.state || by != w.by {
			t.Errorf("premise: steer line %d (%s) = %q %q, want %q %q", i, steerLog[i].Key, steerLog[i].State, by, w.state, w.by)
		}
	}
	for i, o := range offOut {
		if o != (Output{}) {
			t.Errorf("off call %d printed %+v", i, o)
		}
		if steerOut[i] == (Output{}) && i < 3 {
			t.Errorf("premise: steer call %d printed nothing", i)
		}
	}
	withheld := 0
	for i := range offLog {
		s, o := steerLog[i], offLog[i]
		if s.State == LineStateDelivered {
			withheld++
			wantLine(t, o, LineStateSuppressed, SuppressedByArmOff, DeliveryShadow)
			continue
		}
		if o.State != s.State || byOf(o) != byOf(s) || deliveryOf(o) != deliveryOf(s) || o.Key != s.Key {
			t.Errorf("off line %d = %s %q %q %q, want steer's %s %q %q %q", i,
				o.Key, o.State, byOf(o), deliveryOf(o), s.Key, s.State, byOf(s), deliveryOf(s))
		}
	}
	if withheld != maxNotes {
		t.Errorf("off withheld %d notes, want the prompt's budget, %d", withheld, maxNotes)
	}
}

func byOf(l Line) SuppressedBy {
	if l.SuppressedBy == nil {
		return ""
	}
	return *l.SuppressedBy
}

func deliveryOf(l Line) Delivery {
	if l.Delivery == nil {
		return ""
	}
	return *l.Delivery
}

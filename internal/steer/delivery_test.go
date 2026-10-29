package steer

import (
	"slices"
	"testing"
)

// TestDeriveDelivery_EveryStateAndReason walks every (state, reason) pair,
// with no reason too, at every level and both values of printed, against
// tech 4.2.3's derivation written out here by hand: delivered gives
// delivered at nudge or stop and flag_only at flag; suppressed gives
// suppressed_budget for budget_prompt, budget_pattern or budget_batch (a
// list, as production keeps one, not a prefix match) and shadow for every
// other; superseded gives suppressed_budget; stale gives shadow; expired
// gives flag_only when it printed as a turn-end flag, else shadow;
// candidate, deferred and unconfirmed carry null (card 2.2 R11).
func TestDeriveDelivery_EveryStateAndReason(t *testing.T) {
	reasons := []*SuppressedBy{nil}
	for _, r := range AllSuppressedBy() {
		reasons = append(reasons, &r)
	}
	for _, state := range AllLineStates() {
		for _, by := range reasons {
			for _, level := range []Level{LevelFlag, LevelNudge, LevelStop} {
				for _, printed := range []bool{false, true} {
					want := wantDelivery(state, level, by, printed)
					got := DeriveDelivery(state, level, by, printed)
					if (got == nil) != (want == "") || got != nil && *got != want {
						t.Errorf("DeriveDelivery(%s, %s, %v, %v) = %v, want %q", state, level, reasonText(by), printed, deref(got), want)
					}
				}
			}
		}
	}
	if got := DeriveDelivery("not-a-state", LevelNudge, nil, true); got != nil {
		t.Errorf("an unknown state derives %q, want null", *got)
	}
	if got := DeriveDelivery(LineStateDelivered, LevelOff, nil, true); got != nil {
		t.Errorf("a delivered line at off derives %q, want null: off writes no line", *got)
	}
}

func wantDelivery(state LineState, level Level, by *SuppressedBy, printed bool) Delivery {
	switch state {
	case LineStateDelivered:
		if level == LevelFlag {
			return DeliveryFlagOnly
		}
		return DeliveryDelivered
	case LineStateSuppressed:
		if by != nil && slices.Contains([]SuppressedBy{SuppressedByBudgetPrompt, SuppressedByBudgetPattern, SuppressedByBudgetBatch}, *by) {
			return DeliverySuppressedBudget
		}
		return DeliveryShadow
	case LineStateSuperseded:
		return DeliverySuppressedBudget
	case LineStateStale:
		return DeliveryShadow
	case LineStateExpired:
		if printed {
			return DeliveryFlagOnly
		}
		return DeliveryShadow
	}
	return ""
}

func reasonText(by *SuppressedBy) string {
	if by == nil {
		return "null"
	}
	return string(*by)
}

func deref(d *Delivery) string {
	if d == nil {
		return "null"
	}
	return string(*d)
}

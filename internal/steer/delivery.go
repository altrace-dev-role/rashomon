package steer

// DeriveDelivery is a line's delivery, derived from its state, its level
// and its suppression reason (tech 4.2.3; the derivation steer.go's Delivery
// comment leaves to the router): delivered gives delivered at nudge or stop
// and flag_only at flag, demoted included; suppressed gives suppressed_budget
// for a budget_* reason and shadow for every other; superseded gives
// suppressed_budget, the one-per-batch cap having stopped it; stale gives
// shadow; expired gives flag_only when it printed as a turn-end flag and
// shadow when its session ended first, which only the turn-end line knows,
// so printed says it; candidate, deferred and unconfirmed carry null until
// they reach one of those states. A state outside the eight, or a delivered
// line at off, which writes no line, is null too.
//
// The field holdout's holdout is recorded directly by the draw (5.2.4 E),
// never derived: lineOf sets it on a held-out session's note (card 2.5).
func DeriveDelivery(state LineState, level Level, by *SuppressedBy, printed bool) *Delivery {
	var d Delivery
	switch state {
	case LineStateDelivered:
		switch level {
		case LevelFlag:
			d = DeliveryFlagOnly
		case LevelNudge, LevelStop:
			d = DeliveryDelivered
		default:
			return nil
		}
	case LineStateSuppressed:
		d = DeliveryShadow
		if by != nil && budgetReason(*by) {
			d = DeliverySuppressedBudget
		}
	case LineStateSuperseded:
		d = DeliverySuppressedBudget
	case LineStateStale:
		d = DeliveryShadow
	case LineStateExpired:
		d = DeliveryShadow
		if printed {
			d = DeliveryFlagOnly
		}
	default:
		return nil
	}
	return &d
}

// budgetReason reports the budget_* reasons, listed rather than matched by
// prefix, so a reason added to the vocabulary is not counted as a budget's
// by its spelling alone.
func budgetReason(by SuppressedBy) bool {
	switch by {
	case SuppressedByBudgetPrompt, SuppressedByBudgetPattern, SuppressedByBudgetBatch:
		return true
	}
	return false
}

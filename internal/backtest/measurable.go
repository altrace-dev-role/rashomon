package backtest

import "github.com/altrace-dev-role/rashomon/internal/steer/pattern"

// Measurable reports whether a backtest can fire id: Replay runs a transcript's
// tool calls through PreToolUse and the post hooks with no port wired, so it
// reaches only the producers SteerAt and RulesAtCall raise at a call. Its
// scratch store holds no project index and no trusted spec, so no gate is
// required there and stale-evidence, which needs one, never fires either.
// Every other id fires at an event the replay does not run, through a port it
// leaves zero, or nowhere (Reserved), so its count would be 0 by construction.
func Measurable(id pattern.ID) bool {
	switch id {
	case pattern.RegressionLocation, pattern.PlatformRule, pattern.Integrity,
		pattern.OrderingRule, pattern.MaskedStatus:
		return true
	}
	return false
}

// Reserved reports whether id is reserved and not built: no code raises it,
// live or replayed, so a backtest's refusal of it must not give the replay as
// the reason.
func Reserved(id pattern.ID) bool {
	return id == pattern.IdenticalRetry || id == pattern.CredSpray ||
		id == pattern.AgentInterference || id == pattern.ModelSwitch
}

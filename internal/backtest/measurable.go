package backtest

import "github.com/altrace-dev-role/rashomon/internal/steer/pattern"

// Measurable reports whether a backtest can fire id: Replay runs a transcript's
// tool calls through PreToolUse and the post hooks with no port wired, so it
// reaches only the producers SteerAt and RulesAtCall raise at a call. Every
// other id fires at an event the replay does not run, through a port it leaves
// zero, or nowhere, so its count would be 0 by construction.
func Measurable(id pattern.ID) bool {
	switch id {
	case pattern.RegressionLocation, pattern.PlatformRule, pattern.Integrity,
		pattern.OrderingRule, pattern.MaskedStatus, pattern.StaleEvidence:
		return true
	}
	return false
}

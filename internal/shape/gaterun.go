package shape

// GateRun reports a line that runs a gate: it names a runner (runnerDigest),
// hides a test or build runner's status (statusMasked), or is class test. It
// is the one rule, over a shape's own fields, that both readers of a gate run
// apply -- the evaluator to a recorded call as it ran (verify's isGateRun) and
// the post hook to the shape it re-derives from tool_input, to decide whether
// the call's output is read at all (card GATEOUT-WIRE1). A line it does not
// name is never scanned.
func GateRun(runnerDigest, statusMasked *string, verbClass string) bool {
	return runnerDigest != nil || statusMasked != nil && (*statusMasked == MaskedTest || *statusMasked == MaskedBuild) ||
		verbClass == VerbTest
}

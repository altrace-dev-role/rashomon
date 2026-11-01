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

// WrapperRun reports a Bash line that may run a gate no list names: a line of
// class execute (a script, a shell, ./x, a program no list knows) that is no
// gate run. Its output is read only for a runner's signature (WRAPPER-RUN1),
// by the same two readers GateRun has; a line of any other class -- cat of a
// log, a git or network command -- is never read, since an old log is not a
// run.
func WrapperRun(toolName string, runnerDigest, statusMasked *string, verbClass string) bool {
	return toolName == "Bash" && verbClass == VerbExecute && !GateRun(runnerDigest, statusMasked, verbClass)
}

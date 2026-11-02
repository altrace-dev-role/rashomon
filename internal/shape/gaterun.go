package shape

import "strings"

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

// WrapperRun reports a Bash line that may run a gate no list names: one whose
// program (the shape's, past a leading cd and assignments) is a wrapper's
// (wrapperProgram, each of class execute) and that is no gate run. Its output
// is read only for a runner's signature (WRAPPER-RUN1), by the same two
// readers GateRun has; any other line -- cat of a log, a git or network
// command, date or echo before a log read -- is never read, since an old log
// is not a run.
func WrapperRun(toolName string, program, runnerDigest, statusMasked *string, verbClass string) bool {
	return toolName == "Bash" && wrapperProgram(program) && !GateRun(runnerDigest, statusMasked, verbClass)
}

// wrapperProgram reports a program a wrapper run starts with: a shell, which
// runs a script or a line, or a script named by its extension (./test.sh,
// goslot.sh). Any other program of class execute -- date, echo, sleep, one no
// list knows -- is not: in the L1 replay of 11-01 a status check that starts
// with date and greps an old job log had go's summary lines in its output,
// and bound as a run that never happened. A script with no extension, or a
// shell behind timeout or env, is not read either: no run is guessed from it.
func wrapperProgram(program *string) bool {
	if program == nil {
		return false
	}
	switch p := *program; p {
	case "bash", "sh", "zsh", "dash", "ksh":
		return true
	default:
		return strings.HasSuffix(p, ".sh") || strings.HasSuffix(p, ".bash")
	}
}

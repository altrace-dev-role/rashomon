package recap

import (
	"github.com/altrace-dev-role/rashomon/internal/gateout"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The wrapper wording (WRAPPER-RUN1): what a line says of a gate whose last
// run a wrapper's call carried (a script, a shell, a delegate such as make
// or npm), or may have carried. A wrapped green's and a wrapped red's
// sentences stay with their rows in value.go; the unknown row's are here.

// unknownWrapped is the unknown row's sentence for a gate whose last run a
// wrapper's call bound by its runner's read (WrapRead), or one the gate may
// have run in that bound nothing of it (WrapUnread, or WrapBackground where
// that call was launched in the background); ok is false for any
// other last run, whose sentence is the row's own.
func unknownWrapped(t gateSet, g *verify.GateState) (s string, ok bool) {
	switch wrap(g) {
	case verify.WrapUnread:
		if printedStatus(g.LastRun) {
			// The call printed its runner's exit status, which the record
			// keeps as the call's outcome over any summary in its output
			// (STATUS-PROBE): its output was not read for the gate's run,
			// so the line says only the status was, never that the output
			// named no run (#234, a make that ran go test).
			return t.name(g) + " unknown: only call " + seqOf(g) + "'s exit status was read", true
		}
		return t.name(g) + " unknown: call " + seqOf(g) + "'s output names no run", true
	case verify.WrapRead:
		return t.name(g) + " ran in a wrapper (call " + seqOf(g) + "); its result is not in the record", true
	case verify.WrapBackground:
		// Launched in the background, the call's post saw no output, and the
		// record holds no read of it the hook can show: the line names the
		// call and says nothing of its output (BG-OUTCOME phase 2a, the
		// manager's ruling of 11-03: never "not read" unless shown). Nor
		// can the record show the gate ran in it, so the call may have run
		// it, WrapUnread's hedge (the ruling of 11-03 09:3x).
		return t.name(g) + " unknown: background call " + seqOf(g) + " may have run it", true
	}
	return "", false
}

// wrap is how g's last run was bound by a wrapper's call (RunRef.Wrapped),
// WrapNone with no last run.
func wrap(g *verify.GateState) verify.WrapWord {
	if g.LastRun == nil {
		return verify.WrapNone
	}
	return g.LastRun.Wrapped
}

// printedStatus reports a run whose record keeps the exit status its line
// printed (gate_output printed_pass or printed_fail, shape.PrintedStatus).
func printedStatus(r *verify.RunRef) bool {
	return r != nil && r.GateOutput != nil &&
		(r.GateOutput.Kind == gateout.KindPrintedPass || r.GateOutput.Kind == gateout.KindPrintedFail)
}

package acceptance

import (
	"syscall"
	"testing"
)

// H-84 -- an open run is not a coverage gap.
//
// Break: project session coverage and run_not_closed appears on every
// healthy turn. Extended, per review, to the two related "in flight, not
// missing" cases: a declaration with no execution recorded YET (a
// backgrounded call still running) and a declaration whose hook process was
// killed before it wrote a terminal. Neither may flip a still-open turn's
// coverage; both are exactly what an in-flight call looks like on disk until
// the run actually ends.
func TestH84_AnOpenRunIsNotACoverageGap(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))
	// Deliberately no probe("end", ...): the run stays open, as it is at Stop.

	d := e.digest("--session", testSession, "--prompt", p.PromptID)
	if d.Coverage.State != "verified" {
		t.Fatalf("state = %q, want verified: a probe at start and one clean call is a healthy "+
			"turn even though the run has not ended. reasons: %v", d.Coverage.State, d.Coverage.Reasons)
	}
	if e.hasCoverageReason(d, "run_not_closed") {
		t.Fatalf("reasons = %v, carries run_not_closed -- the break this item names: "+
			"projecting session coverage marks every healthy turn unverified because the "+
			"session, by construction, has not ended at Stop", d.Coverage.Reasons)
	}
}

// TestH84_ADeclarationWithNoExecutionYetIsInFlightNotMissing covers a call
// still running in the background: a declaration with no execution record at
// all must not, by itself, make an open turn's coverage unverified.
func TestH84_ADeclarationWithNoExecutionYetIsInFlightNotMissing(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	e.mustHook(p.build(t))
	// No post at all: PostToolUse has not fired yet, exactly as it would not
	// yet have for a backgrounded command still running.

	d := e.digest("--session", testSession, "--prompt", p.PromptID)
	if len(d.Declarations.WithoutExecution) != 1 {
		t.Fatalf("without_execution = %+v, want the one in-flight call listed", d.Declarations.WithoutExecution)
	}
	if d.Coverage.State != "verified" {
		t.Errorf("state = %q, want verified: a declaration with no execution YET, on an open "+
			"run, is what a still-running background call looks like -- not a finding",
			d.Coverage.State)
	}
}

// TestH84_AKilledHookProcessIsInFlightNotMissingWhileOpen simulates the
// uncontrolled path H-11 covers at session scope: a hook process SIGKILLed
// between writing its declaration and its terminal. While the run is open,
// this must not flip the turn's coverage either -- only once the run has
// actually ended does the very same absence become a finding (that is H-11's
// own scope, unmodified by this item).
func TestH84_AKilledHookProcessIsInFlightNotMissingWhileOpen(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	kill, wait := e.process(defaultPayload().build(t))
	kill(syscall.SIGKILL)
	wait()

	if got := e.terminals(testSession); len(got) != 0 {
		t.Fatalf("a SIGKILLed handler left %d terminal records; it cannot have written any", len(got))
	}

	d := e.digest("--session", testSession, "--prompt", "prompt-1")
	if len(d.Declarations.Unterminated) != 1 {
		t.Fatalf("unterminated = %v, want the one killed call listed", d.Declarations.Unterminated)
	}
	if d.Coverage.State != "verified" {
		t.Errorf("state = %q, want verified while the run is still open: an unterminated entry "+
			"is indistinguishable from an in-flight call until the run actually ends. reasons: %v",
			d.Coverage.State, d.Coverage.Reasons)
	}
}

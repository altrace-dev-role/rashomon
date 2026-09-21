package acceptance

import (
	"strings"
	"testing"
)

// H-92 -- status distinguishes "evaluated, no findings" from "not
// evaluated".
//
// Break: report only the last finding and silence becomes indistinguishable
// from a recap that never ran -- exactly the collapse exception-only
// notification would otherwise cause. Silence on Stop is a notification
// policy, never a claim the turn was clean: the plugin can be disabled, the
// hook can fail to run, or a turn can end without Stop at all. Status needs
// a fact silence itself cannot carry, and this test drives a CLEAN turn
// through recap -- recapLine reports ok=false, exactly as H-90 wants -- and
// checks that status nonetheless moves from "not evaluated" to "evaluated":
// the absence of a printed line and the absence of an evaluation are
// different facts, and status is the one place required to keep them apart.
func TestH92_StatusDistinguishesNotEvaluatedFromEvaluatedNoFindings(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	before := e.status()
	if before.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", before.exitCode, before.stderr)
	}
	if !strings.Contains(before.stdout, "recap: not evaluated") {
		t.Fatalf("status before any Stop = %q, want it to say recap is not evaluated", before.stdout)
	}
	if strings.Contains(before.stdout, "recap: evaluated") {
		t.Fatalf("status claims recap evaluated before recap ever ran: %q", before.stdout)
	}

	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))

	line, ok := e.recapLine(stopPayload(testSession, "All good, nothing to report.", false))
	if ok {
		t.Fatalf("premise: this turn should be clean, but recap printed %q", line)
	}

	after := e.status()
	if after.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", after.exitCode, after.stderr)
	}
	if strings.Contains(after.stdout, "recap: not evaluated") {
		t.Errorf("status after a silent, clean recap run still says not evaluated: %q -- "+
			"silence has become indistinguishable from a recap that never ran", after.stdout)
	}
	if !strings.Contains(after.stdout, "recap: evaluated") {
		t.Errorf("status after a recap run = %q, want it to say recap is evaluated", after.stdout)
	}
}

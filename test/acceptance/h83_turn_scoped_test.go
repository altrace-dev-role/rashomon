package acceptance

import "testing"

// H-83 -- the digest is turn-scoped.
//
// Two turns, a failure in the first only; the second digest is empty of it.
// Break: scope to the session and the second turn inherits the first's
// finding.
func TestH83_TheDigestIsTurnScoped(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// Turn 1: prompt-1, one call, fails.
	p1 := defaultPayload()
	p1.ToolUseID, p1.PromptID = "toolu_a1", "prompt-1"
	e.mustHook(p1.build(t))
	e.mustPost(failurePayload(t, "toolu_a1", "Exit code 1", false, 10))

	// Turn 2: prompt-2, one call, succeeds.
	p2 := defaultPayload()
	p2.ToolUseID, p2.PromptID = "toolu_b1", "prompt-2"
	e.mustHook(p2.build(t))
	post2 := defaultPost()
	post2.ToolUseID = "toolu_b1"
	e.mustPost(post2.build(t))

	d1 := e.digest("--session", testSession, "--prompt", "prompt-1")
	if d1.Declarations.Recorded != 1 {
		t.Fatalf("prompt-1 recorded = %d, want 1", d1.Declarations.Recorded)
	}
	if d1.SilentFailures.Failed != 1 {
		t.Fatalf("prompt-1 failed = %d, want 1", d1.SilentFailures.Failed)
	}

	d2 := e.digest("--session", testSession, "--prompt", "prompt-2")
	if d2.Declarations.Recorded != 1 {
		t.Fatalf("prompt-2 recorded = %d, want 1: only its OWN call", d2.Declarations.Recorded)
	}
	if d2.SilentFailures.Failed != 0 {
		t.Errorf("prompt-2 failed = %d, want 0: the second turn's digest must be empty of the "+
			"first turn's failure. Break: scope to the session and this becomes 1.",
			d2.SilentFailures.Failed)
	}
	if len(d2.Declarations.WithoutExecution) != 0 {
		t.Errorf("prompt-2 without_execution = %+v, want none: prompt-1's failed call must not "+
			"appear in prompt-2's digest", d2.Declarations.WithoutExecution)
	}
}

package acceptance

import "testing"

// H-85 -- zero is not unknown.
//
// A turn with tools recorded elsewhere in a healthy session and none used in
// THIS one renders zero; a turn in a session with no evidence the recorder
// ever engaged renders unknown. Break: collapse them and a session with a
// dead recorder reads as a session with a well-behaved agent.
func TestH85_ZeroDeclarationsInAHealthySessionIsAKnownZero(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload() // prompt-1
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))
	e.probe("end", testSession)

	// prompt-2 was never used in this session, which the recorder otherwise
	// demonstrably engaged with (a probe at start and end, one clean call).
	d := e.digest("--session", testSession, "--prompt", "prompt-2")
	if d.Declarations.Recorded != 0 {
		t.Fatalf("premise: recorded = %d, want 0", d.Declarations.Recorded)
	}
	if d.Unknown {
		t.Errorf("unknown = true for a turn that made no calls in a session the recorder "+
			"demonstrably engaged (coverage: %v). Zero calls is a real fact here, not a sign "+
			"the recorder was off.", d.Coverage.Reasons)
	}
}

// TestH85_ZeroDeclarationsWithNoEvidenceOfRecordingIsUnknown is the other
// half, and the break's own scenario made concrete: a session with no
// SessionStart probe at all -- no evidence the recorder ever engaged -- must
// not let an empty turn inside it read as a clean zero.
func TestH85_ZeroDeclarationsWithNoEvidenceOfRecordingIsUnknown(t *testing.T) {
	e := newEnv(t)
	// Deliberately not e.watched(): no probe start, ever. A call still
	// lands -- the hook opens (and creates) the store on first use even
	// without `watch` -- so the session exists, but nothing establishes that
	// the recorder was actually engaged for it.
	p := defaultPayload()
	e.mustHook(p.build(t))

	d := e.digest("--session", testSession, "--prompt", "prompt-missing")
	if d.Declarations.Recorded != 0 {
		t.Fatalf("premise: recorded = %d, want 0", d.Declarations.Recorded)
	}
	if !d.Unknown {
		t.Errorf("unknown = false for a turn in a session with no probe record at all: this is "+
			"the collapse the break names -- a session with a dead recorder reading as a "+
			"session with a well-behaved agent. coverage: %v", d.Coverage.Reasons)
	}
}

// TestH85_NonZeroCountIsNeverUnknown closes the loop: a real, non-zero count
// is never marked unknown, whatever coverage says beside it -- a coverage
// problem next to a real count is a coverage finding, not reason to doubt the
// count itself.
func TestH85_NonZeroCountIsNeverUnknown(t *testing.T) {
	e := newEnv(t)
	p := defaultPayload()
	e.mustHook(p.build(t)) // no watch: coverage will be unverified (probe_absent)

	d := e.digest("--session", testSession, "--prompt", "prompt-1")
	if d.Declarations.Recorded == 0 {
		t.Fatal("premise: recorded should be 1")
	}
	if d.Coverage.State == "verified" {
		t.Fatal("premise: coverage should not be verified with no probe record at all")
	}
	if d.Unknown {
		t.Error("unknown = true for a turn with a real, non-zero recorded count")
	}
}

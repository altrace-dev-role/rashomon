package acceptance

import "testing"

// The settings-load injection point, mirroring internal/fault. The kind
// selected there is the one that returns an error instead of panicking, which
// is what a retry can be measured against.
const pointSettingsLoad = "settings.load"

// TestH2_TransientSettingsReadIsRetried: a settings.json read that fails once,
// because another tool was rewriting the file non-atomically at that instant,
// must not cost the call its coverage. Resolving "unknown" there taints the
// run and not merely the call: one unverified call is enough to make the whole
// run unverified in report, so a millisecond of someone else's write would
// permanently outrank everything the run recorded.
func TestH2_TransientSettingsReadIsRetried(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	res := e.hook(defaultPayload().build(t), "RASHOMON_FAULT="+pointSettingsLoad+":fail=1")
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0 (stderr: %q)", res.exitCode, res.stderr)
	}
	assertNoTraceback(t, res)

	recs := e.coverage(testSession, "call")
	if len(recs) == 0 {
		t.Fatalf("no call-phase coverage record for session %q", testSession)
	}
	if got := recs[len(recs)-1].str("hook_entry"); got != "present_settings" {
		t.Errorf("hook_entry is %q, want \"present_settings\": a later attempt read the file the first one missed", got)
	}
	assertCallCoverage(t, e, testSession, "verified", "")

	// The run, which is what the one failed read would otherwise have cost.
	e.probe("end", testSession)
	if rep := e.report(testSession); rep.Coverage.State != "verified" {
		t.Errorf("run coverage is %s (%v), want verified", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH2_PersistentSettingsReadIsUnresolved is the bound on that retry, and it
// is also the guard that the fault in the test above fired at all: same point,
// same kind, ten failures rather than one, and the only way this test can pass
// is if the injection really is failing the read. Without it, a retry that
// never gave up would pass the test above, and so would a fault that was never
// wired up. A read that keeps failing is a configuration this handler cannot
// see, and it has to say so.
func TestH2_PersistentSettingsReadIsUnresolved(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	res := e.hook(defaultPayload().build(t), "RASHOMON_FAULT="+pointSettingsLoad+":fail=10")
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0 (stderr: %q)", res.exitCode, res.stderr)
	}
	assertNoTraceback(t, res)

	recs := e.coverage(testSession, "call")
	if len(recs) == 0 {
		t.Fatalf("no call-phase coverage record for session %q", testSession)
	}
	if got := recs[len(recs)-1].str("hook_entry"); got != "unknown" {
		t.Errorf("hook_entry is %q, want \"unknown\"", got)
	}
	assertCallCoverage(t, e, testSession, "unverified", "hook_entry_unresolved")
}

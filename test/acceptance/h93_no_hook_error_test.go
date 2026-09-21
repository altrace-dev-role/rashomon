package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H-93 -- the recap never produces a hook error.
//
// Break: wrap it in guarded() and a corrupt store puts an error on screen
// after every turn. Two internal failures are forced here, both required by
// the spec's acceptance text: a panic (fault injection, the same mechanism
// H-1 uses for the recorder) and a store this binary cannot read cleanly.
// Both must exit 0 and print nothing on stdout.
func TestH93_APanicNeverProducesAHookError(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))

	res := e.recap(stopPayload(testSession, "All good.", false), "RASHOMON_FAULT=recap.start:plain_panic")

	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0 -- a panic inside recap must never reach the exit code "+
			"that renders as a visible hook error", res.exitCode)
	}
	if res.stdout != "" {
		t.Errorf("stdout = %q, want empty", res.stdout)
	}
	for _, marker := range []string{"panic:", "goroutine ", "runtime error:"} {
		if strings.Contains(res.stderr, marker) {
			t.Errorf("stderr contains %q: %q", marker, res.stderr)
		}
	}
}

// TestH93_AStoreThisBinaryCannotOpenNeverProducesAHookError corrupts
// install.key -- present, but neither valid hex nor the right length --
// which turns store.Open's read of it into a real error rather than
// store.ErrNoStore. openStoreForRead returns that error to cmdRecap exactly
// as it would any other internal failure, and this is the case the spec
// names "a malformed store".
func TestH93_AStoreThisBinaryCannotOpenNeverProducesAHookError(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))

	if err := os.WriteFile(filepath.Join(e.home, "install.key"), []byte("not-hex-and-not-32-bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := e.recap(stopPayload(testSession, "All good.", false))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0 -- a corrupt store must not put an error on screen", res.exitCode)
	}
	if res.stdout != "" {
		t.Errorf("stdout = %q, want empty", res.stdout)
	}
}

// TestH93_AnUnreadableRunAlsoMarksTheFailureInStatus is the softer half of
// "malformed store": one this binary CAN open, but cannot read cleanly
// (gaps.ndjson replaced with a directory, so the read that expects a file
// gets a real I/O error rather than a missing-file no-op). Unlike the
// install.key case above, nothing here stops status from opening the store
// too, so this is where the "recorded where status reports it" half of the
// spec's failure rule is checked end to end.
func TestH93_AnUnreadableRunAlsoMarksTheFailureInStatus(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))

	gapsPath := filepath.Join(e.home, "gaps.ndjson")
	if err := os.Remove(gapsPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gapsPath, 0o700); err != nil {
		t.Fatal(err)
	}

	res := e.recap(stopPayload(testSession, "All good.", false))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}
	if res.stdout != "" {
		t.Errorf("stdout = %q, want empty", res.stdout)
	}

	st := e.status()
	if st.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", st.exitCode, st.stderr)
	}
	if !strings.Contains(st.stdout, "recap: evaluated") || !strings.Contains(st.stdout, "did not complete cleanly") {
		t.Errorf("status after the unreadable-run recap run = %q, want it to say recap "+
			"evaluated but did not complete cleanly -- this is the ONE place that failure is "+
			"visible, since recap itself must print nothing (H-93)", st.stdout)
	}
}

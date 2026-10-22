package acceptance

import (
	"strings"
	"testing"
)

// TestRubricPERF7_StatusShowsTheStoreAgainstItsCap: how close the store is
// to its cap was invisible until a run was evicted. status prints the size
// eviction compares against the cap in force, and says when eviction is off.
//
// The cap eviction runs with is resolved inside the probe's own hook
// invocation, never in status's process, which is the regression this test
// now pins (hld-7's second half): every env var below is on probe, and
// status itself runs with none set.
func TestRubricPERF7_StatusShowsTheStoreAgainstItsCap(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.probe("start", testSession); res.exitCode != 0 {
		t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.mustHook(defaultPayload().build(t))

	if got := fieldLine(t, e.status().stdout, "size"); !strings.HasSuffix(got, " of a 512 MiB cap") || strings.HasPrefix(got, "0 B") {
		t.Errorf("status size line is %q, want a non-zero size of a 512 MiB cap", got)
	}

	if res := e.probe("start", "sess-1mib", "RASHOMON_STORE_CAP_BYTES=1048576"); res.exitCode != 0 {
		t.Fatalf("probe start under a 1 MiB cap: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := fieldLine(t, e.status().stdout, "size"); !strings.HasSuffix(got, " of a 1 MiB cap") {
		t.Errorf("status size line after a probe under a 1 MiB cap is %q, want the probe's cap even though status's own shell never saw it", got)
	}

	if res := e.probe("start", "sess-off", "RASHOMON_STORE_CAP_BYTES=0"); res.exitCode != 0 {
		t.Fatalf("probe start under a zero cap: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := fieldLine(t, e.status().stdout, "size"); !strings.HasSuffix(got, "(eviction off: RASHOMON_STORE_CAP_BYTES=0)") {
		t.Errorf("status size line after a probe under a zero cap is %q, want it to say eviction is off", got)
	}
}

// TestRubricHLD7_StatusAndWatchSayACapIsNotAByteCount: the cap is read
// inside a hook, which cannot print, so a value that is not a byte count
// used to fall back to the default where nobody could see it. status says
// so using the value the probe actually resolved, and watch, reading its
// own shell directly, says so for itself: on the install that writes the
// entries and on the one that finds them already there.
func TestRubricHLD7_StatusAndWatchSayACapIsNotAByteCount(t *testing.T) {
	e := newEnv(t)
	if res := e.watch("RASHOMON_STORE_CAP_BYTES=1.5e9"); res.exitCode != 0 || !strings.Contains(res.stdout, `RASHOMON_STORE_CAP_BYTES="1.5e9" is not a byte count`) {
		t.Fatalf("first watch under a malformed cap: exit %d, stderr %q, stdout:\n%s", res.exitCode, res.stderr, res.stdout)
	}
	const want = `RASHOMON_STORE_CAP_BYTES="2GB" is not a byte count`

	if res := e.probe("start", testSession, "RASHOMON_STORE_CAP_BYTES=2GB"); res.exitCode != 0 {
		t.Fatalf("probe start under a malformed cap: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	st := e.status()
	if !strings.Contains(st.stdout, want) {
		t.Errorf("status after a probe under a malformed cap does not say so:\n%s", st.stdout)
	}
	if got := fieldLine(t, st.stdout, "size"); !strings.HasSuffix(got, " of a 512 MiB cap") {
		t.Errorf("status size line under a malformed cap is %q, want the default cap", got)
	}

	if w := e.watch("RASHOMON_STORE_CAP_BYTES=-1"); w.exitCode != 0 || !strings.Contains(w.stdout, `RASHOMON_STORE_CAP_BYTES="-1" is not a byte count`) || !strings.Contains(w.stdout, "already watching") {
		t.Errorf("watch again under a negative cap: exit %d, stdout:\n%s", w.exitCode, w.stdout)
	}

	// A store no probe has ever touched has nothing recorded, so status
	// falls back to its own shell -- unset here -- rather than the earlier
	// store's problem.
	fresh := newEnv(t)
	if res := fresh.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := fresh.probe("start", testSession); res.exitCode != 0 {
		t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if st := fresh.status(); strings.Contains(st.stdout, "is not a byte count") {
		t.Errorf("status complains about an unset cap:\n%s", st.stdout)
	}
}

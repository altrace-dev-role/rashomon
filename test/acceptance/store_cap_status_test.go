package acceptance

import (
	"strings"
	"testing"
)

// TestRubricPERF7_StatusShowsTheStoreAgainstItsCap: how close the store is
// to its cap was invisible until a run was evicted. status prints the size
// eviction compares against the cap in force, and says when eviction is off.
func TestRubricPERF7_StatusShowsTheStoreAgainstItsCap(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	if got := fieldLine(t, e.status().stdout, "size"); !strings.HasSuffix(got, " of a 512 MiB cap") || strings.HasPrefix(got, "0 B") {
		t.Errorf("status size line is %q, want a non-zero size of a 512 MiB cap", got)
	}
	if got := fieldLine(t, e.status("RASHOMON_STORE_CAP_BYTES=1048576").stdout, "size"); !strings.HasSuffix(got, " of a 1 MiB cap") {
		t.Errorf("status size line under a 1 MiB cap is %q", got)
	}
	if got := fieldLine(t, e.status("RASHOMON_STORE_CAP_BYTES=0").stdout, "size"); !strings.HasSuffix(got, "(eviction off: RASHOMON_STORE_CAP_BYTES=0)") {
		t.Errorf("status size line under a zero cap is %q, want it to say eviction is off", got)
	}
}

// TestRubricHLD7_StatusAndWatchSayACapIsNotAByteCount: the cap is read
// inside a hook, which cannot print, so a value that is not a byte count
// used to fall back to the default where nobody could see it. status and
// watch, which can print, say so.
func TestRubricHLD7_StatusAndWatchSayACapIsNotAByteCount(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	const want = `RASHOMON_STORE_CAP_BYTES="2GB" is not a byte count`

	st := e.status("RASHOMON_STORE_CAP_BYTES=2GB")
	if !strings.Contains(st.stdout, want) {
		t.Errorf("status under a malformed cap does not say so:\n%s", st.stdout)
	}
	if got := fieldLine(t, st.stdout, "size"); !strings.HasSuffix(got, " of a 512 MiB cap") {
		t.Errorf("status size line under a malformed cap is %q, want the default cap", got)
	}
	if w := e.watch("RASHOMON_STORE_CAP_BYTES=-1"); w.exitCode != 0 || !strings.Contains(w.stdout, `RASHOMON_STORE_CAP_BYTES="-1" is not a byte count`) {
		t.Errorf("watch under a negative cap: exit %d, stdout:\n%s", w.exitCode, w.stdout)
	}
	if st := e.status(); strings.Contains(st.stdout, "is not a byte count") {
		t.Errorf("status complains about an unset cap:\n%s", st.stdout)
	}
}

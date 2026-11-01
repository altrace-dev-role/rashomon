package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSC6_SessionStartSetsTheSteerMarkerFromConsent: the binary's SessionStart
// probe keeps the steer_disclosure marker equal to the highest steer
// disclosure consent.json holds, beside the record_scope marker: an install
// whose consent predates the marker gains it at its next session start, and
// a marker that disagrees is set to consent.json's answer, lowered as well as
// raised: a marker above consent, which a crash or a hand edit can leave,
// reads as agreement to more than the person gave. Neither adds a reason to
// the start coverage record.
func TestSC6_SessionStartSetsTheSteerMarkerFromConsent(t *testing.T) {
	marker := func(e *env) string { return filepath.Join(e.home, "steer_disclosure") }
	start := func(t *testing.T, e *env) string {
		t.Helper()
		if res := e.probe("start", testSession); res.exitCode != 0 {
			t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if got := startReason(t, e); got != nil {
			t.Errorf("start coverage reason = %v, want null", got)
		}
		b, err := os.ReadFile(marker(e))
		if err != nil {
			t.Fatalf("after a session start: %v", err)
		}
		return string(b)
	}
	t.Run("consent from before the marker", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		// watch --yes records this build's steer disclosure (STEER-DISCLOSE-YES1).
		if !strings.Contains(string(e.consentBytes()), `"steer_disclosure":2`) {
			t.Fatal("premise: the watch record does not carry steer_disclosure 2")
		}
		if err := os.Remove(marker(e)); err != nil {
			t.Fatal(err)
		}
		if got := start(t, e); got != "2\n" {
			t.Errorf("after a session start the steer marker holds %q, want \"2\\n\"", got)
		}
	})
	t.Run("a marker that disagrees", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		// Above consent's 2, which watch --yes records (STEER-DISCLOSE-YES1),
		// so the sync has to lower it: the SC-2.2c "only raises" row.
		if err := os.WriteFile(marker(e), []byte("3\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := start(t, e); got != "2\n" {
			t.Errorf("after a session start the steer marker holds %q, want \"2\\n\"", got)
		}
	})
}

package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestH15_ForgetKeepsSpillOutsideTheWindow: forget rewrites the spill file
// as well as the ordered stream, and writes back every spill line outside
// its window. A spilled terminal is the only trace of a declaration that
// could not be written, so one wiped with the window's lines is a dropped
// call nothing records, under a gap that counted only the window (test-2).
func TestH15_ForgetKeepsSpillOutsideTheWindow(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	cut := time.Now().UTC().Add(-30 * time.Minute)
	spill := filepath.Join(e.home, "runs", testSession, "spill.ndjson")
	var body []byte
	for id, at := range map[string]time.Time{
		"toolu_spilled_old": cut.Add(-time.Hour),
		"toolu_spilled_new": cut.Add(time.Hour),
	} {
		line, err := json.Marshal(map[string]any{
			"type": "terminal", "schema_version": 4, "recorded_at_unix_ms": at.UnixMilli(), "seq": nil,
			"tool_use_id": id, "session_id": testSession, "outcome": "error", "reason": "lock_timeout",
		})
		if err != nil {
			t.Fatal(err)
		}
		body = append(append(body, line...), '\n')
	}
	if err := os.WriteFile(spill, body, 0o600); err != nil {
		t.Fatal(err)
	}

	if res := e.forgetBefore(cut.Format(time.RFC3339Nano)); res.exitCode != 0 {
		t.Fatalf("forget --before: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	left := e.read(testSession, "spill.ndjson")
	if len(left) != 1 || left[0].str("tool_use_id") != "toolu_spilled_new" {
		t.Errorf("spill file holds %v after forget --before, want only toolu_spilled_new", left)
	}
	if got := len(e.declarations(testSession)); got != 1 {
		t.Errorf("%d declarations survived, want the 1 recorded after the cut", got)
	}
	gaps := e.gaps()
	if len(gaps) != 1 || gaps[0].fields["removed_records"] != float64(1) {
		t.Errorf("gaps are %v, want one gap removing the 1 spilled terminal in the window", gaps)
	}
}

package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestConflictCheck_IsTheThirdNonCriticalKind: the conflict_check line says
// critical false, holds the edit's seq, the commit, the file's digest and
// the rows, writes a check that found nothing as an empty list, never null,
// a reader with the type files it, and a reader refusing the pair counts it
// Ignored and still reads the session whole.
func TestConflictCheck_IsTheThirdNonCriticalKind(t *testing.T) {
	rec := NewConflictCheck("s", 7, "c0ffee", "sha256:f", []ConflictCheckRow{{Kind: "boundary_cross", RuleID: "b1",
		Unit: "sha256:u", Subject: "sha256:s", Key: "sha256:k"}}, 5)
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"conflict_check"`, `"critical":false`, `"landed_seq":7`, `"commit":"c0ffee"`,
		`"file":"sha256:f"`, `"rows":[{"kind":"boundary_cross","rule_id":"b1","unit":"sha256:u","unit_path":"",` +
			`"subject":"sha256:s","key":"sha256:k"}]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("line %s lacks %s", b, want)
		}
	}
	empty, err := json.Marshal(NewConflictCheck("s", 8, "c0ffee", "sha256:f", nil, 5))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(empty), `"rows":[]`) {
		t.Errorf("a check that found nothing: %s, want rows []", empty)
	}
	if !AcceptsKind(TypeConflictCheck, EventSchemaVersion) {
		t.Error("AcceptsKind(conflict_check) = false")
	}
	var run Run
	classifyByHead(&run, b)
	classifyByHead(&run, empty)
	if len(run.ConflictChecks) != 2 || run.Skipped != 0 || run.Ignored != 0 {
		t.Fatalf("run = %+v, want two conflict checks and nothing skipped or ignored", run)
	}
	if got := run.ConflictChecks[0]; got.LandedSeq != 7 || got.Commit != "c0ffee" || len(got.Rows) != 1 || got.Rows[0].Key != "sha256:k" {
		t.Errorf("read back %+v", got)
	}
	old := strings.Replace(string(b), `"schema_version":`+itoa(EventSchemaVersion), `"schema_version":99`, 1)
	var older Run
	classifyByHead(&older, []byte(old))
	if older.Ignored != 1 || older.Skipped != 0 {
		t.Errorf("a reader refusing the pair: Ignored %d Skipped %d, want 1 and 0", older.Ignored, older.Skipped)
	}
}

package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPlanCheck_IsTheNonCriticalKind: the plan_check line says critical false,
// so a reader that lacks the type counts it Ignored and still reads the
// session whole, holds no null list, and a reader with the type files it.
func TestPlanCheck_IsTheNonCriticalKind(t *testing.T) {
	rec := NewPlanCheck("s", 7, nil, 1, 1, 1, nil, 5)
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"plan_check"`, `"critical":false`, `"rule_ids":[]`, `"path_digests":[]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("line %s lacks %s", b, want)
		}
	}
	if !AcceptsKind(TypePlanCheck, EventSchemaVersion) {
		t.Error("AcceptsKind(plan_check) = false")
	}
	var run Run
	classifyByHead(&run, b)
	if len(run.PlanChecks) != 1 || run.Skipped != 0 || run.Ignored != 0 {
		t.Errorf("run = %+v, want one plan check and nothing skipped or ignored", run)
	}
	old := strings.Replace(string(b), `"schema_version":`+itoa(EventSchemaVersion), `"schema_version":99`, 1)
	var older Run
	classifyByHead(&older, []byte(old))
	if older.Ignored != 1 || older.Skipped != 0 {
		t.Errorf("a reader refusing the pair: Ignored %d Skipped %d, want 1 and 0", older.Ignored, older.Skipped)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

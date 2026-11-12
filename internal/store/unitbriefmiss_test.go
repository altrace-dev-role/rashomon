package store

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// TestUnitBriefMiss_IsTheSecondNonCriticalKind: the unitbrief_miss line says
// critical false, holds the edit's seq, the unit's digest and the reason, a
// reader with the type files it, and a reader refusing the pair counts it
// Ignored and still reads the session whole. Its reasons are a closed set
// of three.
func TestUnitBriefMiss_IsTheSecondNonCriticalKind(t *testing.T) {
	rec := NewUnitBriefMiss("s", 7, "sha256:u", UnitBriefAbsent, 5)
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"unitbrief_miss"`, `"critical":false`, `"landed_seq":7`,
		`"unit":"sha256:u"`, `"reason":"absent"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("line %s lacks %s", b, want)
		}
	}
	if !AcceptsKind(TypeUnitBriefMiss, EventSchemaVersion) {
		t.Error("AcceptsKind(unitbrief_miss) = false")
	}
	var run Run
	classifyByHead(&run, b)
	if len(run.UnitBriefMisses) != 1 || run.Skipped != 0 || run.Ignored != 0 {
		t.Errorf("run = %+v, want one unit-brief miss and nothing skipped or ignored", run)
	}
	old := strings.Replace(string(b), `"schema_version":`+itoa(EventSchemaVersion), `"schema_version":99`, 1)
	var older Run
	classifyByHead(&older, []byte(old))
	if older.Ignored != 1 || older.Skipped != 0 {
		t.Errorf("a reader refusing the pair: Ignored %d Skipped %d, want 1 and 0", older.Ignored, older.Skipped)
	}
	if got := UnitBriefMissReasons(); !slices.Equal(got, []string{"absent", "stale", "refused"}) {
		t.Errorf("reasons %v, want absent, stale, refused", got)
	}
}

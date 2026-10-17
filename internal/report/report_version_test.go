package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestReportVersion_IsCarriedOnEveryReport: the destinations, families and
// chains keys changed incompatibly in the move off the proxy -- renamed,
// removed, and nothing added in their place to say so. A typed decoder gets
// a zero value for a missing key rather than an error, so a consumer has no
// way to notice the shape moved under it short of reading the release notes.
// report_version exists so that noticing is a field read, not a diff.
func TestReportVersion_IsCarriedOnEveryReport(t *testing.T) {
	if Empty(time.UnixMilli(0)).ReportVersion != CurrentReportVersion {
		t.Errorf("Empty's report_version = %d, want %d", Empty(time.UnixMilli(0)).ReportVersion, CurrentReportVersion)
	}

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Build(st, "", time.UnixMilli(0))
	if err != nil {
		t.Fatal(err)
	}
	if rep.ReportVersion != CurrentReportVersion {
		t.Errorf("Build's report_version = %d, want %d", rep.ReportVersion, CurrentReportVersion)
	}

	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if got, ok := decoded["report_version"].(float64); !ok || int(got) != CurrentReportVersion {
		t.Errorf("marshalled report_version = %v, want %d", decoded["report_version"], CurrentReportVersion)
	}
}

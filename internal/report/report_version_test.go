package report

import (
	"encoding/json"
	"fmt"
	"strings"
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

// TestRubricNAMING3_TheReportCarriesItsReportVersion: report --json is a
// consumer's contract, as the digest and spend documents are, and like them
// it says which version of itself it is. Both of its constructors open the
// document with report_version, ahead of generated_at_unix_ms, and the
// number moves only as CurrentReportVersion's bump rule says.
func TestRubricNAMING3_TheReportCarriesItsReportVersion(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(st, "", now)
	if err != nil {
		t.Fatal(err)
	}
	for name, rep := range map[string]*Report{"Empty": Empty(now), "Build": built} {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("{\n  \"report_version\": 2,\n  \"generated_at_unix_ms\": %d,\n", now.UnixMilli())
		if !strings.HasPrefix(string(b), want) {
			t.Errorf("%s: the report opens\n%s\nwant it to open\n%s", name, b, want)
		}
	}
	if CurrentReportVersion != 2 {
		t.Errorf("CurrentReportVersion = %d; a bump is a deliberate change to this test and the golden", CurrentReportVersion)
	}
}

package acceptance

import "testing"

// TestREC3c_ReportPassesOverANonCriticalRecord is REC-3 end to end: a record
// of a kind this binary does not know, marked `"critical": false`, leaves a
// healthy session verified with nothing skipped, while the same record
// marked critical, or carrying no critical key, makes it read
// records_unreadable. Break: read the bit the wrong way round, or treat a
// missing key as non-critical.
func TestREC3c_ReportPassesOverANonCriticalRecord(t *testing.T) {
	for _, c := range []struct {
		name, line string
		verified   bool
	}{
		{"non-critical", `{"type":"future_kind","schema_version":9,"critical":false}`, true},
		{"critical", `{"type":"future_kind","schema_version":9,"critical":true}`, false},
		{"no key", `{"type":"future_kind","schema_version":9}`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)
			e.mustHook(defaultPayload().build(t))
			e.probe("end", testSession)
			appendRawLine(t, e.recordsPath(testSession), c.line)

			rep := e.report(testSession)
			if got := rep.Coverage.State == "verified"; got != c.verified {
				t.Errorf("coverage %s with reasons %v and %d skipped; want verified %v",
					rep.Coverage.State, rep.Coverage.Reasons, rep.SkippedRecords, c.verified)
			}
			if got := e.hasReason(rep, "records_unreadable"); got == c.verified {
				t.Errorf("records_unreadable named: %v, want %v", got, !c.verified)
			}
		})
	}
}

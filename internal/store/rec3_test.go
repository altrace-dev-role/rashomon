package store

import (
	"path/filepath"
	"testing"
)

// TestREC3_UnknownNonCriticalKindIsIgnored: a record this reader does not
// know, of a kind or at a version it does not accept, that says
// `"critical": false` is counted in Run.Ignored and not in Run.Skipped, so a
// future measurement record never turns every older session unverified. A
// known kind at the version it is written at is read, and neither count
// moves; the calls on either side still read.
func TestREC3_UnknownNonCriticalKindIsIgnored(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(testDecl("sess-ign", "toolu_1")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.RunDir("sess-ign"), FileRecords)
	for _, line := range []string{
		`{"type":"future_kind","schema_version":9,"critical":false,"seq":null}`,
		`{"type":"declaration","schema_version":6,"critical":false}`,
		`{"type":"turn_end","schema_version":5,"critical":false,"seq":null,"recorded_at_unix_ms":1,"session_id":"sess-ign"}`,
	} {
		appendRaw(t, path, []byte(line+"\n"))
	}
	if err := st.AppendDeclaration(testDecl("sess-ign", "toolu_2")); err != nil {
		t.Fatal(err)
	}

	run, err := st.ReadRun("sess-ign")
	if err != nil {
		t.Fatal(err)
	}
	if run.Ignored != 2 || run.Skipped != 0 || len(run.Declarations) != 2 {
		t.Errorf("ignored %d, skipped %d, declarations %d; want 2, 0 and 2", run.Ignored, run.Skipped, len(run.Declarations))
	}
}

// TestREC3_UnknownCriticalKindUnverifies: a record this reader cannot read
// that says `"critical": true`, or says nothing, is skipped and counted in
// Run.Skipped, which the report turns into records_unreadable: a record that
// may change a verdict is never dropped silently. A critical key that is not
// a boolean is a line that does not parse, and counts the same way, as does a
// known kind whose header does not parse.
func TestREC3_UnknownCriticalKindUnverifies(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.RunDir("sess-crit"), FileRecords)
	for _, line := range []string{
		`{"type":"future_kind","schema_version":9,"critical":true}`,
		`{"type":"future_kind","schema_version":9}`,
		`{"type":"declaration","schema_version":6}`,
		`{"type":"coverage","schema_version":4,"critical":false}`,
		`{"type":"future_kind","schema_version":9,"critical":"no"}`,
		`{"type":"turn_end","schema_version":4,"critical":true}`,
		`{"type":"turn_end","schema_version":5,"critical":false,"seq":"one"}`,
	} {
		appendRaw(t, path, []byte(line+"\n"))
	}

	run, err := st.ReadRun("sess-crit")
	if err != nil {
		t.Fatal(err)
	}
	// The coverage line at version 4 says critical false and so is ignored:
	// the bit is the writer's, whatever the kind. The last line is a known
	// kind at its version whose header does not parse, which is a line that
	// could not be read, whatever its critical key says.
	if run.Skipped != 6 || run.Ignored != 1 {
		t.Errorf("skipped %d, ignored %d; want 6 and 1", run.Skipped, run.Ignored)
	}
}

// TestREC3b_GapAtAnUnknownVersionIsCounted: the gaps reader applies the same
// acceptance rule as the run reader. A gap at a version this reader does not
// accept used to be read as if it were one it understood; it now counts as a
// line it could not read, which every reader turns into records_unreadable
// for every session (REC-33).
func TestREC3b_GapAtAnUnknownVersionIsCounted(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Root(), FileGaps)
	appendRaw(t, path, []byte(`{"type":"gap","schema_version":3,"session_id":"s","reason":"forget"}`+"\n"))
	appendRaw(t, path, []byte(`{"type":"gap","schema_version":4,"session_id":"s","reason":"forget"}`+"\n"))
	gaps, skipped, err := st.ReadGaps()
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || skipped != 1 {
		t.Errorf("read %d gaps with %d skipped, want 1 and 1", len(gaps), skipped)
	}
}

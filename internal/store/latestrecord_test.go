package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRG1f_LatestRecordBytesIsTheLastOfItsKindAsWritten (5.5, Gate B 1): the
// line handed back is the run's last record of the kind at an accepted
// version, byte for byte as the file holds it, without its newline: the
// spaces json.Marshal would never write survive. A record of another kind,
// the kind at a version AcceptsKind refuses, a damaged line and a torn tail
// are not it, and the last two count in skipped. A run with no record of the
// kind, and a run with no records file, are ok false. Break: hand back the
// first match, or compact the line, and the first row names it.
func TestRG1f_LatestRecordBytesIsTheLastOfItsKindAsWritten(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-rg1f"
	if err := os.MkdirAll(st.RunDir(session), dirMode); err != nil {
		t.Fatal(err)
	}
	last := `{"type": "verify", "schema_version":5,"seq":3,  "n":2}`
	lines := []string{
		`{"type":"verify","schema_version":5,"seq":1,"n":1}`,
		`{"type":"prompt","schema_version":5,"seq":2}`,
		last,
		`{"type":"verify","schema_version":99,"seq":4}`,
		`{"type":"verify",`,
	}
	torn := `{"type":"verify","schema_version":5,"seq":5`
	body := strings.Join(lines, "\n") + "\n" + torn
	if err := os.WriteFile(filepath.Join(st.RunDir(session), FileRecords), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	line, ok, skipped, err := st.LatestRecordBytes(session, TypeVerify)
	if err != nil || !ok || string(line) != last || skipped != 2 {
		t.Errorf("LatestRecordBytes = %q, ok %t, skipped %d, err %v; want %q, ok, 2 skipped (the damaged line and the torn tail)",
			line, ok, skipped, err, last)
	}
	if line, ok, _, err := st.LatestRecordBytes(session, TypeModel); err != nil || ok || line != nil {
		t.Errorf("a kind the run never wrote: %q, ok %t, err %v; want nothing", line, ok, err)
	}
	if line, ok, skipped, err := st.LatestRecordBytes("sess-none", TypeVerify); err != nil || ok || line != nil || skipped != 0 {
		t.Errorf("a run with no records file: %q, ok %t, skipped %d, err %v; want nothing", line, ok, skipped, err)
	}
}

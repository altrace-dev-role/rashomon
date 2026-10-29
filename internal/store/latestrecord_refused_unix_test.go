//go:build unix

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRG1f_ARefusedLatestRecordReadCountsAsSkipped: a records read the
// writer's lock refuses (Windows' mandatory lock; a directory's EISDIR read
// stands in for it here, as TEST-11's tests drive it) is one line skipped and
// no record, not an error that would stop a commit's receipt. Break: return
// eachLine's error as it is, and the read fails.
func TestRG1f_ARefusedLatestRecordReadCountsAsSkipped(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.readRefusal = refusesAsEISDIR
	const session = "sess-rg1f-refused"
	if err := os.MkdirAll(filepath.Join(st.RunDir(session), FileRecords), dirMode); err != nil {
		t.Fatal(err)
	}
	line, ok, skipped, err := st.LatestRecordBytes(session, TypeVerify)
	if err != nil || ok || line != nil || skipped != 1 {
		t.Errorf("a refused read: %q, ok %t, skipped %d, err %v; want no record and one skipped", line, ok, skipped, err)
	}
}

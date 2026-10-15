//go:build unix

package store

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestNextSeqRecoversThroughTheLockedHandleNotTheRecordsPath pins the
// property nextSeq's fallback (maxSeq) relies on: when the seq file is
// missing, it reads the highest existing seq through f, the caller's own
// already-open records handle -- never by reopening records.ndjson's path.
//
// That distinction is invisible on unix, where a second open of the same
// path would read the same bytes either way; it is load-bearing on Windows,
// where LockFileEx is mandatory and refuses a read through any handle but
// the one that holds the lock, including a second handle opened by the
// lock's own holder (lock_windows.go's readRefused; the Windows lock-contract
// job's TestRubricPERF7_AContendedInvocationNotesItsLongestWait is where that
// refusal showed up). This test proves the handle-vs-path distinction itself,
// on the one platform where a held lock cannot be used to force it.
//
// Proof: f is opened on dir/records.ndjson and two declarations are written
// through it directly (bypassing appendOrdered, so no seq file is ever
// created). The path is then unlinked and a new, unrelated file written in
// its place. maxSeq(f) must still report seq 2 -- the content f's descriptor
// still refers to on unix after an unlink -- not seq 9, the unrelated file
// now sitting at that path. A maxSeq that reopened the path instead of
// reading f would see the replacement and get this wrong.
func TestNextSeqRecoversThroughTheLockedHandleNotTheRecordsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileRecords)

	f := openForAppend(t, path)
	for _, seq := range []int64{1, 2} {
		line := []byte(`{"type":"declaration","schema_version":2,"seq":` +
			strconv.FormatInt(seq, 10) + `,"tool_use_id":"toolu_x","session_id":"sess"}` + "\n")
		if err := appendHealed(f, line, true); err != nil {
			t.Fatalf("seeding seq %d: %v", seq, err)
		}
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("unlinking the seeded path: %v", err)
	}
	replacement := `{"type":"declaration","schema_version":2,"seq":9,"tool_use_id":"toolu_y","session_id":"sess"}` + "\n"
	if err := os.WriteFile(path, []byte(replacement), fileMode); err != nil {
		t.Fatalf("writing the replacement file: %v", err)
	}

	got, err := maxSeq(f)
	if err != nil {
		t.Fatalf("maxSeq: %v", err)
	}
	if got != 2 {
		t.Fatalf("maxSeq(f) = %d, want 2 (f's own content) -- got the replacement file's seq "+
			"instead, which means it reopened the path rather than reading f", got)
	}
}

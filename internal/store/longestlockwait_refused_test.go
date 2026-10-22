//go:build unix

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLongestLockWait_SkipsARefusedNoteRead: a lock-wait note's read can be
// refused the same way a run's records can (Windows's mandatory lock, held
// by a concurrent noteLockWait or probe). That is not a damaged note, and
// must be skipped like one that does not parse, rather than failing the
// whole report and showing every run's wait as unknown.
func TestLongestLockWait_SkipsARefusedNoteRead(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"sess-refused", "sess-normal"} {
		if err := st.AppendDeclaration(testDecl(s, "toolu_"+s)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(st.RunDir("sess-normal"), FileLockWait), []byte("42\n"), fileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(st.RunDir("sess-refused"), FileLockWait), dirMode); err != nil {
		t.Fatal(err)
	}
	st.readRefusal = refusesAsEISDIR

	longest, in, err := st.LongestLockWait()
	if err != nil {
		t.Fatalf("LongestLockWait with one note's read refused: %v, want the refusal skipped", err)
	}
	if longest != 42*time.Millisecond || in != segment("sess-normal") {
		t.Errorf("LongestLockWait = %v in %q, want 42ms in %q", longest, in, segment("sess-normal"))
	}
}

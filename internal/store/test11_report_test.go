//go:build unix || windows

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRubricTEST11_TheForgottenHostReadWaitsOutAHeldGapsLock: report reads
// the forgotten hosts strictly, because an unread gaps file must not read as
// "nothing was forgotten". Read unlocked, that strict read failed the whole
// report on Windows whenever a forget or an eviction held the gaps lock,
// whose mandatory lock refuses it. It now waits for the lock and reads
// under it, so a hold that ends inside its budget costs a wait, not the
// report -- and on unix the read sees the gap the holder was writing,
// rather than the file as it was before.
func TestRubricTEST11_TheForgottenHostReadWaitsOutAHeldGapsLock(t *testing.T) {
	root := t.TempDir()
	st, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	holder := openForAppend(t, filepath.Join(root, FileGaps))
	unlock, err := lockFile(holder, lockBudget)
	if err != nil {
		t.Fatalf("taking the gaps lock: %v", err)
	}

	type result struct {
		forgotten func(string) bool
		err       error
	}
	done := make(chan result, 1)
	go func() {
		f, err := st.ForgottenHost()
		done <- result{f, err}
	}()

	const host = "forgotten.example"
	time.Sleep(20 * time.Millisecond)
	g := NewGap(GapForgetHost, "sess-forget", time.Now())
	g.HostDigest = st.HostDigest(host)
	line, err := marshalLine(g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Write(line); err != nil {
		t.Fatal(err)
	}
	unlock()

	r := <-done
	if r.err != nil {
		t.Fatalf("ForgottenHost after a gaps hold that ended inside its budget: %v", r.err)
	}
	if !r.forgotten(host) {
		t.Errorf("ForgottenHost did not see the host forgotten under the lock it waited for: it read past the hold")
	}
}

// TestRubricTEST11_TheReportReadCountsARefusalAsSkipped: report reads every
// run unlocked, and on Windows a hook holding a run's records lock refused
// that read, which failed the report. The report's read counts a refusal as
// one line it could not read, so the session reads unverified rather than
// the report failing; ReadRunDir, which the end probe and eviction rely on
// to fail on any read error, still does.
func TestRubricTEST11_TheReportReadCountsARefusalAsSkipped(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const session = "sess-report"
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	release := refuseRecordsRead(t, st, filepath.Join(dir, FileRecords))
	defer release()

	run, err := st.ReadRunDirForReport(st.DirName(session))
	if err != nil {
		t.Fatalf("ReadRunDirForReport with its records read refused: %v, want the refusal counted", err)
	}
	if run.Skipped != 1 {
		t.Errorf("run has %d skipped lines, want the refused records file counted as one", run.Skipped)
	}
	if _, err := st.ReadRunDir(st.DirName(session)); err == nil {
		t.Error("ReadRunDir with its records read refused returned no error, want the read error")
	}
}

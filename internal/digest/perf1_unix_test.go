//go:build unix

package digest

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// hookHold is the longest a hook's ordered append held the records lock in
// BenchmarkOrderedAppendHold's 64-way deferred runs on the reference machine
// (p99 5.8 to 9.4 ms over three runs), rounded down to the millisecond.
const hookHold = 9 * time.Millisecond

// TestRubricPERF1_DigestWaitsOutAHookHold: digest reads each file under a
// short, best-effort lock, and gives up on it after readLockBudget. That
// budget was 5 ms, set when an append's hold was taken to be sub-millisecond;
// a hook's append now measures up to 9.4 ms at p99 on darwin, so the reader
// gave up mid-hold and read past the writer, where a line caught mid-write
// reads as skipped and the turn as unverified. It now waits out a hold of
// that length and reads under the lock.
func TestRubricPERF1_DigestWaitsOutAHookHold(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-hold"
	records := filepath.Join(st.RunDir(session), store.FileRecords)
	if err := os.MkdirAll(filepath.Dir(records), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(records, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	released := safe.Go(func() {
		time.Sleep(hookHold)
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
			t.Error(err)
		}
	}, func(err error) { t.Error(err) })

	if _, err := st.ReadRunConsistent(session, readLockBudget); err != nil {
		t.Fatal(err)
	}
	waited := time.Since(start)
	<-released
	if waited < hookHold {
		t.Errorf("digest's read gave up on the lock after %v, inside a %v hook hold: readLockBudget %v is shorter than the hold it has to wait out",
			waited, hookHold, readLockBudget)
	}
}

//go:build unix

package digest

import (
	"encoding/json"
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

// loadedHold is a hold past every quiet p99 the benchmark measured -- 9.4 ms
// for a hook's deferred append, 14.1 ms for one that syncs in full inside
// the lock, which is every append on Linux and Windows -- and inside the
// tail it measured while other processes loaded the disk (p99s of 7.7 to
// 41 ms). store/lockbudget.go has the figures.
const loadedHold = 20 * time.Millisecond

// TestRubricPERF1_DigestWaitsOutAHookHold: digest reads each file under a
// short, best-effort lock, and gives up on it after readLockBudget. That
// budget was 5 ms, set when an append's hold was taken to be sub-millisecond;
// a hook's append now measures up to 9.4 ms at p99 on darwin, so the reader
// gave up mid-hold and read past the writer, where a line caught mid-write
// reads as skipped and the turn as unverified. It now waits out a hold of
// that length and reads under the lock.
//
// The hold is a real one: the holder writes the first half of a declaration,
// waits hookHold, writes the rest, and unlocks, as an append caught mid-write
// would. Waiting is not enough on its own -- a reader that slept and then read
// without the lock would wait as long -- so the test also asks that the read
// saw the whole record and no torn line, which only a read made after the
// writer let go can.
func TestRubricPERF1_DigestWaitsOutAHookHold(t *testing.T) {
	digestWaitsOut(t, hookHold)
}

// TestRubricPERF1_DigestWaitsOutALoadedHold: the 10 ms budget, per file, gave
// up inside any hold longer than its polling let it see (15.8 ms on darwin),
// so a digest read past a writer whenever the disk was busy and counted its
// line as skipped. The budget is now one deadline for the whole read, long
// enough to wait this hold out, and costs no more than three per-file
// budgets did.
func TestRubricPERF1_DigestWaitsOutALoadedHold(t *testing.T) {
	digestWaitsOut(t, loadedHold)
}

// digestWaitsOut holds the records lock for hold with half a declaration
// written, and asks that digest's read waited for the writer and saw the
// record whole.
func digestWaitsOut(t *testing.T, hold time.Duration) {
	t.Helper()
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
	line, err := json.Marshal(store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
		Seq: 1, SessionID: session, ToolUseID: "toolu_held", ToolName: "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	line = append(line, '\n')
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	half := len(line) / 2
	if _, err := f.Write(line[:half]); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	released := safe.Go(func() {
		time.Sleep(hold)
		if _, err := f.Write(line[half:]); err != nil {
			t.Error(err)
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
			t.Error(err)
		}
	}, func(err error) { t.Error(err) })

	run, err := st.ReadRunConsistent(session, readLockBudget)
	if err != nil {
		t.Fatal(err)
	}
	waited := time.Since(start)
	<-released
	if waited < hold {
		t.Errorf("digest's read gave up on the lock after %v, inside a %v hold: readLockBudget %v is shorter than the hold it has to wait out",
			waited, hold, readLockBudget)
	}
	if len(run.Declarations) != 1 || run.Skipped != 0 {
		t.Errorf("the read saw %d declarations and %d skipped lines, want the held record whole: it read before the writer let go",
			len(run.Declarations), run.Skipped)
	}
}

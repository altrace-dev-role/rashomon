package recap

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRubricCONC2_ConcurrentWritersNeverCorruptTheState runs two writers of
// different-sized states against one recap.json while a reader reads it. Two
// recap processes can overlap -- a Stop and the next prompt's catch-up -- and
// with a temp path shared between them one writer truncated the inode the
// other had just renamed into place: the file read as half of one state, and
// the catch-up line for a refused turn was lost with it. Every read must
// decode, and no writer's temp file may outlive the writes.
func TestRubricCONC2_ConcurrentWritersNeverCorruptTheState(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	path := statePath(root)

	big, small := newState(), newState()
	for i := 0; i < maxTrackedSessions; i++ {
		big.Spoken[fmt.Sprintf("session-%03d-%s", i, strings.Repeat("x", 40))] = "prompt"
	}
	small.Spoken["s"] = "p"

	const rounds = 300
	var wg sync.WaitGroup
	errs := make(chan error, 3*rounds)
	writer := func(s state) {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if err := writeState(path, s, ""); err != nil {
				errs <- fmt.Errorf("writeState: %w", err)
				return
			}
		}
	}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := readState(path); err != nil {
				errs <- fmt.Errorf("readState saw a corrupt file: %w", err)
				return
			}
		}
	}()
	wg.Add(2)
	go writer(big)
	go writer(small)
	wg.Wait()
	close(stop)
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "install.json" && e.Name() != StateFileName {
			t.Errorf("%s left behind after every write returned", e.Name())
		}
	}
}

// TestRubricCONC2_AKilledWritesTempFileIsClearedByTheNext holds writeState
// to the rest of its own doc comment: a process killed between CreateTemp
// and the rename leaves its temp file behind, since the deferred Remove
// that cleans up a normal return never runs on SIGKILL. The next write must
// clear a temp file old enough to have been orphaned that way, and must
// leave alone one too young to have been -- a second writer's own temp
// file, still in flight.
func TestRubricCONC2_AKilledWritesTempFileIsClearedByTheNext(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	path := statePath(root)
	dir := filepath.Dir(path)

	stale := filepath.Join(dir, StateTempPrefix+"orphan")
	if err := os.WriteFile(stale, []byte("half-written"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleAt := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(stale, staleAt, staleAt); err != nil {
		t.Fatal(err)
	}

	live := filepath.Join(dir, StateTempPrefix+"live")
	if err := os.WriteFile(live, []byte("still being written"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeState(path, newState(), ""); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a temp file orphaned by a killed write survived the next one; stat err = %v", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("a concurrent writer's own live temp file was removed: %v", err)
	}
}

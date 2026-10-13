package recap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
			if err := writeState(path, s); err != nil {
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
		if e.Name() != "install.json" && e.Name() != stateFileName {
			t.Errorf("%s left behind after every write returned", e.Name())
		}
	}
}

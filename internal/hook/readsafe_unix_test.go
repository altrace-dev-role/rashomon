//go:build unix

package hook

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/metrics"
	"syscall"
	"testing"
	"time"
)

// The rows of card READ-SAFE1a for the edit hooks: the file an edit names
// is a path a clone controls, and the Pre and Post hooks read it for its
// overlay, so a named pipe, a link to a device or a file over
// MaxOverlayContentBytes must be refused without holding the hook.

// readSafeChildEnv names the test a child process of this binary runs, and
// readSafeArgEnv the path it runs the call on.
const (
	readSafeChildEnv = "RASHOMON_READSAFE_CHILD"
	readSafeArgEnv   = "RASHOMON_READSAFE_ARG"
)

// readSafeTripBytes is the heap a child may hold before it stops itself:
// a read that passes it is reading without a bound. readSafeChildWait is
// how long the parent waits before it kills the child.
const (
	readSafeTripBytes = 128 << 20
	readSafeChildWait = 10 * time.Second
)

// The child's exit codes: the call returned, the heap passed
// readSafeTripBytes, or the call panicked. None is 0: a child whose
// -test.run pattern matches no test exits 0 as well, so 0 could not tell a
// returned call from a row that never ran.
const (
	readSafeExitReturned = 7
	readSafeExitTripped  = 3
	readSafeExitPanicked = 4
)

// readSafeChild is a no-op in the parent. In the child that
// readSafeChildExit starts for t, it runs call on the path the parent
// passed, under a goroutine that exits readSafeExitTripped once the heap
// passes readSafeTripBytes, and exits with what became of the call; it
// never returns there. A read without a bound stops the child, never this
// process, so a row cannot take the machine's memory or its swap.
//
// The same helper is in internal/graph/readsafe_unix_test.go and
// pgraph/cmd/pgraph/readsafe_unix_test.go: a test file cannot be imported,
// and the pgraph module cannot import this one.
func readSafeChild(t *testing.T, call func(path string)) {
	if os.Getenv(readSafeChildEnv) != t.Name() {
		return
	}
	go func() {
		s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
		for {
			metrics.Read(s)
			if s[0].Value.Uint64() > readSafeTripBytes {
				os.Exit(readSafeExitTripped)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	defer func() {
		if recover() != nil {
			os.Exit(readSafeExitPanicked)
		}
	}()
	call(os.Getenv(readSafeArgEnv))
	os.Exit(readSafeExitReturned)
}

// readSafeChildExit runs t again in a child process with path as the
// call's argument and returns the child's exit code. A child still running
// after readSafeChildWait is killed and fails t: its row is unread, since a
// kill cannot tell a read that grows from one that is slow.
func readSafeChildExit(t *testing.T, path string) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), readSafeChildEnv+"="+t.Name(), readSafeArgEnv+"="+path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		if err == nil {
			t.Fatalf("child exit 0: it ran no row (the name %q matched no test)", t.Name())
		}
		t.Fatalf("child: %v", err)
	case <-time.After(readSafeChildWait):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("the child ran past %v and was killed: the row is unread", readSafeChildWait)
	}
	return -1
}

// readSafeReturnsInside makes path a named pipe with no writer, runs call
// on it, and fails t unless the call returns inside two seconds. Its
// cleanup opens the writer end, so a call still waiting in its open
// returns and ends.
func readSafeReturnsInside(t *testing.T, path string, call func(path string)) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
	})
	done := make(chan struct{})
	go func() { defer close(done); call(path) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: still waiting after 2s on a named pipe with no writer", filepath.Base(path))
	}
}

// TestRS1a_R9_OverlayReadNamedPipeReturnsAtOnce: an edited path that is a
// named pipe with no writer must not hold the edit's hooks.
func TestRS1a_R9_OverlayReadNamedPipeReturnsAtOnce(t *testing.T) {
	readSafeReturnsInside(t, filepath.Join(t.TempDir(), "edited"), func(p string) { _, _ = readOverlayFile(p) })
}

// TestRS1a_R10_OverlayReadLinkToDevZeroIsRefused: an edited path linked to
// /dev/zero is refused as not regular, with the heap far under the
// tripwire. The read runs in a child process.
func TestRS1a_R10_OverlayReadLinkToDevZeroIsRefused(t *testing.T) {
	readSafeChild(t, func(p string) { _, _ = readOverlayFile(p) })
	link := filepath.Join(t.TempDir(), "edited")
	if err := os.Symlink("/dev/zero", link); err != nil {
		t.Fatal(err)
	}
	switch code := readSafeChildExit(t, link); code {
	case readSafeExitReturned:
	case readSafeExitTripped:
		t.Fatalf("the overlay read of a path linked to /dev/zero passed %d MiB of heap: it has no bound", readSafeTripBytes>>20)
	default:
		t.Fatalf("child exit %d", code)
	}
}

// TestRS1a_R11_OverlayReadOverTheCapIsRefused: a file one byte over
// MaxOverlayContentBytes is refused by its size. The file is sparse.
func TestRS1a_R11_OverlayReadOverTheCapIsRefused(t *testing.T) {
	p := filepath.Join(t.TempDir(), "edited")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxOverlayContentBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if b, err := readOverlayFile(p); err == nil {
		t.Fatalf("readOverlayFile returned %d bytes of a file over the %d byte cap", len(b), MaxOverlayContentBytes)
	}
}

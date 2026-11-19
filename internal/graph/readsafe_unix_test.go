//go:build unix

package graph

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/metrics"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The rows of card READ-SAFE1a for the repository index: the index file a
// prompt reads (GraphFile, at the repository or up to MaxIndexDepth parents
// above it) is a path a clone controls, so a named pipe, a link to a device
// or a file too large to decode on every prompt must be refused.

// readSafeIndexCap is the size over which an index is refused: the cap
// LoadFile holds, read by its name.
const readSafeIndexCap = MaxGraphFileBytes

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
// The same helper is in internal/hook/readsafe_unix_test.go and
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

// readSafeIndexPath is GraphFile under a fresh root, its directory made.
func readSafeIndexPath(t *testing.T) (root, path string) {
	t.Helper()
	root = t.TempDir()
	path = filepath.Join(root, filepath.FromSlash(GraphFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, path
}

// TestRS1a_R4_LoadFileNamedPipeReturnsAtOnce: an index that is a named
// pipe with no writer must not hold the prompt.
func TestRS1a_R4_LoadFileNamedPipeReturnsAtOnce(t *testing.T) {
	root, path := readSafeIndexPath(t)
	readSafeReturnsInside(t, path, func(string) { _, _, _ = LoadFile(root) })
}

// TestRS1a_R5_LoadFileLinkToDevZeroIsBounded: an index committed as a link
// to /dev/zero ends in an error with the heap far under the tripwire. The
// read runs in a child process.
func TestRS1a_R5_LoadFileLinkToDevZeroIsBounded(t *testing.T) {
	readSafeChild(t, func(root string) { _, _, _ = LoadFile(root) })
	root, path := readSafeIndexPath(t)
	if err := os.Symlink("/dev/zero", path); err != nil {
		t.Fatal(err)
	}
	switch code := readSafeChildExit(t, root); code {
	case readSafeExitReturned:
	case readSafeExitTripped:
		t.Fatalf("LoadFile of an index linked to /dev/zero passed %d MiB of heap: it has no bound", readSafeTripBytes>>20)
	default:
		t.Fatalf("child exit %d", code)
	}
}

// TestRS1a_R6_LoadFileOverTheCapIsRefused: a valid index one byte over
// readSafeIndexCap is refused, not decoded. The padding is spaces, which a
// decoder skips, so the file decodes whole where nothing refuses it; zeros
// would fail at the first byte for another reason.
func TestRS1a_R6_LoadFileOverTheCapIsRefused(t *testing.T) {
	root, path := readSafeIndexPath(t)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	head := `{"schema":1}` + "\n"
	if _, err := w.WriteString(head); err != nil {
		t.Fatal(err)
	}
	pad := strings.Repeat(" ", 1<<20)
	for left := readSafeIndexCap + 1 - len(head); left > 0; left -= len(pad) {
		if _, err := w.WriteString(pad[:min(left, len(pad))]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() != readSafeIndexCap+1 {
		t.Fatalf("fixture size: %v, %v", fi, err)
	}
	if _, _, err := LoadFile(root); err == nil {
		t.Fatalf("LoadFile decoded an index of %d bytes, over the %d byte cap", readSafeIndexCap+1, readSafeIndexCap)
	}
}

// TestRS1a_R7_ReadIndexHeaderNamedPipeReturnsAtOnce: the overlay writers'
// header read must not wait on a named pipe either.
func TestRS1a_R7_ReadIndexHeaderNamedPipeReturnsAtOnce(t *testing.T) {
	_, path := readSafeIndexPath(t)
	readSafeReturnsInside(t, path, func(p string) { _, _ = ReadIndexHeader(p) })
}

// TestRS1a_R8_ReadIndexHeaderLinkToDevZeroIsBounded: the header read of an
// index linked to /dev/zero ends in an error with the heap far under the
// tripwire. The read runs in a child process.
func TestRS1a_R8_ReadIndexHeaderLinkToDevZeroIsBounded(t *testing.T) {
	readSafeChild(t, func(p string) { _, _ = ReadIndexHeader(p) })
	_, path := readSafeIndexPath(t)
	if err := os.Symlink("/dev/zero", path); err != nil {
		t.Fatal(err)
	}
	switch code := readSafeChildExit(t, path); code {
	case readSafeExitReturned:
	case readSafeExitTripped:
		t.Fatalf("ReadIndexHeader of an index linked to /dev/zero passed %d MiB of heap: it has no bound", readSafeTripBytes>>20)
	default:
		t.Fatalf("child exit %d", code)
	}
}

// TestRS1a_R14_ReadIndexHeaderOverItsCapIsRefused: a first line longer
// than maxHeaderBytes is an error, so the header read stays one small read.
func TestRS1a_R14_ReadIndexHeaderOverItsCapIsRefused(t *testing.T) {
	_, path := readSafeIndexPath(t)
	line := `{"schema":1,"repo":"` + strings.Repeat("a", maxHeaderBytes) + `"}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if h, err := ReadIndexHeader(path); err == nil {
		t.Fatalf("ReadIndexHeader read a %d-byte first line, over the %d byte cap: repo of %d bytes", len(line), maxHeaderBytes, len(h.Repo))
	}
}

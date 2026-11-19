//go:build unix

package main

import (
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

	"github.com/altrace-dev-role/rashomon/pgraph"
)

// The rows of card READ-SAFE1a for the pgraph tool: a path a clone controls
// (.pgraph and the files in it) is read or written only when it is what the
// sync expects. A clone's checkout makes regular files, links and each
// directory of the tree, and a link may name a device or a directory
// outside the tree.

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
// internal/hook/readsafe_unix_test.go: a test file cannot be imported, and
// this module cannot import the root module.
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

// TestRS1a_R1_LogNamedPipeReturnsAtOnce: a log that is a named pipe with
// no writer must not hold the sync.
func TestRS1a_R1_LogNamedPipeReturnsAtOnce(t *testing.T) {
	readSafeReturnsInside(t, filepath.Join(t.TempDir(), pgraph.LogFile), func(p string) {
		_, _ = readLog(p, pgraph.LogFile)
	})
}

// TestRS1a_R2_LogLinkToDevZeroIsNotReadWithoutEnd: a log committed as a
// link to /dev/zero must be refused, not read until memory runs out. The
// read runs in a child process under a heap tripwire.
func TestRS1a_R2_LogLinkToDevZeroIsNotReadWithoutEnd(t *testing.T) {
	readSafeChild(t, func(p string) { _, _ = readLog(p, pgraph.LogFile) })
	link := filepath.Join(t.TempDir(), pgraph.LogFile)
	if err := os.Symlink("/dev/zero", link); err != nil {
		t.Fatal(err)
	}
	switch code := readSafeChildExit(t, link); code {
	case readSafeExitReturned:
	case readSafeExitTripped:
		t.Fatalf("the read of a log linked to /dev/zero passed %d MiB of heap: it has no bound", readSafeTripBytes>>20)
	default:
		t.Fatalf("child exit %d", code)
	}
}

// TestRS1a_R3_LogOverTheCapIsRefused: a log over pgraph.MaxLogBytes is
// refused by its size, before any byte is read. The file is sparse.
func TestRS1a_R3_LogOverTheCapIsRefused(t *testing.T) {
	p := filepath.Join(t.TempDir(), pgraph.LogFile)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(pgraph.MaxLogBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readLog(p, pgraph.LogFile); err == nil || !strings.Contains(err.Error(), "over the") {
		t.Fatalf("readLog of a log of %d bytes: %v, want a refusal over the cap", pgraph.MaxLogBytes+1, err)
	}
}

// TestRS1a_R12_StateDirLinkedOutsideIsNotWritten: a .pgraph committed as a
// link to a directory outside the tree must not lead the sync to write,
// rename or remove anything there.
func TestRS1a_R12_StateDirLinkedOutsideIsNotWritten(t *testing.T) {
	root := module(t, smallModule)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, pgraph.StateDir)); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := sync(root)
	ents, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		names := make([]string, len(ents))
		for i, e := range ents {
			names[i] = e.Name()
		}
		t.Fatalf("sync (exit %d, stderr %q) wrote %v into the directory .pgraph links to", code, stderr, names)
	}
}

// TestRS1a_R13_FixedNameLinkedOutsideKeepsItsTarget: inside a real .pgraph
// directory, a fixed name committed as a link to a file outside the tree
// is replaced by the sync, and the file it named is left byte-equal.
// pgraph.symbols is the name, since the sync writes it and reads it at no
// point before; a link at the log would test the log's read instead.
func TestRS1a_R13_FixedNameLinkedOutsideKeepsItsTarget(t *testing.T) {
	root := module(t, smallModule)
	if err := os.MkdirAll(filepath.Join(root, pgraph.StateDir), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside")
	write(t, target, "bytes outside the tree\n")
	before := sha(read(t, target))
	if err := os.Symlink(target, filepath.Join(root, pgraph.StateDir, pgraph.SymbolsFile)); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := sync(root); code != 0 {
		t.Fatalf("sync exit %d, stderr %q: want 0, so that its writes ran", code, stderr)
	}
	if after := sha(read(t, target)); after != before {
		t.Fatalf("the file outside the tree changed: sha256 %s, was %s", after, before)
	}
}

// TestRS1a_R20_SyncFilesReadMode0644: the four files a sync writes read
// mode 0644, set on the temp before its rename. That mode is the only sign
// of where it was set: by path after Close on base (main.go:375), which
// follows a link raced in on the temp name, and on the file handle before
// Close after READ-SAFE1a; a mutant that drops the handle's Chmod leaves
// the exclusive create's 0600 and fails here.
func TestRS1a_R20_SyncFilesReadMode0644(t *testing.T) {
	root := module(t, smallModule)
	if code, _, stderr := sync(root); code != 0 {
		t.Fatalf("sync exit %d, stderr %q: want 0, so that its writes ran", code, stderr)
	}
	for _, name := range []string{pgraph.LogFile, pgraph.DigestsFile, pgraph.SymbolsFile, walkFile} {
		fi, err := os.Lstat(filepath.Join(root, pgraph.StateDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if m := fi.Mode(); m != 0o644 {
			t.Errorf("%s: mode %v, want -rw-r--r--", name, m)
		}
	}
}

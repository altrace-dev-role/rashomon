//go:build unix

package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// fifoAt makes a named pipe at path. Its cleanup opens the write end without
// blocking, which lets a read still waiting in the pipe's open go on to end
// of file, so a red run leaves no goroutine parked in open(2).
func fifoAt(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", path, err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
}

// returnsWithin runs read and fails the test if it has not returned within
// d: an open of a named pipe with no writer waits for one, the hang these
// rows guard against. read must not call t; its results are read only after
// it returns.
func returnsWithin(t *testing.T, d time.Duration, read func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { read(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("the read had not returned after %v: it waits on the named pipe for a writer", d)
	}
}

// A named pipe at config.json is a file that is there and does not read, so
// it is unreadable at once: the hook clamps on it and `steer on --repair`
// replaces it, as for one that does not parse. Before STORE-READ-REGULAR1
// the open waited for a writer, and the Stop hook with it, every turn.
func TestSRR1_AFIFOConfigReadsUnreadableAtOnce(t *testing.T) {
	root := t.TempDir()
	fifoAt(t, filepath.Join(root, FileConfig))
	var err error
	returnsWithin(t, 2*time.Second, func() { _, err = ReadConfig(root) })
	if !errors.Is(err, ErrConfigUnreadable) {
		t.Errorf("ReadConfig on a named pipe = %v, want ErrConfigUnreadable", err)
	}
}

// A named pipe at a run's records.ndjson ends the tail read at once with an
// error: LatestRecordTail runs on the SessionStart path (receipt's previous
// session line).
func TestSRR1_AFIFORecordsFileEndsTheTailReadAtOnce(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.RunDir("s1"), 0o700); err != nil {
		t.Fatal(err)
	}
	fifoAt(t, filepath.Join(st.RunDir("s1"), FileRecords))
	var ok bool
	var tailErr error
	returnsWithin(t, 2*time.Second, func() { _, ok, _, tailErr = st.LatestRecordTail("s1", TypeVerify, 4096) })
	if ok || tailErr == nil {
		t.Errorf("LatestRecordTail on a named pipe = ok %v, err %v; want ok false and an error", ok, tailErr)
	}
}

// The same for the walk over a run's records of one kind.
func TestSRR1_AFIFORecordsFileEndsTheKindWalkAtOnce(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.RunDir("s1"), 0o700); err != nil {
		t.Fatal(err)
	}
	fifoAt(t, filepath.Join(st.RunDir("s1"), FileRecords))
	var called bool
	var eachErr error
	returnsWithin(t, 2*time.Second, func() {
		_, eachErr = st.EachRecordOfKind("s1", TypeVerify, func([]byte) bool { called = true; return true })
	})
	if called || eachErr == nil {
		t.Errorf("EachRecordOfKind on a named pipe = called %v, err %v; want no line and an error", called, eachErr)
	}
}

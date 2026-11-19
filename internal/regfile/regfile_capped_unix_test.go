//go:build unix

package regfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The rows of the capped reader (card READ-SAFE1a, condition c3) that need
// a named pipe, a device or a link; regfile_capped_test.go has the others.
// The pgraph module's form (pgraph/cmd/pgraph, readCapped) refuses a link;
// this one follows it, so the last row here has no twin there.

// cappedWithin runs call on a named pipe with no writer and fails t unless
// it returns inside two seconds with ErrNotRegular. Its cleanup opens the
// writer end, so a call still waiting in its open returns.
func cappedWithin(t *testing.T, what string, call func(path string) error) {
	t.Helper()
	pipe := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(pipe, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
	var err error
	done := make(chan struct{})
	go func() { err = call(pipe); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s had not returned after 2s on a named pipe with no writer", what)
	}
	if !errors.Is(err, ErrNotRegular) {
		t.Errorf("%s(a named pipe) = %v, want ErrNotRegular", what, err)
	}
}

// TestRS1a_C3_CappedReadersRefuseANamedPipeAtOnce: neither reader waits in
// its open for a writer.
func TestRS1a_C3_CappedReadersRefuseANamedPipeAtOnce(t *testing.T) {
	cappedWithin(t, "ReadCapped", func(p string) error {
		b, err := ReadCapped(p, 1<<20)
		if b != nil {
			t.Errorf("ReadCapped returned %d bytes of a named pipe", len(b))
		}
		return err
	})
	cappedWithin(t, "OpenCapped", func(p string) error {
		rc, err := OpenCapped(p, 1<<20)
		if rc != nil {
			rc.Close() //nolint:errcheck // read-only
			t.Errorf("OpenCapped returned a reader of a named pipe")
		}
		return err
	})
}

// TestRS1a_C3_CappedReadersRefuseALinkToADevice: a link to /dev/zero is
// followed and refused as a character device, before any read. The cap is
// small, so a reader that read the device anyway stops at it.
func TestRS1a_C3_CappedReadersRefuseALinkToADevice(t *testing.T) {
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink("/dev/zero", link); err != nil {
		t.Fatal(err)
	}
	if b, err := ReadCapped(link, 1024); b != nil || !errors.Is(err, ErrNotRegular) {
		t.Errorf("ReadCapped(a link to /dev/zero) = %d bytes, %v; want ErrNotRegular", len(b), err)
	}
	rc, err := OpenCapped(link, 1024)
	if rc != nil {
		rc.Close() //nolint:errcheck // read-only
	}
	if rc != nil || !errors.Is(err, ErrNotRegular) {
		t.Errorf("OpenCapped(a link to /dev/zero) = %v, %v; want ErrNotRegular", rc, err)
	}
}

// TestRS1a_C3_CappedReadersFollowALinkToARegularFile: a link to a regular
// file reads as the file, as Open reads it. This is where the two forms
// differ: pgraph's readCapped refuses any link.
func TestRS1a_C3_CappedReadersFollowALinkToARegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if b, err := ReadCapped(link, 3); err != nil || string(b) != "abc" {
		t.Errorf("ReadCapped(a link to a 3 byte file) = %q, %v; want \"abc\"", b, err)
	}
	rc, err := OpenCapped(link, 3)
	if err != nil {
		t.Fatalf("OpenCapped(a link to a 3 byte file) = %v", err)
	}
	defer rc.Close() //nolint:errcheck // read-only
	if b, err := io.ReadAll(rc); err != nil || string(b) != "abc" {
		t.Errorf("OpenCapped's reader = %q, %v; want \"abc\"", b, err)
	}
}

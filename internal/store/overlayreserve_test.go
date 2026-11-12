package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func reserveStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-ov"
	if _, err := st.DeclareSeq(testDecl(session, "toolu_ov")); err != nil {
		t.Fatal(err)
	}
	return st, session, filepath.Join(st.RunDir(session), FileOverlayBytes)
}

func counter(t *testing.T, path string) int64 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestStore_ReserveOverlayConcurrentSumsExactly: sixteen goroutines, each on
// a Store of its own as sixteen hooks are, reserve k bytes on one session; all
// succeed and the counter reads exactly 16k. A read-add-write outside the
// lock would lose updates.
func TestStore_ReserveOverlayConcurrentSumsExactly(t *testing.T) {
	st, session, path := reserveStore(t)
	const k, ways = 1000, 16
	var wg sync.WaitGroup
	errs := make([]error, ways)
	for w := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			one, err := Open(st.Root())
			if err == nil {
				err = one.ReserveOverlay(session, k)
			}
			errs[w] = err
		}()
	}
	wg.Wait()
	for w, err := range errs {
		if err != nil {
			t.Errorf("reservation %d: %v", w, err)
		}
	}
	if got := counter(t, path); got != ways*k {
		t.Errorf("the counter reads %d, want %d", got, ways*k)
	}
}

// TestStore_ReserveOverlayPastMaxRefuses: a reserve that would pass the bound
// refuses with ErrOverlayFull and leaves the counter as it was; one that lands
// exactly on it is granted. An unreadable counter refuses too.
func TestStore_ReserveOverlayPastMaxRefuses(t *testing.T) {
	st, session, path := reserveStore(t)
	if err := st.ReserveOverlay(session, MaxOverlayBytes-10); err != nil {
		t.Fatal(err)
	}
	if err := st.ReserveOverlay(session, 11); !errors.Is(err, ErrOverlayFull) {
		t.Fatalf("11 bytes with 10 left: %v, want ErrOverlayFull", err)
	}
	if got := counter(t, path); got != MaxOverlayBytes-10 {
		t.Errorf("a refused reserve moved the counter to %d", got)
	}
	if err := st.ReserveOverlay(session, 10); err != nil {
		t.Errorf("the last 10 bytes: %v, want granted", err)
	}
	if err := st.ReserveOverlay(session, 1); !errors.Is(err, ErrOverlayFull) {
		t.Errorf("one past the bound: %v, want ErrOverlayFull", err)
	}
	if got := counter(t, path); got != MaxOverlayBytes {
		t.Errorf("the counter reads %d, want the bound %d", got, MaxOverlayBytes)
	}

	st2, session2, path2 := reserveStore(t)
	if err := os.Mkdir(path2, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := st2.ReserveOverlay(session2, 1); err == nil {
		t.Error("an unreadable counter was granted")
	}
}

// TestStore_OverlayFullRecordedOnce: two refusals leave one marker, and
// OverlayFull reads true; before the first it reads false.
func TestStore_OverlayFullRecordedOnce(t *testing.T) {
	st, session, _ := reserveStore(t)
	if st.OverlayFull(session) {
		t.Fatal("OverlayFull before any refusal")
	}
	if err := st.ReserveOverlay(session, MaxOverlayBytes+1); !errors.Is(err, ErrOverlayFull) {
		t.Fatalf("first refusal: %v", err)
	}
	if err := st.ReserveOverlay(session, MaxOverlayBytes+1); !errors.Is(err, ErrOverlayFull) {
		t.Fatalf("second refusal: %v, want ErrOverlayFull (the marker's EEXIST is no error)", err)
	}
	if !st.OverlayFull(session) {
		t.Error("OverlayFull false after a refusal")
	}
	ents, err := os.ReadDir(st.RunDir(session))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ents {
		if e.Name() == FileOverlayFull {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d overlay-full markers, want 1", n)
	}
}

// TestStore_ReserveOverlayCorruptCounterIsNotFull (C1): an unreadable counter,
// then a malformed one, each refuse with ErrOverlayCounter and never
// ErrOverlayFull; no overlay-full marker is recorded, and the counter's bytes
// are as they were. A corrupt counter is not a full journal.
func TestStore_ReserveOverlayCorruptCounterIsNotFull(t *testing.T) {
	st, session, path := reserveStore(t)
	check := func(label string) {
		t.Helper()
		err := st.ReserveOverlay(session, 1)
		if !errors.Is(err, ErrOverlayCounter) || errors.Is(err, ErrOverlayFull) {
			t.Errorf("%s: %v, want ErrOverlayCounter and not ErrOverlayFull", label, err)
		}
		if st.OverlayFull(session) {
			t.Errorf("%s: overlay_journal_full was recorded for a corrupt counter", label)
		}
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	check("unreadable")
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		t.Errorf("the unreadable counter changed: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"not a number\n", "-5\n", "99999999999999999999999\n", ""} {
		if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		check("malformed " + strconv.Quote(bad))
		if b, _ := os.ReadFile(path); !bytes.Equal(b, []byte(bad)) {
			t.Errorf("malformed %q: the counter became %q", bad, b)
		}
	}
}

// TestStore_ReserveOverlayMissingCounterReadsZero: no counter file is zero
// bytes used.
func TestStore_ReserveOverlayMissingCounterReadsZero(t *testing.T) {
	st, session, path := reserveStore(t)
	if err := st.ReserveOverlay(session, 7); err != nil {
		t.Fatal(err)
	}
	if got := counter(t, path); got != 7 {
		t.Errorf("counter %d, want 7", got)
	}
}

// TestStore_OverlayNoConsentIsAnAppendedByteTally (H4): the count is the
// file's length, one byte per note, zero for a session never noted.
func TestStore_OverlayNoConsentIsAnAppendedByteTally(t *testing.T) {
	st, session, path := reserveStore(t)
	if n := st.OverlayNoConsent(session); n != 0 {
		t.Fatalf("count %d before any note", n)
	}
	for i := 1; i <= 3; i++ {
		st.NoteOverlayNoConsent(session)
		if n := st.OverlayNoConsent(session); n != int64(i) {
			t.Fatalf("count %d after %d notes", n, i)
		}
	}
	if fi, err := os.Stat(filepath.Join(filepath.Dir(path), FileOverlayNoConsent)); err != nil || fi.Size() != 3 {
		t.Errorf("tally file: %v, %v", fi, err)
	}
}

// TestStore_SessionContentMarker: only "1" is consent; absent, "0" and junk
// are not.
func TestStore_SessionContentMarker(t *testing.T) {
	root := t.TempDir()
	if SessionContentMarker(root) {
		t.Error("absent marker read as consent")
	}
	for content, want := range map[string]bool{"1\n": true, "1": true, "0\n": false, "": false, "yes": false} {
		if err := os.WriteFile(filepath.Join(root, FileSessionContent), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := SessionContentMarker(root); got != want {
			t.Errorf("marker %q: %v, want %v", content, got, want)
		}
	}
}

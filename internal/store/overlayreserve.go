package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The overlay's per-run files, in the run directory beside FileSeq. Both
// are notes about the run, like probe and seq, and not records.
const (
	// FileOverlayBytes holds, as decimal bytes, how much overlay the run has
	// reserved (ReserveOverlay). A missing file reads 0.
	FileOverlayBytes = "overlay-bytes"
	// FileOverlayFull is present once a reservation was refused for passing
	// MaxOverlayBytes: overlay_journal_full, recorded once.
	FileOverlayFull = "overlay-full"
)

// MaxOverlayBytes bounds the overlay bytes one run reserves. It equals
// graph.MaxOverlayBytes, which a test in internal/graph pins: store never
// imports graph.
const MaxOverlayBytes = 4 << 20

// overlayError is a constant error: a package-level errors.New would allocate
// at init, which the init budget (RES-2) counts, and these cost none.
type overlayError string

func (e overlayError) Error() string { return string(e) }

// ErrOverlayFull is the bound's answer: the reservation would pass
// MaxOverlayBytes, nothing was reserved, and the run is marked full.
const ErrOverlayFull overlayError = "store: the run's overlay bytes are at the bound (overlay_journal_full)"

// ErrOverlayCounter is the refusal for a counter file that cannot be trusted
// (unreadable or malformed). It is not ErrOverlayFull and records nothing: a
// corrupt counter is not a full journal, and saying "full" for it would be
// false. The bound may be over-honoured, never passed.
const ErrOverlayCounter overlayError = "store: the run's overlay counter is unreadable"

// ReserveOverlay adds n bytes to the run's overlay counter, or refuses and
// changes nothing. It is a second hold of the lock the ordered append takes,
// on the same records.ndjson handle, so concurrent hooks sum exactly: inside
// the hold it reads the counter, adds n and writes the new total, synced as
// nextSeq syncs FileSeq. A crash after the write and before the group's
// append over-counts, which the bound tolerates.
//
// Past MaxOverlayBytes it returns ErrOverlayFull and, once per run, leaves
// the FileOverlayFull marker. An unreadable or malformed counter returns
// ErrOverlayCounter. n is an upper bound on the bytes the caller will append.
func (s *Store) ReserveOverlay(sessionID string, n int64) error {
	if n < 0 {
		return fmt.Errorf("store: a negative overlay reservation (%d)", n)
	}
	if err := s.opened(); err != nil {
		return err
	}
	dir := s.RunDir(sessionID)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, FileRecords), appendFlags, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // the counter's sync reports the write failure
	unlock, err := s.lockAppend(f, s.lockWait(lockBudget))
	if err != nil {
		return err
	}
	err = s.reserveLocked(dir, n)
	unlock()
	if errors.Is(err, ErrOverlayFull) {
		markOverlayFull(dir)
	}
	return err
}

// reserveLocked is ReserveOverlay's critical section, with the records lock
// held by the caller.
func (s *Store) reserveLocked(dir string, n int64) error {
	path := filepath.Join(dir, FileOverlayBytes)
	var used int64
	switch b, err := os.ReadFile(path); {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("%w: %v", ErrOverlayCounter, err)
	default:
		used, err = strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if err != nil || used < 0 {
			return fmt.Errorf("%w: malformed", ErrOverlayCounter)
		}
	}
	if used > MaxOverlayBytes-n {
		return ErrOverlayFull
	}
	return writeSmall(path, []byte(strconv.FormatInt(used+n, 10)+"\n"), s.syncAppend)
}

// markOverlayFull creates the run's overlay-full marker. An existing one is
// the answer "already recorded"; any other failure is dropped, since the
// refusal it accompanies is already the caller's answer.
func markOverlayFull(dir string) {
	f, err := os.OpenFile(filepath.Join(dir, FileOverlayFull), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err == nil {
		_ = f.Close()
	}
}

// OverlayFull reports whether a reservation of the run was refused for
// passing MaxOverlayBytes.
func (s *Store) OverlayFull(sessionID string) bool {
	_, err := os.Stat(filepath.Join(s.RunDir(sessionID), FileOverlayFull))
	return err == nil
}

package store

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// noteLockWait records how long this Store's appends have waited for their
// locks in the session's run directory, when that is longer than any wait
// noted there before (perf-7). A Store is one hook invocation's, so the note
// is the longest any one invocation of the run waited; status shows the
// longest across runs, which is how lock contention becomes visible before
// it costs a record (a lock_timeout spill).
//
// An invocation that never waited writes nothing, so an uncontended append
// costs no more than it did. The note waits briefly for its own file's lock
// and is skipped when it cannot get it, and it is not synced: it is an
// observation of the store, not a record of the session, and a note that is
// not written leaves the longest wait shown as it was rather than making
// anything else read differently.
func (s *Store) noteLockWait(sessionID string) {
	waited := time.Duration(s.lockWaited.Load())
	if waited <= 0 || s.opened() != nil {
		return
	}
	// Rounded up, so a wait of any length reads as one.
	ms := int64((waited + time.Millisecond - 1) / time.Millisecond)

	f, err := os.OpenFile(filepath.Join(s.RunDir(sessionID), FileLockWait), os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck // an unwritten note leaves the old one standing
	unlock, _, err := lockCounted(f, s.lockWait(spillBudget))
	if err != nil {
		return
	}
	defer unlock()
	if prev, err := readLockWait(f); err == nil && prev >= ms {
		return
	}
	line := []byte(strconv.FormatInt(ms, 10) + "\n")
	if err := f.Truncate(0); err != nil {
		return
	}
	// A failed write leaves a note that does not parse, which reads as none.
	_, _ = f.WriteAt(line, 0)
}

// readLockWait parses a lock-wait note: decimal milliseconds and a newline.
func readLockWait(r io.Reader) (int64, error) {
	b, err := io.ReadAll(io.LimitReader(r, 32))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(string(bytes.TrimSpace(b)), 10, 64)
}

// LongestLockWait is the longest any one hook invocation waited for store
// locks, across every run in the store, and the run directory it was noted
// in; zero and "" when no invocation has waited, or none since the runs were
// written by a build that noted waits (perf-7). A note that does not parse
// is skipped, as a run with none. A refused read (s.refusal(), Windows's
// mandatory lock held by a concurrent noteLockWait or probe) is skipped the
// same way: it says nothing about the note's content, only that something
// else holds it right now. Any other read error is returned, because the
// longest wait could be the one not read.
func (s *Store) LongestLockWait() (time.Duration, string, error) {
	runs, err := s.Runs()
	if err != nil {
		return 0, "", err
	}
	refused := s.refusal()
	var longest int64
	var in string
	for _, name := range runs {
		f, err := os.Open(filepath.Join(s.root, dirRuns, name, FileLockWait))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			if refused(err) {
				continue
			}
			return 0, "", err
		}
		ms, perr := readLockWait(f)
		cerr := f.Close()
		if perr != nil {
			var num *strconv.NumError
			if errors.As(perr, &num) || refused(perr) {
				continue
			}
			return 0, "", perr
		}
		if cerr != nil {
			return 0, "", cerr
		}
		if ms > longest {
			longest, in = ms, name
		}
	}
	return time.Duration(longest) * time.Millisecond, in, nil
}

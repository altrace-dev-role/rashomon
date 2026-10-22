package store

import (
	"errors"
	"os"

	"github.com/altrace-dev-role/rashomon/internal/fault"
)

// appendHealed appends one record line to f, whose append lock the caller
// holds, so that the record always starts a line of its own and a failed
// write leaves no part of itself behind.
//
// A writer killed in the middle of an append (SIGKILL at a hook timeout, a
// crash before the write completed) or cut short by a full disk leaves a last
// line with no newline. An O_APPEND write after it would continue that line,
// and the healthy record and the fragment would read back as one damaged
// line: the record lost, and, where its terminal survived elsewhere, blamed
// on the lock (errors-1). So the last byte is read first, with one pread,
// and when it is not a newline one goes out ahead of the record, in the same
// write. The fragment then reads as one skipped line, counted in
// records_unreadable, and the record after it reads intact (REC-32, REC-55).
//
// A write that fails is cut back to the size the file had before it, when
// cut is set. cutBack's own size check and its truncate are two syscalls
// with a gap between them, and a best-effort writer appends without any
// lock at all (appendLine's bestEffort branch), so a cut-back can still
// land on top of a record that writer put there in that gap. The spill
// file is the only one a best-effort writer ever targets, and its callers
// pass cut=false for exactly that reason. Windows refuses the cut on an
// append-only handle regardless; the fragment then stays, and the next
// append starts a fresh line after it, as above.
func appendHealed(f *os.File, line []byte, cut bool) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if size > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], size-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			line = append([]byte{'\n'}, line...)
		}
	}
	n, err := f.Write(line)
	if err == nil {
		err = fault.Fail(fault.PointStoreAppendWrite)
	}
	if err != nil {
		return healAppendError(f, size, n, cut, err)
	}
	return nil
}

// healAppendError is appendHealed's response to a failed write of n bytes
// at size: writeErr as it is when cut is false, or writeErr joined with
// cutBack's own result when it is true.
func healAppendError(f *os.File, size int64, n int, cut bool, writeErr error) error {
	if !cut {
		return writeErr
	}
	return errors.Join(writeErr, cutBack(f, size, n))
}

// cutBack truncates f to size after a failed write of n bytes, when the file
// holds exactly those n bytes past it and nothing a second writer added.
//
// Between the Stat below and the Truncate there is nothing holding that
// true: a writer with no lock on f can append in that gap, and cutBack
// truncates its record away along with ours. cutBackSync, set only by a
// test, runs there to make that window reproducible; it is nil, and the
// call costs nothing, in every build that is not this package's own test.
func cutBack(f *os.File, size int64, n int) error {
	if n == 0 {
		return nil
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() != size+int64(n) {
		return nil
	}
	if cutBackSync != nil {
		cutBackSync()
	}
	return f.Truncate(size)
}

// cutBackSync lets a test land a write inside cutBack's Stat-to-Truncate
// gap. Nil in every other build.
var cutBackSync func()

package store

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// FileSteer is a run's steer log: one line per steer decision, delivered or
// not, in the run's own directory (Technical spec 4.2.3). Its shape is
// STEER's, steer.Line; RECORD writes it (1.2.9). It is a side file, not a
// record stream: it carries no seq, and no record kind names it (ED-1).
const FileSteer = "steer.ndjson"

// MaxSteerTextBytes caps a steer line's model_text and user_text, each, in
// raw bytes (Technical spec 1.2.9). It sits above the renderer's own
// message cap, leaving room for user_text's mirror wrapper, so a line the
// renderer made is never withheld by it.
const MaxSteerTextBytes = 2048

// ErrSteerTextWithheld reports a steer append refused because a line
// reached the writer with a text over MaxSteerTextBytes: build did not cap
// it before deciding its state. Nothing was written, and the caller prints
// nothing, so the steer log and stdout still agree (STR-10).
var ErrSteerTextWithheld = errors.New("store: a steer line reached the writer with its text over its cap")

// SteerLine is one line AppendSteer writes. steer.Line is the one kind
// there is; the store names only what its writer needs of it, because steer
// is the package that calls the writer and so cannot be imported by it.
type SteerLine interface {
	// CapText withholds model_text and user_text over MaxSteerTextBytes
	// (CapSteerText) and reports whether it withheld either. build calls it
	// before it decides the line's state; AppendSteer calls it again and
	// refuses the append if it still withholds.
	CapText() bool
}

// CapSteerText is a steer text as a line stores it: text itself, or, over
// MaxSteerTextBytes, "" with *withheld set to "oversize" (1.2.11's
// convention; the line keeps its text keys as strings, so empty stands in
// for null). It reports whether it withheld the text.
func CapSteerText(text string, withheld **string) (string, bool) {
	if len(text) <= MaxSteerTextBytes {
		return text, false
	}
	var w withholder
	*withheld = w.oversize()
	return "", true
}

// AppendSteer appends steer lines to a run's steer log, all of them under
// one hold of its lock (Technical spec 4.2.4 step 10).
//
// build runs once the lock is held and before anything is written, and
// returns the lines to append: it is where the router reads and charges its
// budget, which must be decided under the same hold as the lines it admits.
// build caps every line (SteerLine.CapText) before it decides that line's
// state or charges budget for it: a line whose text was withheld did not
// print, so it is not delivered (Technical spec 7.2.5 G2, output_gate).
// locked reads the log through the handle that holds the lock; on Windows,
// where the lock is mandatory, a read through any other handle is refused,
// even one this process opened (nextSeq reads the records the same way). build
// must not keep locked, and must not append through the Store: nothing
// holding the steer lock takes another store lock.
//
// The lock is waited for steerBudget, inside LimitLockWaits' deadline when
// one is set. On a give-up AppendSteer returns ErrLockTimeout and writes
// nothing, and there is no best-effort fallback: a note dropped beats a note
// said twice (4.2.5).
//
// Every line is checked against its cap and marshalled before any is
// written, and the lines go out in one write, which starts a line of its
// own after a torn tail and is cut back if it fails (appendHealed). A line
// whose CapText still withholds refuses the whole append with
// ErrSteerTextWithheld, and a line over MaxRecordBytes with
// ErrRecordOversize, each writing nothing. No lines, from build, writes
// nothing and owes no sync.
//
// The append is ordered inside the lock like every other (syncAppend): on a
// deferred Store the one full sync is Flush's, which the caller issues
// after its last append, outside the lock, and before it writes to stdout
// (REC-34, REC-42); an append whose caller then prints nothing does not need
// it. An error from the sync or barrier comes after the write: the lines
// may be in the log, not yet ordered. Any other error means none were
// written. On every error the caller prints nothing (1.2.9).
func (s *Store) AppendSteer(sessionID string, build func(locked io.ReaderAt) ([]SteerLine, error)) error {
	if err := s.opened(); err != nil {
		return err
	}
	dir := s.RunDir(sessionID)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, FileSteer), appendFlags, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // the sync below reports the write failure

	unlock, err := s.lockAppend(f, s.lockWait(steerBudget))
	if err != nil {
		return err
	}
	defer unlock()
	lines, err := build(f)
	if err != nil {
		return err
	}
	batch, err := marshalSteer(lines)
	if err != nil || len(batch) == 0 {
		return err
	}
	if err := appendHealed(f, batch, true); err != nil {
		return err
	}
	return s.syncAppend(f)
}

// marshalSteer is lines as one NDJSON batch. A line whose text is still
// over its cap, or a line over MaxRecordBytes, refuses them all.
func marshalSteer(lines []SteerLine) ([]byte, error) {
	var batch []byte
	for _, l := range lines {
		if l.CapText() {
			return nil, ErrSteerTextWithheld
		}
		line, err := marshalLine(l)
		if err != nil {
			return nil, err
		}
		batch = append(batch, line...)
	}
	return batch, nil
}

// ReadSteer returns the lines of a run's steer log, each one JSON object,
// and how many lines it could not read: a torn tail, a damaged line, or,
// where locks are mandatory (Windows), a read the steer lock refused, which
// counts as one. A run with no steer log has no lines. Decoding a line is
// its reader's: the shape is steer.Line.
//
// The log is read unlocked, as the run's records are (ReadRunDir): its
// readers read after the fact, and a line caught mid-write counts in
// skipped. A reader inside AppendSteer's hold reads through build's handle.
func (s *Store) ReadSteer(sessionID string) (lines [][]byte, skipped int, err error) {
	fn := func(line []byte) {
		if len(line) == 0 || line[0] != '{' || !json.Valid(line) {
			skipped++
			return
		}
		lines = append(lines, line)
	}
	err = refusedAsSkipped(eachLine(filepath.Join(s.RunDir(sessionID), FileSteer), fn), s.refusal(), fn)
	return lines, skipped, err
}

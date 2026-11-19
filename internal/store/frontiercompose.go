package store

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// afterFrontierRemoved, nil outside tests, runs in both forgets right after
// removeFrontier, inside the records lock forget holds to its rewrite: the
// point where a compose that read the lines before the rewrite would save a
// frontier of what forget removes, were its save outside the lock.
var afterFrontierRemoved func()

// composeHeld, nil outside tests, runs in ComposeFrontier after the first
// hold of the records lock and before the fold: the point where an append
// lands past the bounds the hold read.
var composeHeld func()

// composeBetween, nil outside tests, runs in ComposeFrontier after the fold
// and before the second take of the records lock: the point where a forget
// that runs whole between the two holds lands.
var composeBetween func()

// Bounds is what a compose's first hold of the records lock read: G, and the
// size of each run file, the end the fold reads that file to. Every append
// writes its line whole inside the records lock, so RecordsSize, read while
// the lock is held, is a line end -- unless a crash left a torn line, which
// the next append cuts back (appendHealed). SteerSize is read inside the
// same hold, so every steer line read was written after the record its
// after_seq names, and that record is inside RecordsSize; a steer append in
// flight is a last line with no newline, which the fold does not read.
type Bounds struct {
	Generation  int64
	RecordsSize int64
	SteerSize   int64
}

// errGenerationMoved is the second hold's finding that the gaps file's size
// is no longer the one the fold was read under; it saves nothing.
var errGenerationMoved = errors.New("store: the gaps generation moved")

// errOffsetPastEnd is a read from an offset past the file's end.
var errOffsetPastEnd = errors.New("store: offset past the end of the file")

// ComposeFrontier runs one composition of sessionID's frontier around fold,
// which reads the run's lines and returns the frontier file's bytes. G, the
// size of the root gaps.ndjson, is the store's generation for it: only
// appends grow it (AppendGap, evict's gap), only purge removes it, and every
// forget and evict appends its gap before it rewrites or removes a run.
//
// ComposeFrontier takes the run's records lock L, reads the run's Bounds --
// G and the two files' sizes -- and releases L; calls fold(bounds, true),
// which reads each file no further than its size; then takes L again, reads
// G, and saves fold's bytes inside that hold only when G has not moved. A forget holds L from before
// its frontier goes to after its rewrite and moves G inside that hold, so a
// forget wholly before the first hold leaves lines the fold reads, one
// between the holds moves G and nothing is saved, and one after the save
// removes the frontier: no frontier of forgotten lines outlives a forget.
// L alone is taken, never the gaps lock; evict's window is a stated limit.
//
// When the first take fails -- its lock budget ran out (ErrLockTimeout), the
// run has no records file, or the bounds could not be read -- fold runs with
// ok false and zero Bounds, so the caller still answers from the whole
// record, and nothing is saved.
// A missed budget on either take, a moved G and an absent records file are
// no error and save nothing: the next compose folds again. Any other failure
// of the save side is returned with saved false, after fold has run. Both
// takes wait through lockAppend, so a hook's waits count in its tally.
func (s *Store) ComposeFrontier(sessionID string, fold func(b Bounds, ok bool) ([]byte, error)) (bool, error) {
	if err := s.opened(); err != nil {
		return false, err
	}
	var b0 Bounds
	err := s.underRecordsLock(sessionID, func(records *os.File, g int64) error {
		var err error
		b0, err = s.bounds(sessionID, records, g)
		return err
	})
	if composeHeld != nil {
		composeHeld()
	}
	data, ferr := fold(b0, err == nil)
	if ferr != nil {
		return false, ferr
	}
	if composeBetween != nil {
		composeBetween()
	}
	if err != nil || data == nil {
		return false, quietMiss(err)
	}
	saved := false
	err = s.underRecordsLock(sessionID, func(_ *os.File, g int64) error {
		if g != b0.Generation {
			return errGenerationMoved
		}
		if err := s.SaveFrontier(sessionID, data); err != nil {
			return err
		}
		saved = true
		return nil
	})
	return saved, quietMiss(err)
}

// quietMiss is err with the outcomes a compose takes in silence dropped: a
// missed lock budget, a moved generation and a run with no records file.
func quietMiss(err error) error {
	if errors.Is(err, ErrLockTimeout) || errors.Is(err, errGenerationMoved) || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// underRecordsLock takes sessionID's records lock within the Store's lock
// wait, reads G, calls inside with the locked handle and G, and releases the
// lock. The lock is records.ndjson's own, the one every append and forget
// takes, on a read-only handle: flock and LockFileEx take either kind.
func (s *Store) underRecordsLock(sessionID string, inside func(records *os.File, g int64) error) error {
	f, err := os.Open(filepath.Join(s.RunDir(sessionID), FileRecords))
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only; the lock is what matters
	unlock, err := s.lockAppend(f, s.lockWait(lockBudget))
	if err != nil {
		return err
	}
	defer unlock()
	g, err := gapsGeneration(s.root)
	if err != nil {
		return err
	}
	return inside(f, g)
}

// bounds is the run's Bounds, read inside the records lock: g, the records
// file's size through the locked handle -- forget rewrites the file in place
// (rewrite), so the handle's size is the path's -- and the steer file's, 0
// when it is absent.
func (s *Store) bounds(sessionID string, records *os.File, g int64) (Bounds, error) {
	info, err := records.Stat()
	if err != nil {
		return Bounds{}, err
	}
	b := Bounds{Generation: g, RecordsSize: info.Size()}
	steer, err := os.Stat(filepath.Join(s.RunDir(sessionID), FileSteer))
	switch {
	case err == nil:
		b.SteerSize = steer.Size()
	case !errors.Is(err, fs.ErrNotExist):
		return Bounds{}, err
	}
	return b, nil
}

// gapsGeneration is G, the size of root's gaps.ndjson. An absent file is 0,
// as an empty one is: no gap since the store began or since a purge, and
// forget and evict append before they rewrite or remove, so neither runs
// between two reads of 0 without moving it.
func gapsGeneration(root string) (int64, error) {
	info, err := os.Stat(filepath.Join(root, FileGaps))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// RunLinesFrom reads the run file name of sessionID (FileRecords or
// FileSteer) from offset to its last line end at or before limit, or before
// the file's end when limit is negative or past it. It returns the lines
// without their endings, end, the offset just past the last line read, and
// size, the file's size when it was read. A last line with no newline -- an
// append still being written, or a torn one -- is not read, and end stops
// before it so the next read starts there. A line over maxLine comes back
// nil, which parses as nothing, as every other reader counts it. An absent
// file or run directory is no lines at 0; an offset past where the read
// stops is an error.
func (s *Store) RunLinesFrom(sessionID, name string, offset, limit int64) (lines [][]byte, end, size int64, err error) {
	if err := s.opened(); err != nil {
		return nil, 0, 0, err
	}
	f, err := regfile.Open(filepath.Join(s.RunDir(sessionID), name))
	if errors.Is(err, fs.ErrNotExist) && offset == 0 {
		return nil, 0, 0, nil
	}
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		return nil, 0, 0, err
	}
	size = info.Size()
	stop := size
	if limit >= 0 && limit < stop {
		stop = limit
	}
	if offset < 0 || offset > stop {
		return nil, 0, size, errOffsetPastEnd
	}
	b, err := io.ReadAll(io.NewSectionReader(f, offset, stop-offset))
	if err != nil {
		return nil, 0, size, err
	}
	cut := bytes.LastIndexByte(b, '\n') + 1
	for rest := b[:cut]; len(rest) > 0; {
		i := bytes.IndexByte(rest, '\n')
		line := bytes.TrimSuffix(rest[:i], []byte{'\r'})
		rest = rest[i+1:]
		switch {
		case len(line) > maxLine:
			lines = append(lines, nil)
		case len(line) > 0:
			lines = append(lines, line)
		}
	}
	return lines, offset + int64(cut), size, nil
}

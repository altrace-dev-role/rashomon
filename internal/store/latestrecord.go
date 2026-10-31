package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// LatestRecordBytes is the last line of a run's records.ndjson whose head
// (runHead) names kind at a version AcceptsKind accepts, exactly as it was
// written and without its newline. A receipt digests a record's stored bytes
// (5.5), and a re-marshal of the decoded record could differ from them, so
// only each line's head is decoded and the line itself is handed back, as
// ReadSteer leaves its lines' decoding to their reader. A line that does not
// decode, a torn tail among them, counts in skipped. A run with no records
// file, or no record of the kind, is ok false.
//
// The file is read unlocked, as ReadRunDir and ReadSteer read: a line caught
// mid-write counts in skipped, and where locks are mandatory (Windows) a read
// the writer's lock refused counts as one.
func (s *Store) LatestRecordBytes(sessionID, kind string) (line []byte, ok bool, skipped int, err error) {
	fn := func(l []byte) {
		var head runHead
		if len(l) == 0 || json.Unmarshal(l, &head) != nil {
			skipped++
			return
		}
		if head.Type == kind && AcceptsKind(kind, head.SchemaVersion) {
			line, ok = l, true
		}
	}
	err = refusedAsSkipped(eachLine(filepath.Join(s.RunDir(sessionID), FileRecords), fn), s.refusal(), fn)
	return line, ok, skipped, err
}

// headKeys is the KeySet a record's head is matched with, built on first use
// and never at init (RES-2).
var headKeys struct {
	sync.Once
	v *jsonscan.KeySet
}

// isKind reports whether line's head names kind at a version AcceptsKind
// accepts. The head is located by span, not decoded (RES-5), so a line is
// matched without being parsed into a record.
func isKind(line []byte, kind string) bool {
	// A line of this kind holds its name; json.Marshal, the writer's, never
	// escapes it. The check spares the scan's validation of lines that are
	// plainly another kind, the bulk of a long file.
	if !bytes.Contains(line, []byte(kind)) {
		return false
	}
	headKeys.Do(func() { headKeys.v = jsonscan.NewKeySet("type", "schema_version") })
	var out [2]jsonscan.Span
	if len(line) == 0 || jsonscan.Top(line, headKeys.v, out[:]) != nil || out[0].Kind != jsonscan.String || out[1].Kind != jsonscan.Number {
		return false
	}
	typ, err := jsonscan.DecodeString(line, out[0])
	if err != nil || typ != kind {
		return false
	}
	var v int
	for _, c := range out[1].Raw(line) {
		if c < '0' || c > '9' || v > 1<<20 {
			return false
		}
		v = v*10 + int(c-'0')
	}
	return AcceptsKind(kind, v)
}

// EachRecordOfKind hands fn every line of a run's records.ndjson whose head
// names kind at an accepted version, in file order and as written, with no
// window and no slice kept: memory stays at one line beyond fn's own. fn
// returns false to stop the read there. Lines that do not parse count in
// skipped only when they cannot be told from a record of another kind (a
// torn tail, an over-long line). Read unlocked, as LatestRecordBytes reads.
func (s *Store) EachRecordOfKind(sessionID, kind string, fn func(line []byte) bool) (skipped int, err error) {
	f, err := os.Open(filepath.Join(s.RunDir(sessionID), FileRecords))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		if s.refusal()(err) {
			return 1, nil
		}
		return 0, err
	}
	defer f.Close() //nolint:errcheck // read-only
	err = forEachLineUntil(f, func(l []byte) bool {
		if l == nil {
			skipped++
			return true
		}
		if isKind(l, kind) {
			return fn(l)
		}
		return true
	})
	return skipped, err
}

// LatestRecordTail is the last line of kind among the last tail bytes of a
// run's records.ndjson, as LatestRecordBytes would hand it back but for a
// read bounded to the file's end: a checkpoint is cut at a Stop, so a run's
// last one sits there. The read starts at the first newline after the seek,
// so a line the seek cut in two is passed over; a file no longer than tail
// is read whole. readBytes is how many bytes were read. No such record, no
// file, or a refused read is ok false.
func (s *Store) LatestRecordTail(sessionID, kind string, tail int64) (line []byte, ok bool, readBytes int64, err error) {
	f, err := os.Open(filepath.Join(s.RunDir(sessionID), FileRecords))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, 0, nil
	}
	if err != nil {
		if s.refusal()(err) {
			return nil, false, 0, nil
		}
		return nil, false, 0, err
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		return nil, false, 0, err
	}
	var r io.Reader = &countingReader{r: f, n: &readBytes}
	if tail > 0 && info.Size() > tail {
		if _, err := f.Seek(info.Size()-tail, io.SeekStart); err != nil {
			return nil, false, 0, err
		}
		// Streamed, so a long tail costs a buffer, not a copy of itself. The
		// seek landed inside a line unless the byte before it was a newline,
		// which is not known: the first newline ends the partial one.
		br := bufio.NewReaderSize(&countingReader{r: io.LimitReader(f, tail), n: &readBytes}, readChunk)
		for {
			_, err := br.ReadSlice('\n')
			if err == nil {
				break
			}
			if !errors.Is(err, bufio.ErrBufferFull) {
				return nil, false, readBytes, nil
			}
		}
		r = br
	}
	err = forEachLine(r, func(l []byte) {
		if l != nil && isKind(l, kind) {
			line, ok = l, true
		}
	})
	return line, ok, readBytes, err
}

// countingReader adds each read's length to *n.
type countingReader struct {
	r io.Reader
	n *int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	*c.n += int64(n)
	return n, err
}

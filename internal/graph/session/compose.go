package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/fold"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// envelopeSchema is the frontier file's own schema. A file of any other is a
// miss, folded from the record and saved over, never continued.
const envelopeSchema = 1

// envelope is the whole frontier file (store.FileFrontier): a frontier and
// the place in each run file it was folded through. Records and Steer are
// byte cursors, as fold.File's are -- Offset just past the last line read,
// SizeSeen the file's size when it was read -- and fold.CursorValid checks
// them as it checks the fold file's. Seq is 0 in both: Frontier.Through
// holds the place in the merged order. Records.Generation is G, the store's
// gaps generation the lines were read under (store.ComposeFrontier), and
// Steer.Generation is 0. Each SizeSeen is the size the records lock's hold
// read (store.Bounds), so the records cursor's is a line end, and Offset
// equals it unless a crash left a torn line inside it. Mark is the records
// line that ends at Records.Offset, by its length and SHA-256, never its
// bytes (fold.LineMark), so a records file replaced under the cursor is
// refused even where its size and the line end at the cursor are what the
// cursor saw. It is absent when no records line was ever read; an envelope
// with a positive records offset and no mark is a miss.
type envelope struct {
	Schema   int             `json:"schema"`
	Records  fold.Cursor     `json:"records"`
	Steer    fold.Cursor     `json:"steer"`
	Mark     *fold.LineMark  `json:"records_tail,omitempty"`
	Frontier json.RawMessage `json:"frontier"`
}

// runRead is one read of a run's two files: the events decoded from the
// lines read, the cursors just past them, and the mark of the last records
// line read, nil when the read had none.
type runRead struct {
	evs            []Event
	records, steer fold.Cursor
	mark           *fold.LineMark
}

// composeStats is what one compose did: tail, how many events it decoded,
// and full, whether it folded the whole record rather than a tail.
type composeStats struct {
	tail int
	full bool
}

// Compose is session sessionID's frontier, read through store.ComposeFrontier
// and saved by it when the composition allows. The saved frontier is
// continued (Catch) by the lines after its cursors only when the file is an
// envelope of envelopeSchema around a frontier ParseFrontier takes, it was
// read under the store's G of now, both cursors are valid for their files,
// the records line that ends at its cursor is the one the envelope's mark
// names, and Catch does not miss; otherwise the frontier is Fold over every
// line.
// Either way it is Fold over the run's lines as they were read
// (TestCompose_EqualsTheWholeFold). A line that does not decode is skipped
// in both, so the two stay one fold. f is the frontier whenever the fold
// ran, with an error from the save side too; saved is whether it was written.
func Compose(st *store.Store, sessionID string) (f Frontier, saved bool, err error) {
	f, _, saved, err = compose(st, sessionID)
	return f, saved, err
}

// unbounded is the Bounds of a fold no hold read: each file to its end, for
// an answer that is never saved.
var unbounded = store.Bounds{RecordsSize: -1, SteerSize: -1}

// compose is Compose with what it did.
func compose(st *store.Store, sessionID string) (Frontier, composeStats, bool, error) {
	var (
		f     Frontier
		stats composeStats
	)
	saved, err := st.ComposeFrontier(sessionID, func(b store.Bounds, locked bool) ([]byte, error) {
		r, hit := runRead{}, false
		if locked {
			f, r, hit = catchUp(st, sessionID, b)
		} else {
			b = unbounded
		}
		if !hit {
			var err error
			if r, err = readRun(st, sessionID, 0, 0, b); err != nil {
				return nil, err
			}
			f, stats.full = Fold(sessionID, r.evs), true
		}
		stats.tail = len(r.evs)
		if !locked {
			return nil, nil
		}
		return marshalEnvelope(f, r)
	})
	return f, stats, saved, err
}

// catchUp is the saved frontier continued by the lines after its cursors,
// and whether it hit. Every failure on the way is a miss: the caller folds
// the whole record, and a read that fails there too is its error.
//
// The records cursor is checked with its mark (fold.CursorTailValid), the
// steer cursor by fold.CursorValid alone. The mark's limit is the limit of
// this check: a records file replaced by one whose line ending at the cursor
// is byte for byte the marked line, at the same offset, is continued as if
// it were the file the frontier was folded from. Every records line carries
// its own tool use id, so two runs' files do not share one by chance.
func catchUp(st *store.Store, sessionID string, b store.Bounds) (Frontier, runRead, bool) {
	data, ok, err := st.LoadFrontier(sessionID)
	if err != nil || !ok {
		return Frontier{}, runRead{}, false
	}
	env, f, err := parseEnvelope(data)
	if err != nil || env.Records.Generation != b.Generation || env.Steer.Generation != 0 {
		return Frontier{}, runRead{}, false
	}
	dir := st.RunDir(sessionID)
	if valid, err := fold.CursorTailValid(filepath.Join(dir, store.FileRecords), env.Records, env.Mark); err != nil || !valid {
		return Frontier{}, runRead{}, false
	}
	if valid, err := fold.CursorValid(filepath.Join(dir, store.FileSteer), env.Steer); err != nil || !valid {
		return Frontier{}, runRead{}, false
	}
	r, err := readRun(st, sessionID, env.Records.Offset, env.Steer.Offset, b)
	if err != nil {
		return Frontier{}, runRead{}, false
	}
	// A catch-up that read no records line ends at the saved cursor, so the
	// saved mark is still the mark of the line that ends there.
	if r.mark == nil {
		r.mark = env.Mark
	}
	f, hit := Catch(f, r.evs)
	return f, r, hit
}

// readRun reads sessionID's records file from records and its steer file
// from steer, each to the size b holds for it, and the cursors' SizeSeen are
// those sizes. A line appended after the hold is the next compose's tail,
// and no steer line read names a record the read stops before. The mark is
// taken from the records bytes as the file holds them (store.RunRegionFrom),
// not from the lines, which drop a '\r' and empty lines.
func readRun(st *store.Store, sessionID string, records, steer int64, b store.Bounds) (runRead, error) {
	rl, raw, rend, _, err := st.RunRegionFrom(sessionID, store.FileRecords, records, b.RecordsSize)
	if err != nil {
		return runRead{}, err
	}
	sl, send, _, err := st.RunLinesFrom(sessionID, store.FileSteer, steer, b.SteerSize)
	if err != nil {
		return runRead{}, err
	}
	r := runRead{
		records: fold.Cursor{Offset: rend, SizeSeen: b.RecordsSize, Generation: b.Generation},
		steer:   fold.Cursor{Offset: send, SizeSeen: b.SteerSize},
	}
	if len(raw) > 0 {
		body := raw[:len(raw)-1]
		r.mark = fold.MarkOf(body[bytes.LastIndexByte(body, '\n')+1:])
	}
	for _, line := range rl {
		if ev, ok, err := RecordEvent(line); err == nil && ok {
			r.evs = append(r.evs, ev)
		}
	}
	for _, line := range sl {
		if ev, ok, err := SteerEvent(line); err == nil && ok {
			r.evs = append(r.evs, ev)
		}
	}
	return r, nil
}

// marshalEnvelope is the frontier file of f folded through r.
func marshalEnvelope(f Frontier, r runRead) ([]byte, error) {
	fb, err := MarshalFrontier(f)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{Schema: envelopeSchema, Records: r.records, Steer: r.steer, Mark: r.mark, Frontier: fb})
}

// parseEnvelope decodes a frontier file: an envelope of envelopeSchema with
// no unknown field and nothing after it, around a frontier ParseFrontier
// takes. A bare frontier is no envelope: its fold_version is unknown here.
func parseEnvelope(b []byte) (envelope, Frontier, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var e envelope
	if err := dec.Decode(&e); err != nil {
		return envelope{}, Frontier{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return envelope{}, Frontier{}, fmt.Errorf("frontier file: data after the object")
	}
	if e.Schema != envelopeSchema {
		return envelope{}, Frontier{}, fmt.Errorf("frontier file: schema %d, want %d", e.Schema, envelopeSchema)
	}
	f, err := ParseFrontier(e.Frontier)
	if err != nil {
		return envelope{}, Frontier{}, err
	}
	return e, f, nil
}

package verify

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The project index (PF-PG1, 2.2.10) is a project's memory, across sessions,
// of the gate families it has exercised: a family seen in RequiredSessions
// sessions or more, the last within RequiredWindowMS, is required (its gate
// must be green for a turn to read verified); one seen once is advisory. It
// is the only state the evaluator reads that is not the run's own record.
//
// Its life cycle is read-modify-write. Recap (2.4) calls ReadIndex, Observe
// and WriteIndex, in that order, at session end; post (2.2) calls ReadIndex
// once per PostToolUse call, which is why the file is capped on both sides.
// A recap that started each session from a fresh index would hold Sessions
// at 1 forever, and no family would ever be required.
const (
	// RequiredSessions is how many sessions must have run a family for it to
	// be required.
	RequiredSessions = 2
	// RequiredWindowMS is how recent the last of them must be: thirty days,
	// inclusive.
	RequiredWindowMS = 30 * 24 * 60 * 60 * 1000
	// MaxIndexBytes caps the index file on read and on write: 64 KiB, the
	// magnitude of harness_counts.json's cap (internal/store/harnesscounts.go).
	MaxIndexBytes = 64 << 10
)

// IndexEntry is one family's history in the project index.
type IndexEntry struct {
	Family      string `json:"family"` // GateState.Family: runner "@" location, the full key (D33)
	Kind        string `json:"kind"`   // the family's kind when last observed (kindOf)
	Sessions    int    `json:"sessions"`
	LastSession string `json:"last_session"`
	FirstSeen   int64  `json:"first_seen_unix_ms"`
	LastSeen    int64  `json:"last_seen_unix_ms"`
}

// DirName is the directory under the store root that verify writes into, and
// ProjectsDir the directory of index files within it; the disclosure
// inventory and purge read both from here.
const (
	DirName     = "verify"
	ProjectsDir = "projects"
)

// IndexPath is the index file of projectKey under the store root:
// <root>/verify/projects/<name>, where name is baseline.FileName(projectKey),
// as the baseline files are named. baseline.Key is an absolute path, so the
// key is not a file name as it stands; FileName gives it one path element
// and its own ".json".
func IndexPath(root, projectKey string) string {
	return filepath.Join(root, DirName, ProjectsDir, baseline.FileName(projectKey))
}

// IsIndexFile reports whether a file of this name in ProjectsDir is one
// WriteIndex leaves: an index file, which has baseline.FileName's shape, or
// the temporary name store.ReplaceFile writes one under before its rename,
// "." + the index file's name + ".tmp-" and a suffix, left by a process that
// died in between.
func IsIndexFile(name string) bool {
	if baseline.IsFileName(name) {
		return true
	}
	rest, ok := strings.CutPrefix(name, ".")
	i := strings.LastIndex(rest, ".tmp-")
	return ok && i >= 0 && baseline.IsFileName(rest[:i])
}

// Required reports whether the index requires family at nowMS: its entry
// exists, RequiredSessions or more sessions have run it, and the last of them
// is no more than RequiredWindowMS before nowMS (thirty days exactly is still
// required; a millisecond more is not). A nil index, or a family not in it,
// is not required here; what a missing index means is Evaluate's choice
// (1.5), which reads it as every family required.
func (idx *ProjectIndex) Required(family string, nowMS int64) bool {
	if idx == nil {
		return false
	}
	i, ok := slices.BinarySearchFunc(idx.Entries, family, atFamily)
	if !ok {
		return false
	}
	e := idx.Entries[i]
	return e.Sessions >= RequiredSessions && nowMS-e.LastSeen <= RequiredWindowMS
}

// entryOrder is the one order the index keeps its entries in: by Family.
func entryOrder(a, b IndexEntry) int { return cmp.Compare(a.Family, b.Family) }

// atFamily is entryOrder as a binary search target.
func atFamily(e IndexEntry, family string) int { return cmp.Compare(e.Family, family) }

// Observe adds one session's gate runs to the index, nowMS being that
// session's own time (its end, as recap passes it). Each family among runs
// (a run with no family adds nothing) gets an entry, created where it has
// none. Its times only widen, LastSeen to the later of the two and FirstSeen
// to the earlier, so sessions recapped out of order never move LastSeen
// backward. Its Kind becomes the family's over its runs in Seq order
// (kindOf) only when this session is not older than LastSeen, a tie going to
// the later call, so the later session's classification wins (D33), not the
// later recap's. Its Sessions moves only when LastSession is another
// session, which sessionID then becomes on every call. LastSession is the
// dedup key, the session counted last, and Kind is the time question, so the
// one is unconditional and the other follows the later session: a recap run
// again for the session just counted counts it once. The limit is that a
// session recapped again after another session's recap (A, B, A, in either
// order of time) counts again, since only the last id is kept, so a family
// can be required early, never late; it no longer moves LastSeen or Kind.
//
// The entries stay sorted by Family and the result does not depend on the
// order of runs. idx must not be nil: a caller with no index starts from
// &ProjectIndex{ProjectKey: key}.
func (idx *ProjectIndex) Observe(runs []gateRun, sessionID string, nowMS int64) {
	byFamily := make(map[familyKey][]gateRun)
	for _, r := range runs {
		if k, ok := r.family(); ok {
			byFamily[k] = append(byFamily[k], r)
		}
	}
	// Entries is searched only up to known: what is appended below is out of
	// order until the sort at the end.
	known := len(idx.Entries)
	for k, fr := range byFamily {
		slices.SortFunc(fr, func(a, b gateRun) int { return cmp.Compare(a.Seq, b.Seq) })
		kind, family := kindOf(fr), k.runner+"@"+k.location
		i, ok := slices.BinarySearchFunc(idx.Entries[:known], family, atFamily)
		if !ok {
			idx.Entries = append(idx.Entries, IndexEntry{Family: family, Kind: kind, Sessions: 1,
				LastSession: sessionID, FirstSeen: nowMS, LastSeen: nowMS})
			continue
		}
		e := &idx.Entries[i]
		held := e.LastSeen
		e.LastSeen, e.FirstSeen = max(held, nowMS), min(e.FirstSeen, nowMS)
		if nowMS >= held {
			e.Kind = kind
		}
		if e.LastSession != sessionID {
			e.Sessions++
			e.LastSession = sessionID
		}
	}
	slices.SortFunc(idx.Entries, entryOrder)
}

// ReadIndex reads the index at path. A missing file is (nil, nil), no index.
// A file over MaxIndexBytes, one that cannot be read, and one that is not
// this shape are (nil, error), and a caller reads an error as no index, which
// in 1.5 makes every family required: the fail-closed direction. The shape is
// one JSON value with its entries strictly ascending by Family, which
// Required's binary search relies on (out of order, a present family would
// read as absent: not required, the fail-open direction). A key it does not
// name, at the top or in an entry, reads and is dropped: a later build wrote
// it, and refusing it would make this build's next Stop write a fresh index
// over the learned history it could not read (INDEX-TOLERANT1, #171 D1). The
// rewrite loses only that key.
func ReadIndex(path string) (*ProjectIndex, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	// One byte past the cap, so that a file over it is seen to be.
	lr := &io.LimitedReader{R: f, N: MaxIndexBytes + 1}
	dec := json.NewDecoder(lr)
	var idx ProjectIndex
	if err := dec.Decode(&idx); err != nil {
		return nil, fmt.Errorf("verify: project index %s: %w", path, err)
	}
	// Token reads to the end, so lr.N is final after it.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("verify: project index %s: data after its one value", path)
	}
	if lr.N == 0 {
		return nil, fmt.Errorf("verify: project index %s is over its %d-byte cap", path, MaxIndexBytes)
	}
	for i := 1; i < len(idx.Entries); i++ {
		if idx.Entries[i-1].Family >= idx.Entries[i].Family {
			return nil, fmt.Errorf("verify: project index %s: entry %d is not after entry %d by family", path, i, i-1)
		}
	}
	return &idx, nil
}

// WriteIndex writes idx to path, its entries sorted by Family (idx itself is
// not changed): under a temporary name beside it, synced and renamed over it
// at 0600 (store.ReplaceFile), with its directory created at 0700 where it is
// absent. While the encoding is over MaxIndexBytes it evicts entries, the
// least recently seen first (LastSeen ascending, then Family ascending), and
// returns how many. A project key over the cap with no entries is an error,
// and nothing is written. idx must not be nil.
//
// The eviction is this file's own choice. harness_counts.json refuses a write
// over its cap; a refusal here would stop the index exactly when a family
// qualifies. The stalest go instead, the families nearest to aging out of
// Required; an evicted family still inside the window loses its count and is
// advisory again until two more sessions run it.
func WriteIndex(path string, idx *ProjectIndex) (evicted int, err error) {
	entries := slices.Clone(idx.Entries)
	slices.SortFunc(entries, entryOrder)
	entries, evicted, err = evictToCap(*idx, entries)
	if err != nil {
		return 0, err
	}
	encoded, err := encodeIndex(ProjectIndex{ProjectKey: idx.ProjectKey, Entries: entries})
	if err != nil {
		return 0, err
	}
	if len(encoded) > MaxIndexBytes {
		return 0, fmt.Errorf("verify: the project index of %q is over its %d-byte cap with no entries", idx.ProjectKey, MaxIndexBytes)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, err
	}
	if err := store.ReplaceFile(path, encoded); err != nil {
		return 0, err
	}
	return evicted, nil
}

// evictToCap is entries, sorted by Family, less the fewest of the least
// recently seen whose going brings the encoding to MaxIndexBytes or under,
// and how many went. The size is counted per entry rather than re-encoded per
// eviction: the file is its frame (encodeIndex with no entries) and each
// entry's own encoding, comma-separated, so an entry that goes takes its
// bytes and one comma. The count is exact, so the cap has one test, the
// loop's. Where every entry goes and the frame alone is over the cap,
// WriteIndex's check on the encoding refuses it.
func evictToCap(idx ProjectIndex, entries []IndexEntry) ([]IndexEntry, int, error) {
	frame, err := encodeIndex(ProjectIndex{ProjectKey: idx.ProjectKey})
	if err != nil {
		return nil, 0, err
	}
	size := make([]int, len(entries))
	total := len(frame) - 1 // n entries carry n-1 commas
	for i, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, 0, err
		}
		size[i] = len(b) + 1
		total += size[i]
	}
	stalest := make([]int, len(entries))
	for i := range stalest {
		stalest[i] = i
	}
	slices.SortFunc(stalest, func(a, b int) int {
		return cmp.Or(cmp.Compare(entries[a].LastSeen, entries[b].LastSeen), cmp.Compare(entries[a].Family, entries[b].Family))
	})
	gone := make([]bool, len(entries))
	n := 0
	for _, i := range stalest {
		if total <= MaxIndexBytes {
			break
		}
		gone[i], total, n = true, total-size[i], n+1
	}
	kept := entries[:0]
	for i, e := range entries {
		if !gone[i] {
			kept = append(kept, e)
		}
	}
	return kept, n, nil
}

// encodeIndex is the file's bytes: the index as one JSON line, its entries
// [] rather than null when there are none.
func encodeIndex(idx ProjectIndex) ([]byte, error) {
	if idx.Entries == nil {
		idx.Entries = []IndexEntry{}
	}
	b, err := json.Marshal(idx)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

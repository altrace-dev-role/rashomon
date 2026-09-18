package store

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// evictGrace keeps a run that was written to recently out of eviction. The
// current session is protected by name; this protects any other session that
// is still live, so that eviction never races an append.
const evictGrace = time.Hour

// window is the half-open range of record instants a forget removes, in Unix
// milliseconds. An open side is held as the extreme of the range, so one
// comparison serves a bounded side and an unbounded one alike.
type window struct {
	fromMS, toMS int64
}

func newWindow(from, to *time.Time) window {
	w := window{fromMS: math.MinInt64, toMS: math.MaxInt64}
	if from != nil {
		w.fromMS = from.UnixMilli()
	}
	if to != nil {
		w.toMS = to.UnixMilli()
	}
	return w
}

func (w window) holds(ms int64) bool { return ms >= w.fromMS && ms < w.toMS }

func (w window) openBelow() bool { return w.fromMS == math.MinInt64 }
func (w window) openAbove() bool { return w.toMS == math.MaxInt64 }

// ForgetWindow removes every declaration, execution and terminal recorded in
// the half-open window [from, to), leaving a gap record per affected run.
// Coverage records stay: they describe whether the run could be trusted, which
// remains true of it after its content has been forgotten.
//
// A nil bound is unbounded on that side, which is what the two flags are:
// --since names the lower bound and removes everything after it, --before names
// the upper bound and removes everything before it. They are the same operation
// with opposite ends left open, and one code path so that neither end can
// acquire a rule the other does not have.
//
// The records sharing a tool_use_id leave together -- declaration, execution
// and terminal. Removing by timestamp alone can split a set that straddles the
// instant, and a declaration left without its terminal reads exactly like a
// handler that was killed.
func (s *Store) ForgetWindow(from, to *time.Time, now time.Time) ([]Gap, error) {
	w := newWindow(from, to)
	names, err := s.Runs()
	if err != nil {
		return nil, err
	}
	var gaps []Gap
	for _, name := range names {
		g, err := s.forgetRun(filepath.Join(s.root, dirRuns, name), w, now)
		if err != nil {
			return gaps, err
		}
		if g != nil {
			gaps = append(gaps, *g)
		}
	}
	return gaps, nil
}

// lineHead is what forget needs to know about a record to decide its fate.
type lineHead struct {
	RecordedAtMS int64  `json:"recorded_at_unix_ms"`
	ToolUseID    string `json:"tool_use_id"`
	SessionID    string `json:"session_id"`
}

func (s *Store) forgetRun(dir string, w window, now time.Time) (*Gap, error) {
	recordsPath := filepath.Join(dir, FileRecords)
	spillPath := filepath.Join(dir, FileSpill)

	// The ordered stream's lock is held for the whole operation so that no
	// handler appends between the plan and the rewrite.
	records, err := os.OpenFile(recordsPath, os.O_RDWR, fileMode)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if records != nil {
		defer records.Close() //nolint:errcheck // Sync reports the write failure
		unlock, err := lockFile(records, lockBudget)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}

	recLines, err := readLines(records)
	if err != nil {
		return nil, err
	}
	spillLines, err := readLinesAt(spillPath)
	if err != nil {
		return nil, err
	}

	// Plan: every tool_use_id with any record in the window, from either file.
	doomed := map[string]bool{}
	sessionID := ""
	for _, line := range append(append([][]byte{}, recLines...), spillLines...) {
		var h lineHead
		if json.Unmarshal(line, &h) != nil {
			continue
		}
		if sessionID == "" {
			sessionID = h.SessionID
		}
		if w.holds(h.RecordedAtMS) {
			doomed[h.ToolUseID] = true
		}
	}
	if len(doomed) == 0 {
		return nil, nil
	}
	// The earliest instant actually removed, which is what the gap's own span
	// starts at when the window was left open below: there is no lower bound
	// to name there, and the records are the only thing that says how far back
	// the removal reached.
	earliest := int64(math.MaxInt64)
	keep := func(line []byte) bool {
		var h lineHead
		// A line that does not parse is kept: forget removes what it can
		// identify, and never widens into "remove whatever is here".
		if json.Unmarshal(line, &h) != nil || !doomed[h.ToolUseID] {
			return true
		}
		if h.RecordedAtMS < earliest {
			earliest = h.RecordedAtMS
		}
		return false
	}
	keptRec, removedRec := partition(recLines, keep)
	keptSpill, removedSpill := partition(spillLines, keep)
	if removedRec+removedSpill == 0 {
		return nil, nil
	}
	if sessionID == "" {
		sessionID = filepath.Base(dir)
	}

	// The gap spans what was removed. A bound the caller named is that bound;
	// an open side is closed by what was there -- the earliest record removed
	// below, and this instant above -- so the record never claims a window
	// wider than the one it emptied.
	from, to := w.fromMS, w.toMS
	if w.openBelow() {
		from = earliest
	}
	if w.openAbove() {
		to = now.UnixMilli()
	}

	// The gap lands before anything is removed. A failure between the two
	// leaves a gap for records that still exist -- an overstatement a reader
	// can see -- rather than a deletion nothing admits to.
	g := Gap{
		Type:           TypeGap,
		SchemaVersion:  SchemaVersion,
		RecordedAtMS:   now.UnixMilli(),
		SessionID:      sessionID,
		Reason:         GapForget,
		FromUnixMS:     from,
		ToUnixMS:       to,
		RemovedRecords: removedRec + removedSpill,
	}
	if err := s.AppendGap(g); err != nil {
		return nil, err
	}

	if records != nil && removedRec > 0 {
		if err := rewrite(records, keptRec); err != nil {
			return &g, err
		}
	}
	if removedSpill > 0 {
		if err := rewriteAt(spillPath, keptSpill); err != nil {
			return &g, err
		}
	}
	return &g, nil
}

// Evict removes whole runs, oldest first, until the store fits under capBytes.
// The named session and any run written to within evictGrace are never
// candidates. Every removed run leaves a gap record.
func (s *Store) Evict(capBytes int64, protect string, now time.Time) ([]Gap, error) {
	if capBytes <= 0 {
		return nil, nil
	}

	// One evictor at a time. Holding the gaps file's lock for the whole pass
	// means a second handler that arrives mid-eviction sees the removals and
	// does not write a second gap record for a run that is already gone.
	gf, err := os.OpenFile(filepath.Join(s.root, FileGaps), appendFlags, fileMode)
	if err != nil {
		return nil, err
	}
	defer gf.Close() //nolint:errcheck // Sync per record reports write failures
	unlock, err := lockFile(gf, lockBudget)
	if err != nil {
		return nil, err
	}
	defer unlock()

	runsDir := filepath.Join(s.root, dirRuns)
	entries, err := os.ReadDir(runsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	type candidate struct {
		name  string
		size  int64
		mtime time.Time
	}
	var (
		cands []candidate
		total int64
	)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c := candidate{name: e.Name()}
		files, err := os.ReadDir(filepath.Join(runsDir, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			info, err := f.Info()
			if err != nil {
				return nil, err
			}
			c.size += info.Size()
			if info.ModTime().After(c.mtime) {
				c.mtime = info.ModTime()
			}
		}
		cands = append(cands, c)
		total += c.size
	}
	if total <= capBytes {
		return nil, nil
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mtime.Before(cands[j].mtime) })

	protectDir := segment(protect)
	var gaps []Gap
	for _, c := range cands {
		if total <= capBytes {
			break
		}
		if c.name == protectDir || now.Sub(c.mtime) < evictGrace {
			continue
		}

		run, err := s.ReadRunDir(c.name)
		if err != nil {
			return gaps, err
		}
		from, to := recordSpan(run, c.mtime)
		g := Gap{
			Type:           TypeGap,
			SchemaVersion:  SchemaVersion,
			RecordedAtMS:   now.UnixMilli(),
			SessionID:      run.SessionID(),
			Reason:         GapSizeCap,
			FromUnixMS:     from,
			ToUnixMS:       to,
			RemovedRecords: len(run.Declarations) + len(run.Executions) + len(run.Terminals),
		}

		// The gap lands before the run goes, for the same reason as in forget.
		line, err := marshalLine(g)
		if err != nil {
			return gaps, err
		}
		if _, err := gf.Write(line); err != nil {
			return gaps, err
		}
		if err := gf.Sync(); err != nil {
			return gaps, err
		}
		if err := os.RemoveAll(filepath.Join(runsDir, c.name)); err != nil {
			return gaps, err
		}

		total -= c.size
		gaps = append(gaps, g)
	}
	return gaps, nil
}

// recordSpan is the time range a run's records cover, falling back to the
// directory's modification time for a run that holds none.
func recordSpan(run *Run, fallback time.Time) (from, to int64) {
	first := true
	consider := func(ms int64) {
		if first || ms < from {
			from = ms
		}
		if first || ms > to {
			to = ms
		}
		first = false
	}
	for _, d := range run.Declarations {
		consider(d.RecordedAtMS)
	}
	for _, x := range run.Executions {
		consider(x.RecordedAtMS)
	}
	for _, t := range run.Terminals {
		consider(t.RecordedAtMS)
	}
	if first {
		ms := fallback.UnixMilli()
		return ms, ms
	}
	return from, to
}

func partition(lines [][]byte, keep func([]byte) bool) (kept [][]byte, removed int) {
	for _, line := range lines {
		if keep(line) {
			kept = append(kept, line)
		} else {
			removed++
		}
	}
	return kept, removed
}

// readLines reads every non-empty line of an open file from its start. A nil
// file has no lines.
func readLines(f *os.File) ([][]byte, error) {
	if f == nil {
		return nil, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	for sc.Scan() {
		if line := sc.Bytes(); len(line) > 0 {
			lines = append(lines, append([]byte(nil), line...))
		}
	}
	return lines, sc.Err()
}

func readLinesAt(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	return readLines(f)
}

// rewrite replaces an open file's content in place.
//
// In place rather than temp-and-rename on purpose. The append lock is taken on
// the file's inode; a handler blocked on that lock holds a descriptor to this
// inode and will append to it once the lock is released. A rename would leave
// that handler appending to an orphan.
func rewrite(f *os.File, lines [][]byte) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return f.Sync()
}

// rewriteAt rewrites the spill file under its lock. Spill writers wait only
// briefly for that lock and then append regardless, so a rewrite that takes
// longer than their patience can race one; the rewrite is a few microseconds,
// their patience is fifty milliseconds, and the loss on the far side of that
// is one terminal, which reads as unverified rather than as anything false.
func rewriteAt(path string, lines [][]byte) error {
	f, err := os.OpenFile(path, os.O_RDWR, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync reports the write failure
	unlock, err := lockFile(f, lockBudget)
	if err != nil {
		return err
	}
	defer unlock()
	return rewrite(f, lines)
}

// declHead is what a host-scoped forget needs from a record: the join key and
// the hostnames a declaration named.
type declHead struct {
	Type      string   `json:"type"`
	ToolUseID string   `json:"tool_use_id"`
	SessionID string   `json:"session_id"`
	Hosts     []string `json:"hosts"`
	SSHHosts  []string `json:"ssh_hosts"`
}

// ForgetHost removes every record of every call that NAMED the given host,
// across all runs, leaving a gap record per affected run.
//
// It removes whole calls rather than editing the host out of a declaration's
// list. Editing would keep more information -- a call that named two hosts
// loses the record of the other this way -- but it means rewriting a record's
// contents, and every other removal in this store works by dropping whole
// lines whose tool_use_id is doomed. One mechanism, already tested, is worth
// more here than the extra field survived: the caller asked for that host's
// records to be gone, and the declaration IS that host's record.
//
// The host is matched against both declared lists. An ssh host is not
// observable on the wire, but it is still something the session named and still
// something a user can ask to have forgotten.
//
// What this canNOT do is delete the row from the PROXY's store. That is the
// closed product's hash-chained audit database, opened read-only here, and
// deleting from it would break the chain it exists to provide. The gap record
// carries the host so the report keeps the destination suppressed from its
// view instead, and says so rather than implying the row is gone.
func (s *Store) ForgetHost(host string, now time.Time) ([]Gap, error) {
	if host == "" {
		return nil, errors.New("store: forget --host needs a host")
	}
	names, err := s.Runs()
	if err != nil {
		return nil, err
	}
	var gaps []Gap
	for _, name := range names {
		g, err := s.forgetHostInRun(filepath.Join(s.root, dirRuns, name), host, now)
		if err != nil {
			return gaps, err
		}
		if g != nil {
			gaps = append(gaps, *g)
		}
	}
	return gaps, nil
}

func (s *Store) forgetHostInRun(dir, host string, now time.Time) (*Gap, error) {
	recordsPath := filepath.Join(dir, FileRecords)
	spillPath := filepath.Join(dir, FileSpill)

	records, err := os.OpenFile(recordsPath, os.O_RDWR, fileMode)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if records != nil {
		defer records.Close() //nolint:errcheck // Sync reports the write failure
		unlock, err := lockFile(records, lockBudget)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}

	recLines, err := readLines(records)
	if err != nil {
		return nil, err
	}
	spillLines, err := readLinesAt(spillPath)
	if err != nil {
		return nil, err
	}

	// Plan: every tool_use_id whose DECLARATION named the host. Only a
	// declaration carries hostnames, so only a declaration can nominate a call
	// for removal; its execution and terminal follow it out by id, exactly as
	// they do for a window forget.
	doomed := map[string]bool{}
	sessionID := ""
	for _, line := range append(append([][]byte{}, recLines...), spillLines...) {
		var h declHead
		if json.Unmarshal(line, &h) != nil {
			continue
		}
		if sessionID == "" {
			sessionID = h.SessionID
		}
		if h.Type != TypeDeclaration {
			continue
		}
		if namesHost(h, host) {
			doomed[h.ToolUseID] = true
		}
	}
	if len(doomed) == 0 {
		return nil, nil
	}

	earliest, latest := int64(math.MaxInt64), int64(math.MinInt64)
	keep := func(line []byte) bool {
		var h lineHead
		if json.Unmarshal(line, &h) != nil || !doomed[h.ToolUseID] {
			return true
		}
		if h.RecordedAtMS < earliest {
			earliest = h.RecordedAtMS
		}
		if h.RecordedAtMS > latest {
			latest = h.RecordedAtMS
		}
		return false
	}
	keptRec, removedRec := partition(recLines, keep)
	keptSpill, removedSpill := partition(spillLines, keep)
	if removedRec+removedSpill == 0 {
		return nil, nil
	}
	if sessionID == "" {
		sessionID = filepath.Base(dir)
	}

	// The gap spans what was actually removed rather than all of time: a
	// host-scoped forget names no window, so the records themselves are the
	// only honest bounds.
	g := Gap{
		Type:           TypeGap,
		SchemaVersion:  SchemaVersion,
		RecordedAtMS:   now.UnixMilli(),
		SessionID:      sessionID,
		Reason:         GapForgetHost,
		FromUnixMS:     earliest,
		ToUnixMS:       latest,
		RemovedRecords: removedRec + removedSpill,
		HostDigest:     s.HostDigest(host),
	}
	// The gap lands first, as it does for a window forget: a failure between
	// the two leaves a gap for records that still exist, an overstatement a
	// reader can see, rather than a deletion nothing admits to.
	if err := s.AppendGap(g); err != nil {
		return nil, err
	}

	if records != nil && removedRec > 0 {
		if err := rewrite(records, keptRec); err != nil {
			return &g, err
		}
	}
	if removedSpill > 0 {
		if err := rewriteAt(spillPath, keptSpill); err != nil {
			return &g, err
		}
	}
	return &g, nil
}

// namesHost reports whether a declaration named the host, in either list.
func namesHost(h declHead, host string) bool {
	for _, list := range [][]string{h.Hosts, h.SSHHosts} {
		for _, got := range list {
			if got == host {
				return true
			}
		}
	}
	return false
}

// HostDigest is the keyed identifier a gap record carries for a forgotten
// host.
//
// HMAC under the per-install key, for the reason set out on Gap.HostDigest: the
// report has to recognise the host again while the store must not hold its
// name. Two installs produce different digests for the same host, so a store
// cannot be tested against a dictionary of hostnames.
func (s *Store) HostDigest(host string) string {
	if host == "" {
		return ""
	}
	m := hmac.New(sha256.New, s.Key())
	m.Write([]byte("forget-host\x00"))
	m.Write([]byte(host))
	return hex.EncodeToString(m.Sum(nil))
}

// ForgottenHost reports whether a host has been removed by a host-scoped
// forget, by recomputing its keyed digest and looking for it among the gaps.
//
// Returned as a predicate rather than a set of names because the names are not
// recoverable: the caller asks about a host it already has, which is exactly
// what the report does for each destination it observed.
func (s *Store) ForgottenHost() (func(string) bool, error) {
	gaps, err := s.ReadGaps()
	if err != nil {
		return nil, err
	}
	digests := map[string]bool{}
	for _, g := range gaps {
		if g.Reason == GapForgetHost && g.HostDigest != "" {
			digests[g.HostDigest] = true
		}
	}
	if len(digests) == 0 {
		// A predicate that allocates and hashes nothing on the overwhelmingly
		// common path where nothing has been forgotten.
		return func(string) bool { return false }, nil
	}
	return func(host string) bool { return digests[s.HostDigest(host)] }, nil
}

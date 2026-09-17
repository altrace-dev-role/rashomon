package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// evictGrace keeps a run that was written to recently out of eviction. The
// current session is protected by name; this protects any other session that
// is still live, so that eviction never races an append.
const evictGrace = time.Hour

// Forget removes every declaration and terminal recorded at or after since,
// leaving a gap record per affected run. Coverage records stay: they describe
// whether the run could be trusted, which remains true of it after its content
// has been forgotten.
//
// A declaration and its terminal leave together. Removing by timestamp alone
// can split a pair that straddles the instant, and a declaration left without
// its terminal reads exactly like a handler that was killed.
func (s *Store) Forget(since, now time.Time) ([]Gap, error) {
	names, err := s.Runs()
	if err != nil {
		return nil, err
	}
	var gaps []Gap
	for _, name := range names {
		g, err := s.forgetRun(filepath.Join(s.root, dirRuns, name), since.UnixMilli(), now)
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

func (s *Store) forgetRun(dir string, sinceMS int64, now time.Time) (*Gap, error) {
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
		if h.RecordedAtMS >= sinceMS {
			doomed[h.ToolUseID] = true
		}
	}
	if len(doomed) == 0 {
		return nil, nil
	}
	keep := func(line []byte) bool {
		var h lineHead
		// A line that does not parse is kept: forget removes what it can
		// identify, and never widens into "remove whatever is here".
		return json.Unmarshal(line, &h) != nil || !doomed[h.ToolUseID]
	}
	keptRec, removedRec := partition(recLines, keep)
	keptSpill, removedSpill := partition(spillLines, keep)
	if removedRec+removedSpill == 0 {
		return nil, nil
	}
	if sessionID == "" {
		sessionID = filepath.Base(dir)
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
		FromUnixMS:     sinceMS,
		ToUnixMS:       now.UnixMilli(),
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
	gf, err := os.OpenFile(filepath.Join(s.root, FileGaps), os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
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
			RemovedRecords: len(run.Declarations) + len(run.Terminals),
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

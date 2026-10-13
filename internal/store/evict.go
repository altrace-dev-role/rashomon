package store

import (
	"errors"
	"fmt"
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
	unlock, err := lockCounted(gf, lockBudget)
	if err != nil {
		return nil, err
	}
	defer unlock()

	runsDir := filepath.Join(s.root, dirRuns)
	cands, total, err := evictCandidates(runsDir)
	if err != nil || total <= capBytes {
		return nil, err
	}

	protectDir := segment(protect)
	var (
		gaps []Gap
		errs []error
	)
	for _, c := range cands {
		if total <= capBytes {
			break
		}
		if c.name == protectDir || now.Sub(c.mtime) < evictGrace {
			continue
		}

		// A run that cannot be read is still removed, by size: its gap says
		// how many records went is unknown, and the read error is returned
		// with whatever else the pass did. Stopping here, as it used to, kept
		// the oldest run -- and so every run -- in the store for good.
		g, err := s.evictionGap(c, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %s: %w", c.name, err))
		}

		// The gap lands before the run goes, for the same reason as in forget.
		line, err := marshalLine(g)
		if err != nil {
			return gaps, errors.Join(append(errs, err)...)
		}
		if err := appendHealed(gf, line); err != nil {
			return gaps, errors.Join(append(errs, err)...)
		}
		if err := syncFull(gf); err != nil {
			return gaps, errors.Join(append(errs, err)...)
		}
		if err := os.RemoveAll(filepath.Join(runsDir, c.name)); err != nil {
			return gaps, errors.Join(append(errs, err)...)
		}

		total -= c.size
		gaps = append(gaps, g)
	}
	return gaps, errors.Join(errs...)
}

// RemovedUnknown is Gap.RemovedRecords for a run eviction removed without
// being able to count it: a run it could not read, or one holding a line it
// could not parse, whose kind is not known.
const RemovedUnknown = -1

// evictCandidate is one run eviction may remove: its size on disk and the
// newest modification time among its files.
type evictCandidate struct {
	name  string
	size  int64
	mtime time.Time
}

// evictCandidates lists every run with its size, oldest first, and the
// store's total. A store with no runs directory has none.
func evictCandidates(runsDir string) ([]evictCandidate, int64, error) {
	entries, err := os.ReadDir(runsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var (
		cands []evictCandidate
		total int64
	)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c := evictCandidate{name: e.Name()}
		files, err := os.ReadDir(filepath.Join(runsDir, e.Name()))
		if err != nil {
			return nil, 0, err
		}
		for _, f := range files {
			info, err := f.Info()
			if err != nil {
				return nil, 0, err
			}
			c.size += info.Size()
			if info.ModTime().After(c.mtime) {
				c.mtime = info.ModTime()
			}
		}
		cands = append(cands, c)
		total += c.size
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mtime.Before(cands[j].mtime) })
	return cands, total, nil
}

// evictionGap is the gap record for evicting one run, with the error that
// kept it from reading the run, if any. A run it could not read gets a gap
// that names its directory, spans its modification time and counts
// RemovedUnknown records.
func (s *Store) evictionGap(c evictCandidate, now time.Time) (Gap, error) {
	run, err := s.ReadRunDir(c.name)
	if err != nil {
		run = &Run{Dir: c.name}
	}
	from, to := recordSpan(run, c.mtime)
	g := Gap{
		Type:           TypeGap,
		SchemaVersion:  SessionSchemaVersion,
		RecordedAtMS:   now.UnixMilli(),
		SessionID:      run.SessionID(),
		Reason:         GapSizeCap,
		FromUnixMS:     from,
		ToUnixMS:       to,
		RemovedRecords: len(run.Declarations) + len(run.Executions) + len(run.Terminals),
	}
	if err != nil || run.Skipped > 0 {
		g.RemovedRecords = RemovedUnknown
	}
	return g, err
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

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
// is still live. Eviction takes no run's records lock, so it can race an
// append to a run idle for longer than this; the package doc states that as
// an accepted risk.
const evictGrace = time.Hour

// Evict removes whole runs, oldest first, until the store fits under capBytes.
// The named session and any run written to within evictGrace are never
// candidates. Every removed run leaves a gap record. A cap of zero turns
// eviction off (CapBytes), and so does a negative one, which CapBytes never
// returns.
func (s *Store) Evict(capBytes int64, protect string, now time.Time) ([]Gap, error) {
	if err := s.opened(); err != nil {
		return nil, err
	}
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
	// An eviction runs after the invocation's coverage record, where its
	// lock waits are noted (noteLockWait), so this wait has nowhere to go.
	unlock, _, err := lockCounted(gf, s.lockWait(lockBudget))
	if err != nil {
		return nil, err
	}
	defer unlock()

	runsDir := filepath.Join(s.root, dirRuns)
	plan, err := evictCandidates(runsDir)
	if err != nil {
		return nil, err
	}
	total, errs := plan.total, plan.unlisted
	if total <= capBytes {
		return nil, errors.Join(errs...)
	}

	protectDir := segment(protect)
	var gaps []Gap
	for _, c := range plan.cands {
		if total <= capBytes {
			break
		}
		// A probe's deadline is shared across every lock wait it makes
		// (LimitLockWaits); eviction runs inside that same hook invocation.
		// A deep backlog must not make one pass run past it looking for
		// more to remove, where the hook's own timeout could cut a removal
		// short mid-delete. Checked between runs, never inside one, so a
		// run this pass starts removing always finishes. What is left
		// waits for the next probe.
		if !s.waitUntil.IsZero() && time.Now().After(s.waitUntil) {
			break
		}
		if c.name == protectDir || now.Sub(c.mtime) < evictGrace {
			continue
		}

		// A run that cannot be read is still removed, by size: its gap says
		// how many records went is unknown, and the read error is returned
		// with whatever else the pass did. Stopping here, as it used to, kept
		// the oldest run -- and so every run -- in the store for good.
		//
		// A refused read (Windows's mandatory lock, s.refusal()) is not that
		// kind of error: it means a hook holds this run's lock right now,
		// which is a live session idle past evictGrace rather than a run
		// this pass can safely characterize. It is left for the next pass,
		// the same way an unlistable run already was.
		g, err := s.evictionGap(c, now)
		if err != nil && s.refusal()(err) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("run %s: %w", c.name, err))
		}

		// The gap lands before the run goes, for the same reason as in forget.
		line, err := marshalLine(g)
		if err != nil {
			return gaps, errors.Join(append(errs, err)...)
		}
		if err := appendHealed(gf, line, true); err != nil {
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

// evictPlan is every run eviction may remove, oldest first, with the store's
// total size and the error of each run it could not list.
type evictPlan struct {
	cands    []evictCandidate
	total    int64
	unlisted []error
}

// evictCandidates plans an eviction. A store with no runs directory has no
// candidates.
//
// A run whose directory cannot be listed is left out, with its error in
// unlisted: its size and its age are both unknown, so removing it could take
// a session still being written, and stopping at it, as this used to, would
// keep every run after it in the store too.
func evictCandidates(runsDir string) (evictPlan, error) {
	var plan evictPlan
	entries, err := os.ReadDir(runsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return plan, nil
	}
	if err != nil {
		return plan, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c, err := sizeRun(filepath.Join(runsDir, e.Name()))
		if err != nil {
			plan.unlisted = append(plan.unlisted, fmt.Errorf("run %s: %w", e.Name(), err))
			continue
		}
		c.name = e.Name()
		plan.cands = append(plan.cands, c)
		plan.total += c.size
	}
	sort.Slice(plan.cands, func(i, j int) bool { return plan.cands[i].mtime.Before(plan.cands[j].mtime) })
	return plan, nil
}

// sizeRun is a run directory's size and newest modification time.
func sizeRun(dir string) (evictCandidate, error) {
	var c evictCandidate
	files, err := os.ReadDir(dir)
	if err != nil {
		return c, err
	}
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			return c, err
		}
		c.size += info.Size()
		if info.ModTime().After(c.mtime) {
			c.mtime = info.ModTime()
		}
	}
	return c, nil
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
	g := NewGap(GapSizeCap, run.SessionID(), now)
	g.FromUnixMS, g.ToUnixMS = from, to
	g.RemovedRecords = len(run.Declarations) + len(run.Executions) + len(run.Terminals)
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

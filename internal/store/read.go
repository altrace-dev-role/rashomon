package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Run is everything recorded for one session.
type Run struct {
	// Dir is the run directory's name under runs/, which is the session id
	// when that id is a plain path element and a digest of it otherwise.
	Dir          string
	Declarations []Declaration
	Executions   []Execution
	Terminals    []Terminal
	Coverage     []Coverage
	// Skipped counts lines that did not parse, carried an unknown type, or
	// carried a schema version this reader does not understand. They are
	// counted rather than failed on so that one bad line cannot hide a run.
	Skipped int
}

// SessionID is the session id the run's records carry, or the directory name
// when the run holds no records.
func (r *Run) SessionID() string {
	if len(r.Declarations) > 0 {
		return r.Declarations[0].SessionID
	}
	if len(r.Terminals) > 0 {
		return r.Terminals[0].SessionID
	}
	if len(r.Coverage) > 0 {
		return r.Coverage[0].SessionID
	}
	return r.Dir
}

// Runs lists run directory names, oldest first by modification time.
func (s *Store) Runs() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, dirRuns))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	type stamped struct {
		name  string
		mtime int64
	}
	var runs []stamped
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		runs = append(runs, stamped{e.Name(), info.ModTime().UnixNano()})
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].mtime < runs[j].mtime })

	names := make([]string, len(runs))
	for i, r := range runs {
		names[i] = r.name
	}
	return names, nil
}

// ReadRun reads one session's records by session id.
func (s *Store) ReadRun(sessionID string) (*Run, error) {
	return s.ReadRunDir(segment(sessionID))
}

// ReadRunDir reads one session's records by run directory name.
func (s *Store) ReadRunDir(name string) (*Run, error) {
	dir := filepath.Join(s.root, dirRuns, name)
	run := &Run{Dir: name}

	for _, fname := range []string{FileRecords, FileSpill, FileCoverage} {
		if err := eachLine(filepath.Join(dir, fname), func(line []byte) { classifyRunLine(run, line) }); err != nil {
			return nil, err
		}
	}
	return run, nil
}

// ReadRunConsistent reads one session's records the same way ReadRunDir does,
// except each file is read under a short, best-effort lock against the same
// flock a writer takes in appendOrdered/appendLine.
//
// ReadRunDir takes no lock at all, which suits its callers (the package doc
// names them): each reads a run after the fact, or accepts a line caught
// mid-write as one counted in Skipped. A turn digest is read at Stop, while a
// backgrounded tool call or a concurrent subagent can still be appending to
// this exact run directory, and a reader with no lock can observe a line
// while its write is in flight.
//
// budget is deliberately NOT lockBudget (2s): that number bounds how long a
// WRITER may starve a hook's own record, and a reader borrowing it could stall
// for two seconds on every call, which is the opposite of a command with a
// 50ms ceiling. It is one deadline for the whole read, not a wait per file:
// each file waits for the time left, and a file with none left tries its
// lock once, so three held files cost what one does and a single held file
// may take all of it (perf-1). It should wait out one writer's hold, which
// BenchmarkOrderedAppendHold measures (lockbudget.go); digest's
// readLockBudget is derived from it. A lock this reader cannot get within
// budget is not fatal -- the read proceeds without it, exactly as ReadRunDir
// always has, and a line actually caught mid-write still lands in Skipped
// rather than being counted as something it is not.
func (s *Store) ReadRunConsistent(sessionID string, budget time.Duration) (*Run, error) {
	name := segment(sessionID)
	dir := filepath.Join(s.root, dirRuns, name)
	run := &Run{Dir: name}
	deadline := time.Now().Add(budget)

	for _, fname := range []string{FileRecords, FileSpill, FileCoverage} {
		left := max(0, time.Until(deadline))
		err := eachLineTryLock(filepath.Join(dir, fname), left, func(line []byte) { classifyRunLine(run, line) })
		if err != nil {
			return nil, err
		}
	}
	return run, nil
}

// classifyRunLine parses one NDJSON line into the run it belongs to. Shared by
// ReadRunDir and ReadRunConsistent so the two read paths cannot drift on what
// a record type or an unrecognised schema version means.
func classifyRunLine(run *Run, line []byte) {
	var head struct {
		Type          string `json:"type"`
		SchemaVersion int    `json:"schema_version"`
	}
	// Accepts, not equality against SchemaVersion: a store written before
	// the v2 fields existed is still readable, and equality here would have
	// silently skipped every record already on disk the moment the writer
	// moved on.
	if json.Unmarshal(line, &head) != nil || !Accepts(head.SchemaVersion) {
		run.Skipped++
		return
	}
	switch head.Type {
	case TypeDeclaration:
		var rec Declaration
		if json.Unmarshal(line, &rec) != nil {
			run.Skipped++
			return
		}
		run.Declarations = append(run.Declarations, rec)
	case TypeExecution:
		var rec Execution
		if json.Unmarshal(line, &rec) != nil {
			run.Skipped++
			return
		}
		run.Executions = append(run.Executions, rec)
	case TypeTerminal:
		var rec Terminal
		if json.Unmarshal(line, &rec) != nil {
			run.Skipped++
			return
		}
		run.Terminals = append(run.Terminals, rec)
	case TypeCoverage:
		var rec Coverage
		if json.Unmarshal(line, &rec) != nil {
			run.Skipped++
			return
		}
		run.Coverage = append(run.Coverage, rec)
	default:
		run.Skipped++
	}
}

// ReadGaps reads every gap record in the store, and counts the lines it
// could not read as one: damaged, too long, or not a gap.
//
// The count is the gaps' Run.Skipped, and a reader must not drop it. A gap
// line that does not parse cannot be attributed to a session -- its session
// id is part of what failed to read -- so a reader marks every session
// unverified when it is not zero (REC-33). Dropping the line silently, as
// this did, let the one record saying "records were removed here" vanish,
// and the session it described read verified.
func (s *Store) ReadGaps() (gaps []Gap, skipped int, err error) {
	err = eachLine(filepath.Join(s.root, FileGaps), func(line []byte) {
		var g Gap
		if json.Unmarshal(line, &g) != nil || g.Type != TypeGap {
			skipped++
			return
		}
		gaps = append(gaps, g)
	})
	return gaps, skipped, err
}

// eachLine calls fn for every non-empty line of an NDJSON file. A missing file
// has no lines, which is a real and distinct outcome from an empty one.
func eachLine(path string, fn func(line []byte)) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only
	return scanLines(f, fn)
}

// eachLineTryLock is eachLine under a short, best-effort exclusive lock against
// the same flock a writer takes -- see ReadRunConsistent's doc for why a
// digest needs this and report never has.
//
// It takes the lock itself, and tries rather than requires it: the name says
// so, because by the Go convention a name ending in Locked is a function
// that expects its caller to hold the lock already (naming-8).
//
// A lock this reader could not get within budget is not treated as a reason
// to fail: the scan below then runs exactly as eachLine's always has, and a
// line actually caught mid-write still lands in Skipped rather than being
// silently mis-read as something it is not.
func eachLineTryLock(path string, budget time.Duration, fn func(line []byte)) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only

	if unlock, lerr := lockCounted(f, budget); lerr == nil {
		defer unlock()
	}
	return scanLines(f, fn)
}

// scanLines is the NDJSON scan itself, shared by eachLine and eachLineTryLock
// so the two differ only in whether a lock was taken first. A line too long
// to read reaches fn as nil (forEachLine), which every caller counts as a
// line that does not parse.
func scanLines(f *os.File, fn func(line []byte)) error {
	return forEachLine(f, fn)
}

// Unterminated returns the tool_use_ids of declarations that no terminal
// record closes: the signature of a handler that was killed between the two.
// A flush_failed marker closes nothing (Terminal.FlushFailed).
func (r *Run) Unterminated() []string {
	closed := map[string]bool{}
	for _, t := range r.Terminals {
		if !t.FlushFailed() {
			closed[t.ToolUseID] = true
		}
	}
	var ids []string
	for _, d := range r.Declarations {
		if !closed[d.ToolUseID] {
			ids = append(ids, d.ToolUseID)
		}
	}
	return ids
}

// Unexecuted returns the tool_use_ids of declarations that no execution record
// names: a call that was denied, that failed, or whose PostToolUse invocation
// did not record one. Which of the three it was is not something this store
// knows, and nothing here narrows it down.
func (r *Run) Unexecuted() []string {
	executed := map[string]bool{}
	for _, x := range r.Executions {
		executed[x.ToolUseID] = true
	}
	var ids []string
	for _, d := range r.Declarations {
		if !executed[d.ToolUseID] {
			ids = append(ids, d.ToolUseID)
		}
	}
	return ids
}

// Dropped returns the tool_use_ids of terminals that close no declaration: a
// declaration that could not be written, whose terminal carried its id to the
// spill file so the drop is visible under its name. A flush_failed marker is
// not such a terminal (Terminal.FlushFailed).
func (r *Run) Dropped() []string {
	declared := map[string]bool{}
	for _, d := range r.Declarations {
		declared[d.ToolUseID] = true
	}
	var ids []string
	for _, t := range r.Terminals {
		if !declared[t.ToolUseID] && !t.FlushFailed() {
			ids = append(ids, t.ToolUseID)
		}
	}
	return ids
}

// NotDurable returns the tool_use_ids flush_failed markers name, each once,
// in the order the markers were read: calls whose records a hook or post
// appended and then could not make durable (REC-34 (c)). A marker naming no
// call names nothing here; the report still marks its session.
//
// Nor does one naming a call with no record of any kind. A marker's own
// write is the LAST thing a failing invocation does: the record it speaks
// for -- Handler's declaration, or Post's execution, per the marker's Phase
// -- is appended earlier in the same process, through the same ordered-append
// path (appendOrdered). When that append's own barrier is what fails -- the
// shared seq counter's, ahead of the record's own write -- the record is
// never written at all, and flushOrMark still fires on the sticky failure
// Flush now returns. Listing that id here would call a declaration or
// execution possibly not durable when there never was one to lose. Dropped
// excludes a flush_failed marker for the complementary reason, so the two
// lists never both name the same call.
func (r *Run) NotDurable() []string {
	hasRecord := map[string]bool{}
	for _, d := range r.Declarations {
		hasRecord[d.ToolUseID] = true
	}
	for _, x := range r.Executions {
		hasRecord[x.ToolUseID] = true
	}
	seen := map[string]bool{}
	var ids []string
	for _, t := range r.Terminals {
		if t.FlushFailed() && t.ToolUseID != "" && hasRecord[t.ToolUseID] && !seen[t.ToolUseID] {
			seen[t.ToolUseID] = true
			ids = append(ids, t.ToolUseID)
		}
	}
	return ids
}

// NewestRun returns the run directory most recently written to, or "" when the
// store holds none.
//
// Most recently WRITTEN rather than most recently created, because a session's
// directory is created at its first tool call and appended to for as long as it
// runs: two overlapping sessions would otherwise be ordered by which started
// first rather than which just finished.
//
// It exists for callers that act without ever being told Claude Code's
// session id -- `recap pending`'s check at the start of a new prompt, and
// `digest`'s fallback when none was passed -- each of which only knows that
// whatever ran last is what finished.
//
// The modification time is returned with the name because "whatever ran last"
// is only the right answer when something ran at all. A caller that launched a
// child has to be able to tell a session that child produced from one that was
// already in the store, and the name alone cannot say.
func (s *Store) NewestRun() (string, time.Time, error) {
	names, err := s.Runs()
	if err != nil {
		return "", time.Time{}, err
	}
	var newest string
	var newestAt time.Time
	for _, name := range names {
		// The records file rather than the directory: a directory's mtime moves
		// when a child is created, which happens once, while the records file
		// is appended to for the life of the session.
		info, err := os.Stat(filepath.Join(s.root, dirRuns, name, FileRecords))
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestAt) {
			newest, newestAt = name, info.ModTime()
		}
	}
	return newest, newestAt, nil
}

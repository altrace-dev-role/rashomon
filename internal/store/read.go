package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// maxLine bounds a single record on read. Records are small by construction;
// the bound is what stops a corrupted file from being read as one giant line.
const maxLine = 16 << 20

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

	classify := func(line []byte) {
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

	for _, name := range []string{FileRecords, FileSpill, FileCoverage} {
		if err := eachLine(filepath.Join(dir, name), classify); err != nil {
			return nil, err
		}
	}
	return run, nil
}

// ReadGaps reads every gap record in the store.
func (s *Store) ReadGaps() ([]Gap, error) {
	var gaps []Gap
	err := eachLine(filepath.Join(s.root, FileGaps), func(line []byte) {
		var g Gap
		if json.Unmarshal(line, &g) == nil && g.Type == TypeGap {
			gaps = append(gaps, g)
		}
	})
	return gaps, err
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

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		// The scanner reuses its buffer; hand fn a copy it may keep.
		fn(append([]byte(nil), line...))
	}
	return sc.Err()
}

// Unterminated returns the tool_use_ids of declarations that no terminal
// record closes: the signature of a handler that was killed between the two.
func (r *Run) Unterminated() []string {
	closed := map[string]bool{}
	for _, t := range r.Terminals {
		closed[t.ToolUseID] = true
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
// spill file so the drop is visible under its name.
func (r *Run) Dropped() []string {
	declared := map[string]bool{}
	for _, d := range r.Declarations {
		declared[d.ToolUseID] = true
	}
	var ids []string
	for _, t := range r.Terminals {
		if !declared[t.ToolUseID] {
			ids = append(ids, t.ToolUseID)
		}
	}
	return ids
}

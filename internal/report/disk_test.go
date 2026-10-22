package report

import (
	"errors"
	"io/fs"
	"os"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// This package's tests write transcripts to disk and read each one the way
// the read step reads one file: open it, then hand it to the readers this
// package exports. Finding a session's subagent files is the read step's own
// rule (internal/report/gather) and is tested there; nothing here looks for
// them.

// transcriptIDsAt reads one transcript file into a fresh set and returns its
// parts, in the order the path-taking TranscriptIDs used to return them.
func transcriptIDsAt(path string) (ids, results, denied, unhooked map[string]bool, files, tooLong int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}
	defer f.Close() //nolint:errcheck // read-only
	set := NewTranscriptSet()
	if err := TranscriptIDs(f, set); err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}
	return set.IDs, set.Results, set.Denied, set.Unhooked, set.Files, set.TooLong, nil
}

// readsFromDisk reads every transcript path the run's declarations name. A
// path is missing when opening it says it does not exist; nothing here reads
// subagent files, so the main path's error is the only one there is.
func readsFromDisk(run *store.Run) map[string]TranscriptReading {
	out := map[string]TranscriptReading{}
	for _, path := range TranscriptPaths(run) {
		var read TranscriptReading
		if f, err := os.Open(path); err == nil {
			read.Final, read.FinalRead = FinalAssistantText(f)
			f.Close() //nolint:errcheck // read-only
		}
		ids, results, denied, unhooked, files, tooLong, err := transcriptIDsAt(path)
		if err != nil {
			read.Err = err
			read.Missing = errors.Is(err, fs.ErrNotExist)
		} else {
			read.Set = &TranscriptSet{IDs: ids, Results: results, Denied: denied, Unhooked: unhooked, Files: files, TooLong: tooLong}
		}
		out[path] = read
	}
	return out
}

// buildFromDisk is build over the transcripts the run names, read from disk.
func buildFromDisk(run *store.Run) Session { return build(run, readsFromDisk(run)) }

// accountFromDisk is buildAccount over the transcripts the run names.
func accountFromDisk(run *store.Run) Account { return buildAccount(run, readsFromDisk(run)) }

package report

// This file is the report's one way into the filesystem for transcripts: it
// finds a session's transcript files, opens them, and hands each to a reader
// in transcript.go that takes an io.Reader and opens nothing. Keeping every
// open here is what lets digest and spend use the readers and the verdict
// helpers without inheriting a path to a file they promise not to read
// (TestRubricHLD4_OnlyTheAdapterFilesReachTheFilesystem).

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TranscriptIDs reads the distinct tool_use ids from a session transcript and
// from its subagent transcripts, which live at <session>/subagents/agent-*.jsonl
// where <session> is the transcript path without its extension, the distinct
// ids of the tool_result blocks answering them, which of those results are
// the user having DENIED the call, and which have the shape of a call Claude
// Code refused before any hook fired (unhooked; see toolUseErrorTag). A
// transcript cannot show whether a hook fired, so unhooked only marks the
// shape; the report confirms it against the store.
//
// The sets are kept apart because they mean different things: a tool_use block
// is a call the model asked for, a tool_result block is that call having
// finished, and a denial is neither -- the call never ran, and the user is why.
//
// DENIALS ARE NOT RESULTS. A denial does produce a tool_result block, so
// counting it as one put every denied call into executed-but-unrecorded, a list
// that exists to surface the recorder having failed to fire. The user
// exercising the permission prompt then read as the tool being broken.
//
// It reads five fields from each content block -- type, id, tool_use_id,
// is_error and content -- and nothing else. Content is read to classify the
// result and is never retained. A line that does not parse is skipped, not
// fatal: the transcript is Claude Code's file and its shape is not this
// program's to enforce.
func TranscriptIDs(path string) (ids, results, denied, unhooked map[string]bool, files int, err error) {
	ids, results, denied, unhooked = map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}

	if err := collectIDsFile(path, ids, results, denied, unhooked); err != nil {
		return nil, nil, nil, nil, 0, err
	}
	files++

	matches, err := subagentTranscripts(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents"))
	if err != nil {
		return nil, nil, nil, nil, 0, err
	}
	for _, m := range matches {
		if err := collectIDsFile(m, ids, results, denied, unhooked); err != nil {
			return nil, nil, nil, nil, 0, err
		}
		files++
	}
	return ids, results, denied, unhooked, files, nil
}

// subagentTranscripts lists every agent-*.jsonl under dir, at any depth.
//
// A plain subagent writes <session>/subagents/agent-*.jsonl, but a workflow's
// subagents write <session>/subagents/workflows/<run>/agent-*.jsonl, two levels
// further down. A one-level glob read 35 of a real session's 407 transcripts,
// and every call the other 372 made then read as recorded-but-not-in-the-
// transcript -- a gap that was this function's, not the recorder's. A missing
// directory is the common case, a session with no subagents, and not an error.
func subagentTranscripts(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match("agent-*.jsonl", d.Name()); ok {
			out = append(out, p)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	sort.Strings(out)
	return out, err
}

// collectIDsFile opens one transcript file and reads it with collectIDs.
func collectIDsFile(path string, into, results, denied, unhooked map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only
	return collectIDs(f, into, results, denied, unhooked)
}

// finalAssistantTextOfFile opens a transcript and reads its final assistant
// text with FinalAssistantText. A file that cannot be opened has none.
func finalAssistantTextOfFile(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close() //nolint:errcheck // read-only
	return FinalAssistantText(f)
}

package report

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxLine bounds one transcript line. Transcript lines carry whole messages
// and can be large; the bound is against a corrupted file, not a real one.
const maxLine = 64 << 20

// TranscriptIDs reads the distinct tool_use ids from a session transcript and
// from its subagent transcripts, which live at <session>/subagents/agent-*.jsonl
// where <session> is the transcript path without its extension, and the
// distinct ids of the tool_result blocks answering them.
//
// The two sets are kept apart because they mean different things: a tool_use
// block is a call the model asked for, and a tool_result block is that call
// having finished. A transcript routinely holds the first without the second,
// for a call that was denied, that failed, or that the file has not caught up
// with yet.
//
// It reads three fields from each content block -- type, id and tool_use_id --
// and nothing else. A line that does not parse is skipped, not fatal: the
// transcript is Claude Code's file and its shape is not this program's to
// enforce.
func TranscriptIDs(path string) (ids, results map[string]bool, files int, err error) {
	ids, results = map[string]bool{}, map[string]bool{}

	n, err := collectIDs(path, ids, results)
	if err != nil {
		return nil, nil, 0, err
	}
	files += n

	pattern := filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "agent-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, 0, err
	}
	for _, m := range matches {
		n, err := collectIDs(m, ids, results)
		if err != nil {
			return nil, nil, 0, err
		}
		files += n
	}
	return ids, results, files, nil
}

func collectIDs(path string, into, results map[string]bool) (int, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	if err != nil {
		return 0, err
	}
	defer f.Close() //nolint:errcheck // read-only

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), maxLine)
	for sc.Scan() {
		var line struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil || len(line.Message.Content) == 0 {
			continue
		}
		var blocks []struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			ToolUseID string `json:"tool_use_id"`
		}
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			continue // a string content, or a shape this reader does not know
		}
		for _, b := range blocks {
			switch {
			case b.Type == "tool_use" && b.ID != "":
				into[b.ID] = true
			case b.Type == "tool_result" && b.ToolUseID != "":
				results[b.ToolUseID] = true
			}
		}
	}
	return 1, sc.Err()
}

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

// deniedPrefix is the text Claude Code writes when the USER refuses a tool
// call, and the only thing that separates a denial from an ordinary failure.
//
// There is no structured signal. A denial and a failed command both arrive as a
// tool_result with is_error true; the difference is this sentence, and it is
// matched as a PREFIX because the three variants seen in real transcripts share
// only their opening. Measured across 1,858 transcripts: 14 denials, three
// endings -- "STOP what you are doing...", the same plus a memory note, and
// "To tell you how to proceed, the user said: ...".
//
// Anchored at the start, and never matched on its own. A command whose output
// happens to contain this sentence RAN, and calling that a denial would hide a
// real execution behind a user decision. That is not hypothetical: the search
// that established this prefix printed it, and its own output came back as a
// tool_result in the same session.
const deniedPrefix = "The user doesn't want to proceed with this tool use. The tool use was rejected"

// TranscriptIDs reads the distinct tool_use ids from a session transcript and
// from its subagent transcripts, which live at <session>/subagents/agent-*.jsonl
// where <session> is the transcript path without its extension, the distinct
// ids of the tool_result blocks answering them, and which of those results are
// the user having DENIED the call.
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
func TranscriptIDs(path string) (ids, results, denied map[string]bool, files int, err error) {
	ids, results, denied = map[string]bool{}, map[string]bool{}, map[string]bool{}

	n, err := collectIDs(path, ids, results, denied)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	files += n

	pattern := filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "agent-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	for _, m := range matches {
		n, err := collectIDs(m, ids, results, denied)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		files += n
	}
	return ids, results, denied, files, nil
}

// resultText renders a tool_result's content for classification only.
//
// Content is a plain string in some messages and an array of blocks in others,
// and both shapes occur in real transcripts, so both are handled rather than
// one being assumed. Only the leading text is needed, and nothing is kept.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			return b.Text
		}
	}
	return ""
}

// isDenial reports whether a tool_result is the user refusing the call.
//
// BOTH halves are required. is_error alone is every failed command; the prefix
// alone is any output that quotes it. Each shortcut is wrong in its own
// direction: the first hides denials among failures, the second hides real
// executions among denials.
func isDenial(isError bool, text string) bool {
	return isError && strings.HasPrefix(text, deniedPrefix)
}

func collectIDs(path string, into, results, denied map[string]bool) (int, error) {
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
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			ToolUseID string          `json:"tool_use_id"`
			IsError   bool            `json:"is_error"`
			Content   json.RawMessage `json:"content"`
		}
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			continue // a string content, or a shape this reader does not know
		}
		for _, b := range blocks {
			switch {
			case b.Type == "tool_use" && b.ID != "":
				into[b.ID] = true
			case b.Type == "tool_result" && b.ToolUseID != "":
				if isDenial(b.IsError, resultText(b.Content)) {
					denied[b.ToolUseID] = true
					continue
				}
				results[b.ToolUseID] = true
			}
		}
	}
	return 1, sc.Err()
}

// FinalAssistantText returns the text of the LAST assistant message in a
// transcript, and whether one was found.
//
// This is the only place in the product that reads message content, and it is
// deliberately a render-time read: the text is never written to the store,
// never logged, and never leaves the machine. The store holds identifiers and
// hostnames; this is read from Claude Code's own file at the moment a human
// asks for a report, and discarded when the process exits.
//
// It is the agent's own account of what it did. Putting it beside the record is
// the entire point of the report -- not to catch the model out, but because a
// summary and a set of records are two descriptions of one session and only a
// reader can reconcile them.
//
// Only text blocks of the last assistant message are taken. A thinking block is
// not the account the user was given, and tool_use blocks are already recorded
// far more precisely on the declaration side.
func FinalAssistantText(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close() //nolint:errcheck // read-only

	var last string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), maxLine)
	for sc.Scan() {
		var line struct {
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.Message.Role != "assistant" || len(line.Message.Content) == 0 {
			continue
		}
		if text, ok := assistantText(line.Message.Content); ok {
			last = text
		}
	}
	if sc.Err() != nil || last == "" {
		return "", false
	}
	return last, true
}

// assistantText joins the text blocks of one assistant message.
//
// Content is either a plain string or an array of blocks, and both shapes occur
// in real transcripts, so both are handled rather than one being assumed.
func assistantText(content json.RawMessage) (string, bool) {
	var plain string
	if json.Unmarshal(content, &plain) == nil {
		if strings.TrimSpace(plain) == "" {
			return "", false
		}
		return plain, true
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return "", false
	}
	var parts []string
	for _, b := range blocks {
		// Text only. A thinking block is not the account the user was given.
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n"), true
}

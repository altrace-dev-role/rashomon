package verify_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

// The transcript's own account of a turn, for the turn replay
// (instrument_replay_test.go). It reads a session's .jsonl transcript alone
// and shares no code with the rule it checks: it imports the standard
// library only and calls nothing it does not declare
// (TestTurnTruth_SharesNoCode), and the five tool names below are its whole
// table. Bash counts as acting whatever its line: telling a line that reads
// from one that writes needs a program table, which is the rule's.

// tFileTools are the tools that write a file by name.
var tFileTools = map[string]bool{"Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true}

// tBash is the shell tool.
const tBash = "Bash"

// tFramedKinds are the origin kinds of a prompt that a harness, not a
// person, wrote; human is typed. Any other kind is reported as a miss and
// counted in neither.
var tFramedKinds = map[string]bool{"peer": true, "task-notification": true, "queued_command": true, "cron": true, "auto-continuation": true}

// tClass is the truth's class of a turn, ordered: a file tool outranks Bash.
type tClass int

const (
	tNoAct    tClass = iota // no main-agent call of a file tool or Bash
	tBashOnly               // Bash, and no file tool
	tFileTool               // a file tool
)

func (c tClass) String() string { return [...]string{"no act", "Bash only", "a file tool"}[c] }

// tOrigin is a record's origin.
type tOrigin struct {
	Kind string `json:"kind"`
}

// tLine is the part of a transcript line the truth reads.
type tLine struct {
	Type        string   `json:"type"`
	Subtype     string   `json:"subtype"`
	PromptID    string   `json:"promptId"`
	IsSidechain bool     `json:"isSidechain"`
	Origin      *tOrigin `json:"origin"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Attachment *struct {
		Type   string   `json:"type"`
		Origin *tOrigin `json:"origin"`
	} `json:"attachment"`
}

// tBlock is one content block of a message.
type tBlock struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id"`
	Text      string `json:"text"`
}

// tCut is the first n lines of a transcript, each with its newline, and
// their sha256 in hex. A transcript shorter than n lines is an error.
func tCut(r io.Reader, n int) (prefix []byte, sum string, err error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	end := 0
	for i := 0; i < n; i++ {
		j := bytes.IndexByte(b[end:], '\n')
		if j < 0 {
			return nil, "", fmt.Errorf("the transcript holds %d whole lines, fewer than %d", i, n)
		}
		end += j + 1
	}
	h := sha256.Sum256(b[:end])
	return b[:end], hex.EncodeToString(h[:]), nil
}

// tRead parses a transcript, one record per line; a line that does not
// parse fails the read.
func tRead(r io.Reader) ([]tLine, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	var ls []tLine
	for n := 1; sc.Scan(); n++ {
		var l tLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, fmt.Errorf("transcript line %d: %w", n, err)
		}
		ls = append(ls, l)
	}
	return ls, sc.Err()
}

// tBlocks is a message's content as blocks; a string content holds none.
func tBlocks(l *tLine) []tBlock {
	if len(l.Message.Content) == 0 || l.Message.Content[0] != '[' {
		return nil
	}
	var bs []tBlock
	if json.Unmarshal(l.Message.Content, &bs) != nil {
		return nil
	}
	return bs
}

// tAnswers reports a user record that answers a tool call.
func tAnswers(l *tLine) bool {
	for _, b := range tBlocks(l) {
		if b.Type == "tool_result" {
			return true
		}
	}
	return false
}

// tKind is a record's origin kind, "none" where it carries none.
func tKind(o *tOrigin) string {
	if o == nil || o.Kind == "" {
		return "none"
	}
	return o.Kind
}

// tTurn is the truth for the turn a last-turn note is about. The note's
// prompt is the first user record carrying the note's prompt id; the turn is
// the prompt id of the last user record before the stop_hook_summary record
// that precedes it. Its class is read off the main agent's tool_use calls at
// or before that Stop record, each joined to its turn by the prompt id of
// the tool_result that answers it; a call on a sidechain's record is never
// the main agent's. called is whether the turn held any main-agent call; ok
// is false where the transcript does not place the note.
func tTurn(ls []tLine, notePrompt string) (turn string, class tClass, called, ok bool) {
	p := -1
	for i := range ls {
		if ls[i].Type == "user" && ls[i].PromptID == notePrompt {
			p = i
			break
		}
	}
	s := -1
	for i := p - 1; i >= 0; i-- {
		if ls[i].Type == "system" && ls[i].Subtype == "stop_hook_summary" {
			s = i
			break
		}
	}
	u := -1
	for i := s - 1; i >= 0; i-- {
		if ls[i].Type == "user" && ls[i].PromptID != "" {
			u = i
			break
		}
	}
	if p < 0 || s < 0 || u < 0 {
		return "", tNoAct, false, false
	}
	turn = ls[u].PromptID
	answeredIn := map[string]string{} // a tool_use id -> the prompt id of its tool_result
	for i := 0; i <= s; i++ {
		if ls[i].Type != "user" {
			continue
		}
		for _, b := range tBlocks(&ls[i]) {
			if b.Type == "tool_result" {
				answeredIn[b.ToolUseID] = ls[i].PromptID
			}
		}
	}
	for i := 0; i <= s; i++ {
		if ls[i].Type != "assistant" || ls[i].IsSidechain {
			continue
		}
		for _, b := range tBlocks(&ls[i]) {
			if b.Type != "tool_use" || answeredIn[b.ID] != turn {
				continue
			}
			called = true
			switch {
			case tFileTools[b.Name]:
				class = tFileTool
			case b.Name == tBash && class < tBashOnly:
				class = tBashOnly
			}
		}
	}
	return turn, class, called, true
}

// tPrompt is a prompt's own record: the first user record carrying id that
// answers no tool call. kind is its origin's; text is its content, a string
// or its text blocks joined by newlines. ok is false where the transcript
// holds no such record.
func tPrompt(ls []tLine, id string) (kind, text string, ok bool) {
	for i := range ls {
		l := &ls[i]
		if l.Type != "user" || l.PromptID != id || tAnswers(l) {
			continue
		}
		var s string
		if json.Unmarshal(l.Message.Content, &s) == nil {
			return tKind(l.Origin), s, true
		}
		var parts [][]byte
		for _, b := range tBlocks(l) {
			if b.Type == "text" {
				parts = append(parts, []byte(b.Text))
			}
		}
		return tKind(l.Origin), string(bytes.Join(parts, []byte("\n"))), true
	}
	return "", "", false
}

// tPromptKinds counts the transcript's prompt-side records by origin kind:
// user records that answer no tool call, and queued_command attachments
// (keyed "queued_command/" and the kind).
func tPromptKinds(ls []tLine) map[string]int {
	n := map[string]int{}
	for i := range ls {
		l := &ls[i]
		switch {
		case l.Type == "user" && !tAnswers(l):
			n[tKind(l.Origin)]++
		case l.Type == "attachment" && l.Attachment != nil && l.Attachment.Type == "queued_command":
			n["queued_command/"+tKind(l.Attachment.Origin)]++
		}
	}
	return n
}

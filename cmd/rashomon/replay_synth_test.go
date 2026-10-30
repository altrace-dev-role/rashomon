package main

import (
	"encoding/json"
	"strings"
	"time"
)

// A synthetic transcript, built line by line from the shapes the 2.1.312
// fixture shows (internal/replay's tests build theirs the same way, and a
// test in another package cannot borrow them). Every line carries a timestamp
// one second after the last, so a test can read the clock it expects off the
// line number.

const (
	tSession = "5e551011-0000-4000-8000-000000000001"
	tPath    = "/home/dev/.claude/projects/-repo/5e551011-0000-4000-8000-000000000001.jsonl"
)

var tEpoch = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

// tdoc is a transcript under construction.
type tdoc struct {
	lines  []string
	prompt string
	cwd    string
	// sid is the session the lines name, tSession when empty. shift moves every
	// line's time, for a file that begins later than another.
	sid   string
	shift time.Duration
}

func newTdoc() *tdoc { return &tdoc{cwd: "/repo"} }

// at is the timestamp the n-th line (1-based) carries.
func tAt(n int) time.Time { return tEpoch.Add(time.Duration(n) * time.Second) }

func (d *tdoc) add(m map[string]any) int {
	n := len(d.lines) + 1
	m["sessionId"], m["version"] = tSession, "2.1.312"
	if d.sid != "" {
		m["sessionId"] = d.sid
	}
	if _, ok := m["timestamp"]; !ok {
		m["timestamp"] = tAt(n).Add(d.shift).Format("2006-01-02T15:04:05.000Z")
	}
	if _, ok := m["cwd"]; !ok {
		m["cwd"] = d.cwd
	}
	if _, ok := m["isSidechain"]; !ok {
		m["isSidechain"] = false
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	d.lines = append(d.lines, string(b))
	return n
}

// raw appends a line as it is.
func (d *tdoc) raw(s string) int { d.lines = append(d.lines, s); return len(d.lines) }

func (d *tdoc) promptLine(id string) int {
	d.prompt = id
	return d.add(map[string]any{"type": "user", "promptId": id, "permissionMode": "default",
		"message": map[string]any{"role": "user", "content": "do the thing"}})
}

type tUse struct {
	id, name string
	input    map[string]any
}

func tBash(id, cmd string) tUse { return tUse{id, "Bash", map[string]any{"command": cmd}} }

func tWrite(id, path string) tUse {
	return tUse{id, "Write", map[string]any{"file_path": path, "content": "x"}}
}

// uses is an assistant line holding tool_use blocks, stop_reason tool_use.
func (d *tdoc) uses(msg string, us ...tUse) int { return d.usesIn("", msg, us...) }

// usesIn is uses with the line's cwd given ("" for the document's).
func (d *tdoc) usesIn(cwd, msg string, us ...tUse) int { return d.usesWith(cwd, nil, msg, us...) }

// usesWith is usesIn with more keys on the line (a sidechain's, say).
func (d *tdoc) usesWith(cwd string, extra map[string]any, msg string, us ...tUse) int {
	var blocks []any
	for _, u := range us {
		blocks = append(blocks, map[string]any{"type": "tool_use", "id": u.id, "name": u.name, "input": u.input})
	}
	m := map[string]any{"type": "assistant",
		"message": map[string]any{"id": msg, "role": "assistant", "stop_reason": "tool_use", "content": blocks}}
	if cwd != "" {
		m["cwd"] = cwd
	}
	for k, v := range extra {
		m[k] = v
	}
	return d.add(m)
}

type tResult struct {
	id      string
	isError bool
	text    string
	tur     any
}

func tOK(id string) tResult {
	return tResult{id: id, text: "fine", tur: map[string]any{"stdout": "fine"}}
}

func tFail(id string) tResult {
	return tResult{id: id, isError: true, text: "Exit code 1\nFAILED (failures=1)", tur: "Error: Exit code 1\nFAILED"}
}

// results is a user line holding one tool_result.
func (d *tdoc) results(cwd string, r tResult) int { return d.resultsWith(cwd, nil, r) }

// resultsWith is results with more keys on the line.
func (d *tdoc) resultsWith(cwd string, extra map[string]any, r tResult) int {
	blocks := []any{map[string]any{"type": "tool_result", "tool_use_id": r.id, "is_error": r.isError, "content": r.text}}
	m := map[string]any{"type": "user", "promptId": d.prompt, "message": map[string]any{"role": "user", "content": blocks}}
	if r.tur != nil {
		m["toolUseResult"] = r.tur
	}
	if cwd != "" {
		m["cwd"] = cwd
	}
	for k, v := range extra {
		m[k] = v
	}
	return d.add(m)
}

// ends is the assistant line that ends a turn.
func (d *tdoc) ends(msg, text string) int { return d.endsAs(msg, text, "end_turn") }

// endsAs is ends with the stop_reason given: anything but end_turn is a turn
// the transcript does not show ending.
func (d *tdoc) endsAs(msg, text, stopReason string) int {
	return d.add(map[string]any{"type": "assistant",
		"message": map[string]any{"id": msg, "role": "assistant", "stop_reason": stopReason,
			"content": []any{map[string]any{"type": "text", "text": text}}}})
}

func (d *tdoc) String() string { return strings.Join(d.lines, "\n") + "\n" }

func (d *tdoc) reader() *strings.Reader { return strings.NewReader(d.String()) }

// prefix is the document's first n lines, as a document of its own.
func (d *tdoc) prefix(n int) *strings.Reader {
	return strings.NewReader(strings.Join(d.lines[:n], "\n") + "\n")
}

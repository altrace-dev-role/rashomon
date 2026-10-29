package replay

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

// A synthetic transcript is built line by line from the shapes the 2.1.312
// fixture shows, and keeps its own account of where each thing is, so a test
// can state the timeline it expects without reading the replay's output.

const (
	synthSession = "s1"
	synthPath    = "/home/dev/.claude/projects/-repo/s1.jsonl"
)

// synthTime is the timestamp the n-th line (1-based) of a synthetic transcript
// carries: one second apart, so an event's time says which line it is from.
func synthTime(n int) time.Time {
	return time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Second)
}

// doc is a transcript under construction.
type doc struct {
	lines  []string
	prompt string // the current prompt's id
	cwd    string
	// want is the timeline the builder expects, in file order.
	want []wantEvent
}

type wantEvent struct {
	Kind Kind
	At   history.Pos
	ID   string // tool_use_id
}

func newDoc() *doc { return &doc{cwd: "/repo"} }

func (d *doc) add(m map[string]any) int {
	m["sessionId"], m["version"] = synthSession, "2.1.312"
	if _, ok := m["timestamp"]; !ok {
		m["timestamp"] = synthTime(len(d.lines) + 1).Format("2006-01-02T15:04:05.000Z")
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
	return len(d.lines)
}

// raw appends a line as it is: blank, cut, or anything else.
func (d *doc) raw(s string) int { d.lines = append(d.lines, s); return len(d.lines) }

// prompt is a typed prompt, which starts a turn.
func (d *doc) promptLine(id string) int {
	d.prompt = id
	n := d.add(map[string]any{"type": "user", "promptId": id, "permissionMode": "default",
		"message": map[string]any{"role": "user", "content": "do the thing"}})
	d.want = append(d.want, wantEvent{UserPromptSubmit, history.Pos{Line: n}, ""})
	return n
}

type toolUse struct {
	id, name string
	input    map[string]any
}

func bashUse(id, cmd string) toolUse {
	return toolUse{id, "Bash", map[string]any{"command": cmd}}
}

func writeUse(id, path string) toolUse {
	return toolUse{id, "Write", map[string]any{"file_path": path, "content": "x"}}
}

// uses is an assistant line holding the tool_use blocks, in one line (so the
// blocks are 0, 1, ...). The line says stop_reason tool_use.
func (d *doc) uses(msg string, us ...toolUse) int { return d.usesWith(nil, msg, us...) }

// usesWith is uses with more keys on the line (a sidechain's, say).
func (d *doc) usesWith(extra map[string]any, msg string, us ...toolUse) int {
	var blocks []any
	for _, u := range us {
		blocks = append(blocks, map[string]any{"type": "tool_use", "id": u.id, "name": u.name, "input": u.input})
	}
	m := map[string]any{"type": "assistant",
		"message": map[string]any{"id": msg, "role": "assistant", "stop_reason": "tool_use", "content": blocks}}
	for k, v := range extra {
		m[k] = v
	}
	n := d.add(m)
	for i, u := range us {
		d.want = append(d.want, wantEvent{PreToolUse, history.Pos{Line: n, Block: i}, u.id})
	}
	return n
}

type toolResult struct {
	id      string
	isError bool
	text    string
	// post: the Post event the result gives rise to, 0 for none (a denial).
	post Kind
	// tur is the line's toolUseResult.
	tur any
}

func okResult(id string) toolResult {
	return toolResult{id: id, text: "fine", post: PostToolUse, tur: map[string]any{"stdout": "fine"}}
}

func failResult(id string) toolResult {
	return toolResult{id: id, isError: true, text: "Exit code 1\nboom", post: PostToolUseFailure, tur: "Error: Exit code 1\nboom"}
}

func denyResult(id string) toolResult {
	return toolResult{id: id, isError: true, text: "Permission to use Bash with command rm has been denied.", tur: "Error: denied"}
}

// results is a user line holding tool_result blocks (several only when the
// test means a line with several; toolUseResult is then the first's).
func (d *doc) results(cwd string, rs ...toolResult) int {
	var blocks []any
	for _, r := range rs {
		blocks = append(blocks, map[string]any{"type": "tool_result", "tool_use_id": r.id, "is_error": r.isError, "content": r.text})
	}
	m := map[string]any{"type": "user", "promptId": d.prompt, "message": map[string]any{"role": "user", "content": blocks}}
	switch cwd {
	case "":
	case "-":
		m["cwd"] = "" // a line that names none
	default:
		m["cwd"] = cwd
	}
	if len(rs) == 1 {
		m["toolUseResult"] = rs[0].tur
	}
	n := d.add(m)
	for i, r := range rs {
		if r.post != 0 {
			d.want = append(d.want, wantEvent{r.post, history.Pos{Line: n, Block: i}, r.id})
		}
	}
	return n
}

// ends is the assistant line that ends a turn: one text block, end_turn. The
// Stop stands just past its last block.
func (d *doc) ends(msg, text string) int {
	n := d.add(map[string]any{"type": "assistant",
		"message": map[string]any{"id": msg, "role": "assistant", "stop_reason": "end_turn",
			"content": []any{map[string]any{"type": "text", "text": text}}}})
	d.want = append(d.want, wantEvent{Stop, history.Pos{Line: n, Block: 1}, ""})
	return n
}

// hookNote is the system line a Stop hook leaves behind: not evidence.
func (d *doc) hookNote() int {
	return d.add(map[string]any{"type": "system", "subtype": "stop_hook_summary", "hookCount": 1})
}

func (d *doc) String() string { return strings.Join(d.lines, "\n") + "\n" }

func (d *doc) reader() *strings.Reader { return strings.NewReader(d.String()) }

func (d *doc) build(t *testing.T, lim history.Limits) Timeline {
	t.Helper()
	tl, err := Build(d.reader(), Options{TranscriptPath: synthPath, Limits: lim})
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

// got lists a timeline as the builder's account does.
func got(tl Timeline) []wantEvent {
	var out []wantEvent
	for _, e := range tl.Events {
		out = append(out, wantEvent{e.Kind, e.Pos, e.ToolUseID})
	}
	return out
}

func (w wantEvent) String() string {
	return fmt.Sprintf("%s %s@%d.%d", w.Kind, w.ID, w.At.Line, w.At.Block)
}

func show(ws []wantEvent) string {
	var s []string
	for _, w := range ws {
		s = append(s, w.String())
	}
	return strings.Join(s, "\n  ")
}

// sorted is want in file order, which is how the builder's account is
// compared: the builder appends in the order it writes, which is file order.
func (d *doc) expected() []wantEvent { return d.want }

// history0 is the zero Limits: history.DefaultLimits.
var history0 = history.Limits{}

func mustJSON(m map[string]any) string {
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(b)
}

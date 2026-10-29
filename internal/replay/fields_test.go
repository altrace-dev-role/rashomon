package replay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

const fixtureDir = "../../test/fixtures/claude-code/"

// placeholderPath is the path the tests hand Options, standing for the file
// the caller read. The captured payloads carry Claude Code's own location of
// the session's transcript, which the relocated fixture cannot reproduce.
const placeholderPath = "/replay/caller-path.jsonl"

// captured is one line of a fixture's payloads.ndjson.
type captured struct {
	idx   int
	raw   []byte
	keys  map[string]json.RawMessage
	event string
	id    string
}

func (c *captured) str(key string) string {
	var s string
	_ = json.Unmarshal(c.keys[key], &s)
	return s
}

func loadCaptured(t *testing.T, version string) []captured {
	t.Helper()
	raw, err := os.ReadFile(fixtureDir + version + "/payloads.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	var out []captured
	for i, l := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		c := captured{idx: i, raw: []byte(l), keys: map[string]json.RawMessage{}}
		if err := json.Unmarshal(c.raw, &c.keys); err != nil {
			t.Fatalf("%s payload %d: %v", version, i, err)
		}
		c.event, c.id = c.str("hook_event_name"), c.str("tool_use_id")
		out = append(out, c)
	}
	return out
}

func fixtureFile(t *testing.T, version string) *os.File {
	t.Helper()
	f, err := os.Open(fixtureDir + version + "/transcript.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func buildFixture(t *testing.T, version string) Timeline {
	t.Helper()
	tl, err := Build(fixtureFile(t, version), Options{TranscriptPath: placeholderPath})
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

func compact(t *testing.T, raw []byte) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		t.Fatalf("%q: %v", raw, err)
	}
	return b.String()
}

// backgroundBits is the projection of a tool_response onto the two keys the
// recorder reads: whether each is set (neither null nor false).
func backgroundBits(raw json.RawMessage) [2]bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return [2]bool{}
	}
	set := func(k string) bool {
		v := strings.TrimSpace(string(m[k]))
		return v != "" && v != "null" && v != "false"
	}
	return [2]bool{set("backgroundTaskId"), set("backgroundedByUser")}
}

var claudeLayout = regexp.MustCompile(`^~/\.claude/projects/[^/]+/([0-9a-f-]{36})\.jsonl$`)

// observation is what the table test saw of one field of one kind.
type observation struct {
	classes map[Class]int
	n       map[string]int // by fixture version
}

// TestREPLAY1_FieldCompleteness rebuilds the timeline of both paired
// fixtures from their transcripts alone, matches every event to the payload
// Claude Code captured for it, and classifies every key present in either
// (Fields): rebuilt equal (a), equal after a stated normalisation (b), not
// read (c), read and not in the transcript (d), or read, stored and omitted
// by design (e). Each row of the table is asserted, so a transcript or
// recorder change that moves a field between classes, or a new key in a
// payload, fails here.
//
// SessionStart and SessionEnd are out of scope: they write the liveness
// marker, not evidence, and a transcript records no moment for either. They
// are the first and last captured payloads on both fixtures and are set aside
// after being checked to be so.
//
// The (c) rows rest on cmd/rashomon's TestREPLAY5_, which removes the key
// from the captured payloads and runs the real entry points; this test shows
// the replay does not rebuild them (or rebuilds them equal, for the two
// supplied for shape).
func TestREPLAY1_FieldCompleteness(t *testing.T) {
	seen := map[string]*observation{} // "Kind/key/tools"
	unsupported, events := 0, 0
	for _, version := range []string{"2.1.280", "2.1.312"} {
		caps := loadCaptured(t, version)
		if caps[0].event != "SessionStart" || caps[len(caps)-1].event != "SessionEnd" {
			t.Fatalf("%s: the first and last payloads are %s and %s, not the session's own start and end", version, caps[0].event, caps[len(caps)-1].event)
		}
		caps = caps[1 : len(caps)-1]
		tl := buildFixture(t, version)

		// The sequence and the order: the replay's events are the captured
		// payloads, one for one, in the order they were sent.
		if len(tl.Events) != len(caps) {
			t.Fatalf("%s: %d events, %d captured payloads", version, len(tl.Events), len(caps))
		}
		for i, e := range tl.Events {
			c := &caps[i]
			events++
			if e.Kind.String() != c.event || e.ToolUseID != c.id {
				t.Fatalf("%s event %d: %s %q, captured %s %q", version, i, e.Kind, e.ToolUseID, c.event, c.id)
			}
			if !e.Supported() || e.Gap != "" || !e.Taint.IsZero() || e.BoundaryUnknown {
				unsupported++
				t.Errorf("%s event %d (%s %s): unsupported %v, gap %q, taint %+v", version, i, e.Kind, e.ToolUseID, e.Unsupported, e.Gap, e.Taint)
				continue
			}
			// A denial has a PreToolUse and no Post, on the captured side as
			// on the rebuilt one; no call on a fixture is missing its result.
			if e.Denied != (e.Kind == PreToolUse && !hasPost(caps, e.ToolUseID)) || e.Missing {
				t.Errorf("%s event %d: denied %v missing %v, and the captured payloads hold a Post for the call: %v", version, i, e.Denied, e.Missing, hasPost(caps, e.ToolUseID))
			}
			var rep map[string]json.RawMessage
			if err := json.Unmarshal(e.Payload, &rep); err != nil {
				t.Fatalf("%s event %d: payload %s: %v", version, i, e.Payload, err)
			}
			tool := ""
			if e.ToolName != "" {
				tool = "other"
				if e.ToolName == "Bash" {
					tool = "bash"
				}
			}
			keys := map[string]bool{}
			for k := range c.keys {
				keys[k] = true
			}
			for k := range rep {
				keys[k] = true
			}
			for key := range keys {
				row := findRow(e.Kind, key, tool)
				if row == nil {
					t.Errorf("%s event %d (%s): key %q is in a payload and in no row of the field table", version, i, e.Kind, key)
					continue
				}
				got := classify(t, version, i, e, c, rep, key, row)
				ok := fmt.Sprintf("%s/%s/%s", e.Kind, key, row.Tools)
				o := seen[ok]
				if o == nil {
					o = &observation{classes: map[Class]int{}, n: map[string]int{}}
					seen[ok] = o
				}
				o.classes[got]++
				o.n[version]++
				// A Normalised row may be Equal on a call whose normalisation
				// happens to change nothing.
				if got != row.Class && !(row.Class == Normalised && got == Equal) && !(row.Class == Unread && row.Supplied && got == Equal) {
					t.Errorf("%s event %d (%s %s): key %q observed as class %s, the table says %s", version, i, e.Kind, e.ToolUseID, key, got, row.Class)
				}
			}
		}
	}
	if unsupported != 0 {
		t.Errorf("%d of %d fixture events are unsupported", unsupported, events)
	}

	// Every row of the table was seen, except the Unrecoverable ones, which
	// no fixture can show: they are shown by TestREPLAY2_Unsupported.
	var rows []string
	for _, k := range []Kind{UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure, Stop} {
		for _, f := range Fields(k) {
			id := fmt.Sprintf("%s/%s/%s", k, f.Key, f.Tools)
			o := seen[id]
			switch {
			case f.Class == Unrecoverable:
				if o != nil {
					t.Errorf("%s is Unrecoverable and a fixture payload carries it", id)
				}
			case o == nil:
				t.Errorf("row %s was observed on neither fixture", id)
			default:
				tools := f.Tools
				if tools == "" {
					tools = "any"
				}
				split := ""
				if f.Class == Unread && f.Supplied {
					split = " (supplied, equal)"
				}
				if f.Class == Normalised {
					split = fmt.Sprintf(" (equal %d, normalised %d)", o.classes[Equal], o.classes[Normalised])
				}
				rows = append(rows, fmt.Sprintf("| %s | `%s` | %s%s | %s | %d | %d |", k, f.Key, f.Class, split, tools, o.n["2.1.280"], o.n["2.1.312"]))
			}
		}
	}
	sort.Strings(rows)
	t.Log("| event | key | class | tools | 2.1.280 | 2.1.312 |\n|---|---|---|---|---|---|\n" + strings.Join(rows, "\n"))
}

func hasPost(caps []captured, id string) bool {
	for _, c := range caps {
		if c.id == id && strings.HasPrefix(c.event, "PostToolUse") {
			return true
		}
	}
	return false
}

func findRow(k Kind, key, tool string) *Field {
	for _, f := range Fields(k) {
		if f.Key == key && (f.Tools == "" || f.Tools == tool) {
			f := f
			return &f
		}
	}
	return nil
}

// classify says what became of key on this event, from the two payloads.
func classify(t *testing.T, version string, i int, e Event, c *captured, rep map[string]json.RawMessage, key string, row *Field) Class {
	t.Helper()
	cv, inCap := c.keys[key]
	rv, inRep := rep[key]
	where := fmt.Sprintf("%s event %d (%s %s) key %s", version, i, e.Kind, e.ToolUseID, key)
	switch {
	case inCap && inRep:
		if compact(t, cv) == compact(t, rv) {
			return Equal
		}
		if row.Class == Normalised && normalEqual(t, e, c, key, cv, rv) {
			return Normalised
		}
		t.Errorf("%s: captured %.120s, rebuilt %.120s", where, cv, rv)
		return ""
	case inCap:
		if row.Class == Unread && row.Supplied {
			t.Errorf("%s: a Supplied key was left out", where)
		}
		// Left out of the rebuilt payload.
		if row.Class == Normalised && normalEqual(t, e, c, key, cv, nil) {
			return Normalised
		}
		return row.Class
	default:
		t.Errorf("%s: rebuilt and never sent: %.120s", where, rv)
		return ""
	}
}

// normalEqual applies the normalisation a Normalised row states.
func normalEqual(t *testing.T, e Event, c *captured, key string, cv, rv json.RawMessage) bool {
	t.Helper()
	switch key {
	case "transcript_path":
		var got string
		_ = json.Unmarshal(rv, &got)
		m := claudeLayout.FindStringSubmatch(c.str("transcript_path"))
		return got == placeholderPath && m != nil && m[1] == c.str("session_id")
	case "tool_input":
		// The same JSON value with its keys in another order: Claude Code
		// wrote the transcript's Edit input as { replace_all, file_path, ... }
		// and sent the hook { file_path, ..., replace_all }.
		var a, b any
		return json.Unmarshal(cv, &a) == nil && json.Unmarshal(rv, &b) == nil && reflect.DeepEqual(a, b)
	case "tool_response":
		return backgroundBits(cv) == backgroundBits(rv)
	}
	t.Fatalf("no normalisation for %q", key)
	return false
}

// TestREPLAY1b_FixturePositions places every fixture event at the line an
// independent read of the transcript finds it on: a PreToolUse at the line
// holding its tool_use, a Post at the line holding its tool_result, the
// prompt event at the one genuine prompt, and the Stop at the end of the last
// assistant line, which is the line the captured Stop's last_assistant_message
// is the text of.
func TestREPLAY1b_FixturePositions(t *testing.T) {
	for _, version := range []string{"2.1.280", "2.1.312"} {
		raw, err := os.ReadFile(fixtureDir + version + "/transcript.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		at := map[string]history.Pos{} // "use:id", "result:id"
		var promptLine, lastText int
		var finalText string
		for n, l := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
			var line struct {
				Type    string `json:"type"`
				Message struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(l), &line) != nil {
				continue
			}
			var s string
			if json.Unmarshal(line.Message.Content, &s) == nil && line.Type == "user" {
				promptLine = n + 1
				continue
			}
			var blocks []struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				ToolUseID string `json:"tool_use_id"`
				Text      string `json:"text"`
			}
			_ = json.Unmarshal(line.Message.Content, &blocks)
			for b, blk := range blocks {
				switch blk.Type {
				case "tool_use":
					at["use:"+blk.ID] = history.Pos{Line: n + 1, Block: b}
				case "tool_result":
					at["result:"+blk.ToolUseID] = history.Pos{Line: n + 1, Block: b}
				case "text":
					if line.Type == "assistant" {
						lastText, finalText = n+1, blk.Text
					}
				}
			}
		}
		caps := loadCaptured(t, version)
		var stop captured
		for _, c := range caps {
			if c.event == "Stop" {
				stop = c
			}
		}
		if stop.str("last_assistant_message") != finalText {
			t.Fatalf("%s: premise: captured Stop %q, last assistant text %q", version, stop.str("last_assistant_message"), finalText)
		}
		for _, e := range buildFixture(t, version).Events {
			var want history.Pos
			switch e.Kind {
			case UserPromptSubmit:
				want = history.Pos{Line: promptLine}
			case PreToolUse:
				want = at["use:"+e.ToolUseID]
			case PostToolUse, PostToolUseFailure:
				want = at["result:"+e.ToolUseID]
			case Stop:
				want = history.Pos{Line: lastText, Block: 1}
			}
			if e.Pos != want {
				t.Errorf("%s: %s %s at %+v, an independent read puts it at %+v", version, e.Kind, e.ToolUseID, e.Pos, want)
			}
		}
	}
}

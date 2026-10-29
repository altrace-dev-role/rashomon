package replay

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

func reasons(rs []Reason) string {
	var s []string
	for _, r := range rs {
		s = append(s, string(r))
	}
	return strings.Join(s, ",")
}

// find returns the first event of a kind for an id ("" for the prompt and
// the Stop).
func find(t *testing.T, tl Timeline, k Kind, id string) Event {
	t.Helper()
	for _, e := range tl.Events {
		if e.Kind == k && e.ToolUseID == id {
			return e
		}
	}
	t.Fatalf("no %s %q in %d events", k, id, len(tl.Events))
	return Event{}
}

// TestREPLAY4_UnsupportedNamesItsReason: whatever the recorder reads and the
// transcript cannot supply makes the event Unsupported, with the reason
// named, no payload, and the event a Gap. Nothing is filled with a guess.
func TestREPLAY4_UnsupportedNamesItsReason(t *testing.T) {
	type tc struct {
		name  string
		build func(d *doc)
		lim   history.Limits
		opt   func(*Options)
		kind  Kind
		id    string
		want  string // reasons, comma-joined; "" for supported
	}
	basic := func(d *doc) {
		d.promptLine("p1")
		d.uses("m1", bashUse("a", "make"))
		d.results("", okResult("a"))
		d.ends("m2", "done")
	}
	cases := []tc{
		{"the baseline is supported", basic, history.Limits{}, nil, PostToolUse, "a", ""},
		{"a subagent's call has an agent_type no line names", func(d *doc) {
			d.promptLine("p1")
			d.usesWith(map[string]any{"isSidechain": true, "agentId": "ag1"}, "m1", bashUse("a", "make"))
			d.results("", okResult("a"))
		}, history.Limits{}, nil, PreToolUse, "a", "sidechain_agent_type"},
		{"an interrupted call has a payload no fixture shows", func(d *doc) {
			d.promptLine("p1")
			d.uses("m1", bashUse("a", "sleep 99"))
			d.results("", toolResult{id: "a", isError: true, text: "Exit code 130", post: PostToolUseFailure, tur: map[string]any{"interrupted": true}})
		}, history.Limits{}, nil, PostToolUseFailure, "a", "interrupted_payload_unmeasured"},
		{"an input over the bound is not kept", basic, history.Limits{MaxOutstanding: 8, MaxQueuedBytes: 1 << 20, MaxInputBytes: 4, OutputHeadBytes: 64, OutputTailBytes: 64, MaxCallsPerFile: 8}, nil, PreToolUse, "a", "tool_input_dropped"},
		{"a result line that names no cwd", func(d *doc) {
			d.promptLine("p1")
			d.uses("m1", bashUse("a", "cd x"))
			d.results("-", okResult("a"))
		}, history.Limits{}, nil, PostToolUse, "a", "no_result_cwd"},
		{"a background launch whose result names no task", func(d *doc) {
			d.promptLine("p1")
			d.uses("m1", toolUse{"a", "Bash", map[string]any{"command": "make", "run_in_background": true}})
			d.results("", okResult("a"))
		}, history.Limits{}, nil, PostToolUse, "a", "background_unconfirmed"},
		{"a Bash result whose toolUseResult is not an object", func(d *doc) {
			d.promptLine("p1")
			d.uses("m1", bashUse("a", "make"))
			d.results("", toolResult{id: "a", text: "fine", post: PostToolUse})
		}, history.Limits{}, nil, PostToolUse, "a", "tool_response_unreadable"},
		{"a call before the file's first prompt", func(d *doc) {
			d.uses("m1", bashUse("a", "make"))
			d.results("", okResult("a"))
		}, history.Limits{}, nil, PreToolUse, "a", "no_prompt_id,no_permission_mode"},
		{"a prompt line naming no permission mode", func(d *doc) {
			d.add(map[string]any{"type": "user", "promptId": "p1", "message": map[string]any{"role": "user", "content": "go"}})
			d.prompt = "p1"
			d.uses("m1", bashUse("a", "make"))
			d.results("", okResult("a"))
		}, history.Limits{}, nil, PreToolUse, "a", "no_permission_mode"},
		{"the mode may have changed mid-turn", func(d *doc) {
			d.promptLine("p1")
			d.add(map[string]any{"type": "permission-mode", "permissionMode": "plan"})
			d.uses("m1", bashUse("a", "make"))
		}, history.Limits{}, nil, PreToolUse, "a", "permission_mode_changed_mid_turn"},
		{"no path for the file", basic, history.Limits{}, func(o *Options) { o.TranscriptPath = "" }, PreToolUse, "a", "no_transcript_path"},
		{"a prompt that is not a plain string", func(d *doc) {
			d.prompt = "p1"
			d.add(map[string]any{"type": "user", "promptId": "p1", "permissionMode": "default",
				"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hi"}, map[string]any{"type": "image"}}}})
		}, history.Limits{}, nil, UserPromptSubmit, "", "prompt_shape_unverified"},
		{"a final message of two text blocks", func(d *doc) {
			d.promptLine("p1")
			d.add(map[string]any{"type": "assistant", "message": map[string]any{"id": "m", "role": "assistant", "stop_reason": "end_turn",
				"content": []any{map[string]any{"type": "text", "text": "one"}, map[string]any{"type": "text", "text": "two"}}}})
		}, history.Limits{}, nil, Stop, "", "final_message_text_unmeasured"},
		{"a final message with no text", func(d *doc) {
			d.promptLine("p1")
			d.add(map[string]any{"type": "assistant", "message": map[string]any{"id": "m", "role": "assistant", "stop_reason": "end_turn",
				"content": []any{map[string]any{"type": "thinking", "thinking": ""}}}})
		}, history.Limits{}, nil, Stop, "", "no_final_text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDoc()
			c.build(d)
			opt := Options{TranscriptPath: synthPath, Limits: c.lim}
			if c.opt != nil {
				c.opt(&opt)
			}
			tl, err := Build(d.reader(), opt)
			if err != nil {
				t.Fatal(err)
			}
			e := find(t, tl, c.kind, c.id)
			if got := reasons(e.Unsupported); got != c.want {
				t.Fatalf("%s %q unsupported = %q, want %q", c.kind, c.id, got, c.want)
			}
			switch {
			case c.want == "" && (e.Payload == nil || e.Gap != ""):
				t.Errorf("a supported event has payload %q and gap %q", e.Payload, e.Gap)
			case c.want != "" && (e.Payload != nil || e.Gap != e.Unsupported[0] || e.Supported() || e.Taint.IsZero()):
				t.Errorf("an unsupported event has payload %q, gap %q, taint %+v", e.Payload, e.Gap, e.Taint)
			}
		})
	}

	// A background launch whose result does name the task is supported, and the
	// payload says so in the one key the recorder reads.
	d := newDoc()
	d.promptLine("p1")
	d.uses("m1", toolUse{"a", "Bash", map[string]any{"command": "make", "run_in_background": true}})
	d.results("", toolResult{id: "a", text: "started", post: PostToolUse, tur: map[string]any{"backgroundTaskId": "bg-1", "stdout": ""}})
	e := find(t, d.build(t, history.DefaultLimits), PostToolUse, "a")
	if !e.Supported() || !strings.Contains(string(e.Payload), `"tool_response":{"backgroundTaskId":true}`) {
		t.Errorf("background launch: %v %s", e.Unsupported, e.Payload)
	}

	// A version with no fixture makes every event of the file unsupported.
	d = newDoc()
	basic(d)
	text := strings.ReplaceAll(d.String(), `"version":"2.1.312"`, `"version":"9.9.9"`)
	tl, err := Build(strings.NewReader(text), Options{TranscriptPath: synthPath})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tl.Events {
		if e.Supported() || e.Unsupported[0] != UnsupportedVersion {
			t.Errorf("%s %s with a version no fixture shows: %v", e.Kind, e.ToolUseID, e.Unsupported)
		}
	}

	// The Unrecoverable rows of the field table name reasons that occur.
	for _, k := range []Kind{PreToolUse, PostToolUse, PostToolUseFailure} {
		for _, f := range Fields(k) {
			if f.Class == Unrecoverable && f.Reason != SidechainAgentType {
				t.Errorf("%s/%s is Unrecoverable with reason %q, which no test above produces", k, f.Key, f.Reason)
			}
		}
	}
}

// TestREPLAY4_TaintPropagates: an event that cannot be fed, a Stop that
// cannot be placed and a line that cannot be read each leave evidence
// incomplete from their position on. Every later event says so (Taint), no
// earlier one does, and the timeline as a whole says where it began. A
// denial, and a call with no result at the end of the file, are exactly what
// a live run recorded and are not gaps.
func TestREPLAY4_TaintPropagates(t *testing.T) {
	t.Run("an unsupported event", func(t *testing.T) {
		d := newDoc()
		d.promptLine("p1")
		d.uses("m1", bashUse("ok1", "make"))
		d.results("", okResult("ok1"))
		sub := d.usesWith(map[string]any{"isSidechain": true, "agentId": "ag"}, "m2", bashUse("sub", "go test"))
		d.results("", okResult("sub"))
		d.uses("m3", bashUse("late", "make"))
		d.results("", okResult("late"))
		d.ends("m4", "done")
		tl := d.build(t, history.DefaultLimits)
		at := history.Pos{Line: sub}
		seenGap := 0
		for _, e := range tl.Events {
			switch {
			case e.Pos.Before(at):
				if !e.Taint.IsZero() || e.Gap != "" {
					t.Errorf("%s %s before the gap carries taint %+v gap %q", e.Kind, e.ToolUseID, e.Taint, e.Gap)
				}
			default:
				if e.Taint != at {
					t.Errorf("%s %s at %+v after the gap carries taint %+v, want %+v", e.Kind, e.ToolUseID, e.Pos, e.Taint, at)
				}
			}
			if e.Gap != "" {
				seenGap++
			}
		}
		if tl.TaintFrom != at || tl.Gaps != 2 || seenGap != 2 {
			t.Errorf("timeline: taint %+v (want %+v), gaps %d / %d", tl.TaintFrom, at, tl.Gaps, seenGap)
		}
		if st := stopsOf(tl); len(st) != 1 || st[0].Taint != at || !st[0].Supported() {
			t.Errorf("the Stop is supported and tainted: %+v", st)
		}
	})
	t.Run("an unknown Stop taints the next turn", func(t *testing.T) {
		d := newDoc()
		d.promptLine("p1")
		d.uses("m1", bashUse("a", "make"))
		d.results("", okResult("a")) // the turn ends here, nothing closes it
		p2 := d.promptLine("p2")
		d.uses("m2", bashUse("b", "make"))
		d.results("", okResult("b"))
		d.ends("m3", "done")
		tl := d.build(t, history.DefaultLimits)
		unknown := stopsOf(tl)[0]
		if !unknown.BoundaryUnknown || unknown.Taint != unknown.Pos {
			t.Fatalf("first Stop: %+v", unknown)
		}
		for _, e := range tl.Events {
			after := !e.Pos.Before(unknown.Pos)
			if after && e.Taint != unknown.Pos || !after && !e.Taint.IsZero() {
				t.Errorf("%s %s at %+v: taint %+v, unknown Stop at %+v", e.Kind, e.ToolUseID, e.Pos, e.Taint, unknown.Pos)
			}
		}
		if e := find(t, tl, UserPromptSubmit, ""); e.Pos.Line == p2 && false {
			t.Fatal("unreachable")
		}
		if last := stopsOf(tl)[1]; last.Taint != unknown.Pos || !last.Supported() {
			t.Errorf("the second turn's Stop: %+v", last)
		}
	})
	t.Run("an unreadable line", func(t *testing.T) {
		d := newDoc()
		d.promptLine("p1")
		d.uses("m1", bashUse("a", "make"))
		d.results("", okResult("a"))
		bad := d.raw("{this is not json")
		d.uses("m2", bashUse("b", "make"))
		d.results("", okResult("b"))
		d.ends("m3", "done")
		tl := d.build(t, history.DefaultLimits)
		for _, e := range tl.Events {
			after := e.Pos.Line > bad
			if after && e.Taint != (history.Pos{Line: bad}) || !after && !e.Taint.IsZero() {
				t.Errorf("%s %s at %+v: taint %+v, bad line %d", e.Kind, e.ToolUseID, e.Pos, e.Taint, bad)
			}
		}
		if tl.TaintFrom != (history.Pos{Line: bad}) || tl.Unreadable != 1 || tl.Gaps != 0 {
			t.Errorf("timeline taint %+v unreadable %d gaps %d", tl.TaintFrom, tl.Unreadable, tl.Gaps)
		}
	})
	t.Run("an unreadable line after the last event", func(t *testing.T) {
		d := newDoc()
		d.promptLine("p1")
		d.ends("m1", "done")
		// A subagent's line that cannot be read: it never moves a turn's end,
		// and history counts it unread all the same.
		bad := d.raw(`{"isSidechain":true,"message":{"content":`)
		tl := d.build(t, history.DefaultLimits)
		if st := stopsOf(tl); len(st) != 1 || st[0].BoundaryUnknown || !st[0].Taint.IsZero() {
			t.Errorf("the Stop is before the bad line: %+v", st)
		}
		if tl.TaintFrom != (history.Pos{Line: bad}) {
			t.Errorf("TaintFrom = %+v", tl.TaintFrom)
		}
	})
	t.Run("a denial and an unanswered last call are not gaps", func(t *testing.T) {
		d := newDoc()
		d.promptLine("p1")
		d.uses("m1", bashUse("d", "rm x"), bashUse("a", "make"))
		d.results("", denyResult("d"))
		d.results("", okResult("a"))
		d.uses("m2", bashUse("m", "sleep 1"))
		tl := d.build(t, history.DefaultLimits)
		for _, e := range tl.Events {
			if e.Kind != Stop && (e.Gap != "" || !e.Taint.IsZero()) {
				t.Errorf("%s %s: gap %q taint %+v", e.Kind, e.ToolUseID, e.Gap, e.Taint)
			}
		}
	})
	t.Run("a result history dropped", func(t *testing.T) {
		d := newDoc()
		d.promptLine("p1")
		d.uses("m1", bashUse("a", "make"), bashUse("b", "make"), bashUse("c", "make"))
		d.results("", okResult("c"))
		d.results("", okResult("b"))
		d.results("", okResult("a"))
		d.ends("m2", "done")
		lim := history.DefaultLimits
		lim.MaxOutstanding = 2
		tl := d.build(t, lim)
		a := find(t, tl, PreToolUse, "a")
		if !a.Missing || a.Gap != Evicted || a.Taint != a.Pos {
			t.Fatalf("a: missing %v gap %q taint %+v", a.Missing, a.Gap, a.Taint)
		}
		if e := find(t, tl, PostToolUse, "c"); e.Taint != a.Pos {
			t.Errorf("c's execution carries taint %+v, want %+v", e.Taint, a.Pos)
		}
	})
}

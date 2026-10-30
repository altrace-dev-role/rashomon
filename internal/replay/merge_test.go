package replay

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

// The merge of a session's files (merge.go): causal order across files, copies,
// disagreeing copies, the subagent link, and the places where the order of two
// files cannot be established, which are gaps with a name.

type strSrc struct{ *strings.Reader }

func (strSrc) Close() error { return nil }

// files makes sources from documents, the first being the session's own and
// the rest, by main, its subagents'.
func files(main int, docs ...string) []Source {
	var out []Source
	for i, d := range docs {
		d := d
		out = append(out, Source{Name: fmt.Sprintf("f%d", i), Main: i < main,
			Open: func() (io.ReadSeekCloser, error) { return strSrc{strings.NewReader(d)}, nil }})
	}
	return out
}

func mopt() MergeOptions { return MergeOptions{Options: Options{TranscriptPath: synthPath}} }

func session(t *testing.T, main int, docs ...string) SessionTimeline {
	t.Helper()
	tl, err := BuildSession(synthSession, files(main, docs...), mopt())
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

func kinds(tl SessionTimeline) string {
	var s []string
	for _, e := range tl.Events {
		id := e.ToolUseID
		if id == "" {
			id = "-"
		}
		short := map[Kind]string{UserPromptSubmit: "Prompt", PreToolUse: "Pre", PostToolUse: "Post", PostToolUseFailure: "PostFailure", Stop: "Stop"}[e.Kind]
		s = append(s, short+":"+id)
	}
	return strings.Join(s, " ")
}

func tsAt(base int, plus time.Duration) string {
	return synthTime(base).Add(plus).Format("2006-01-02T15:04:05.000Z")
}

// agentCallDoc is a parent transcript whose second line dispatches an Agent
// call (subagent_type "Explore") answered, on its third, by a result naming
// agent "A1". agentID is what that result names; "" names none.
func agentCallDoc(agentID, subType string) *doc {
	d := newDoc()
	d.promptLine("p1")
	d.uses("m1", toolUse{"ag1", "Agent", map[string]any{"description": "look", "subagent_type": subType, "prompt": "x"}})
	tur := map[string]any{"status": "completed"}
	if agentID != "" {
		tur["agentId"], tur["agentType"] = agentID, subType
	}
	d.results("", toolResult{id: "ag1", text: "done", post: PostToolUse, tur: tur})
	d.uses("m2", bashUse("b1", "make"))
	d.results("", okResult("b1"))
	d.ends("m3", "done")
	return d
}

// childDoc is a subagent's file: one Bash call between the given offsets after
// the parent's second line (seconds are the parent's line numbers).
func childDoc(agent, attr string, use, result time.Duration) *doc {
	d := newDoc()
	d.prompt = "p1"
	side := map[string]any{"isSidechain": true, "agentId": agent}
	call := map[string]any{"timestamp": tsAt(2, use)}
	for k, v := range side {
		call[k] = v
	}
	if attr != "" {
		call["attributionAgent"] = attr
	}
	d.usesWith(call, "cm1", bashUse("c1", "make test"))
	res := map[string]any{"type": "user", "promptId": "p1", "timestamp": tsAt(2, result),
		"message":       map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "c1", "is_error": false, "content": "fine"}}},
		"toolUseResult": map[string]any{"stdout": "fine"}}
	for k, v := range side {
		res[k] = v
	}
	d.add(res)
	return d
}

// TestREPLAY18_ASubagentFileSitsBetweenItsParentsCallAndItsResult merges a
// parent with its subagent's file: the subagent's declaration and execution
// come after the parent's Agent declaration and before the Agent call's own
// execution, though they are in another file, and carry the agent id and the
// type the parent's call gave.
func TestREPLAY18_ASubagentFileSitsBetweenItsParentsCallAndItsResult(t *testing.T) {
	parent, child := agentCallDoc("A1", "Explore"), childDoc("A1", "Explore", 300*time.Millisecond, 600*time.Millisecond)
	tl := session(t, 1, parent.String(), child.String())
	want := "Prompt:- Pre:ag1 Pre:c1 Post:c1 Post:ag1 Pre:b1 Post:b1 Stop:-"
	if got := kinds(tl); got != want {
		t.Fatalf("merged order:\n got %s\nwant %s", got, want)
	}
	if !tl.TaintFrom.IsZero() || tl.Gaps != 0 || tl.OrderGaps != 0 {
		t.Errorf("a causally ordered merge is tainted from %+v: gaps %d, order gaps %d (%s)", tl.TaintFrom, tl.Gaps, tl.OrderGaps, tl.TaintReason)
	}
	for _, e := range tl.Events {
		sub := e.ToolUseID == "c1"
		if sub != (e.File == 1) {
			t.Errorf("%s %s is in file %d", e.Kind, e.ToolUseID, e.File)
		}
		if sub {
			if !e.Supported() || e.AgentID != "A1" || e.AgentType != "Explore" {
				t.Errorf("%s c1: unsupported %v, agent %q type %q", e.Kind, e.Unsupported, e.AgentID, e.AgentType)
			}
			for _, w := range []string{`"agent_id":"A1"`, `"agent_type":"Explore"`, `"prompt_id":"p1"`, `"permission_mode":"default"`} {
				if !strings.Contains(string(e.Payload), w) {
					t.Errorf("%s c1 payload lacks %s: %s", e.Kind, w, e.Payload)
				}
			}
		} else if strings.Contains(string(e.Payload), "agent_id") {
			t.Errorf("%s %s of the main agent carries an agent id: %s", e.Kind, e.ToolUseID, e.Payload)
		}
	}
	if len(tl.Agents) != 1 || tl.Agents[0].Status != LinkOK || tl.Agents[0].Type != "Explore" {
		t.Errorf("links: %+v", tl.Agents)
	}
	// A tie at the very instant of the Agent call's execution still puts the
	// subagent's execution first: the call returned it.
	child = childDoc("A1", "Explore", 300*time.Millisecond, time.Second)
	if got := kinds(session(t, 1, parent.String(), child.String())); got != want {
		t.Errorf("a subagent result at the instant of its parent's result: %s", got)
	}
}

// TestREPLAY18b_ASubagentLinkIsTheParentCallAlone: every way the one link can
// fail keeps the subagent's calls Unsupported and taints from the first of
// them, and none of them guesses a type from anywhere else: not the file's
// name, not the line's attributionAgent, not text.
func TestREPLAY18b_ASubagentLinkIsTheParentCallAlone(t *testing.T) {
	good := func() *doc { return childDoc("A1", "Explore", 300*time.Millisecond, 600*time.Millisecond) }
	two := func() string {
		d := agentCallDoc("A1", "Explore")
		// A second Agent call, answered by a line naming the same agent.
		d.uses("m4", toolUse{"ag2", "Agent", map[string]any{"subagent_type": "Plan"}})
		d.results("", toolResult{id: "ag2", text: "done", post: PostToolUse, tur: map[string]any{"agentId": "A1"}})
		return d.String()
	}
	unreadable := func() string {
		d := agentCallDoc("A1", "Explore")
		d.lines[2] = d.lines[2][:len(d.lines[2])/2] // the result line is cut
		return d.String()
	}
	otherTool := func() string {
		// A Bash call's result names the agent: it is no Agent call.
		d := agentCallDoc("", "Explore")
		d.uses("m4", bashUse("b9", "make"))
		d.results("", toolResult{id: "b9", text: "fine", post: PostToolUse, tur: map[string]any{"agentId": "A1"}})
		return d.String()
	}
	cases := []struct {
		name   string
		parent string
		child  *doc
		status LinkStatus
	}{
		{"a result of another tool names the agent", otherTool(), good(), LinkNoParent},
		{"no parent names the agent", agentCallDoc("", "Explore").String(), good(), LinkNoParent},
		{"another agent is named", agentCallDoc("A9", "Explore").String(), good(), LinkNoParent},
		{"two calls claim it", two(), good(), LinkAmbiguous},
		{"the parent's line is unreadable", unreadable(), good(), LinkNoParent},
		{"the parent names no type", agentCallDoc("A1", "").String(), good(), LinkNoType},
		{"a line contradicts the type", agentCallDoc("A1", "Explore").String(), childDoc("A1", "Plan", 300*time.Millisecond, 600*time.Millisecond), LinkAttributionMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tl := session(t, 1, c.parent, c.child.String())
			if len(tl.Agents) != 1 || tl.Agents[0].Status != c.status || tl.Agents[0].Type != "" {
				t.Fatalf("links %+v, want status %s with no type", tl.Agents, c.status)
			}
			var gap *Event
			for i := range tl.Events {
				e := &tl.Events[i]
				if e.ToolUseID != "c1" {
					continue
				}
				if e.Supported() || e.Unsupported[0] != SidechainAgentType || e.AgentID != "" || e.Payload != nil {
					t.Errorf("%s c1 is %v with agent %q: want Unsupported(sidechain_agent_type)", e.Kind, e.Unsupported, e.AgentID)
				}
				if gap == nil {
					gap = e
				}
			}
			if gap == nil {
				t.Fatal("no subagent event at all")
			}
			if tl.TaintFrom.IsZero() {
				t.Fatalf("an unsupported subagent call taints nothing")
			}
			for _, e := range tl.Events {
				after := gap.Pos.Before(e.Pos) && e.File == gap.File || e.Kind == Stop
				if after && e.Kind != PreToolUse || e.ToolUseID == "c1" {
					if e.Taint.IsZero() && e.ToolUseID == "c1" || e.Kind == Stop && e.Taint.IsZero() {
						t.Errorf("%s %s is not tainted", e.Kind, e.ToolUseID)
					}
				}
			}
		})
	}
}

// TestREPLAY18c_TheParentCopiesMustAgreeOnTheType: two copies of the parent
// that name different types for the one Agent call give the subagent no type.
func TestREPLAY18c_TheParentCopiesMustAgreeOnTheType(t *testing.T) {
	a, b := agentCallDoc("A1", "Explore"), agentCallDoc("A1", "Plan")
	child := childDoc("A1", "", 300*time.Millisecond, 600*time.Millisecond)
	tl := session(t, 2, a.String(), b.String(), child.String())
	if len(tl.Agents) != 1 || tl.Agents[0].Status != LinkParentConflict || tl.Agents[0].Type != "" {
		t.Fatalf("links %+v", tl.Agents)
	}
	for _, e := range tl.Events {
		if e.ToolUseID == "c1" && (e.Supported() || e.AgentType != "") {
			t.Errorf("%s c1 is fed with type %q", e.Kind, e.AgentType)
		}
	}
	// The one call, named by its copies as the parent of two different agents,
	// is the parent of neither.
	c := agentCallDoc("A2", "Explore")
	both := session(t, 2, a.String(), c.String(), child.String())
	if len(both.Agents) != 1 || both.Agents[0].Status != LinkAmbiguous {
		t.Errorf("a call that claims two agents: %+v", both.Agents)
	}
	// The same two copies naming one type give the link.
	same := session(t, 2, a.String(), a.String()+`{"type":"last-prompt","sessionId":"s1"}`+"\n", child.String())
	if len(same.Agents) != 1 || same.Agents[0].Status != LinkOK {
		t.Errorf("copies of one parent: %+v", same.Agents)
	}
}

// TestREPLAY18d_ASubagentCallIsFedOnlyOnAnAssumptionItNames: a subagent's result
// lines carry no toolUseResult, so a foreground Bash call of its is fed as not
// backgrounded and the event says it assumed so; one that asked for the
// background is not fed at all.
func TestREPLAY18d_ASubagentCallIsFedOnlyOnAnAssumptionItNames(t *testing.T) {
	parent := agentCallDoc("A1", "Explore").String()
	mk := func(input map[string]any) string {
		d := newDoc()
		d.prompt = "p1"
		d.usesWith(map[string]any{"isSidechain": true, "agentId": "A1", "attributionAgent": "Explore", "timestamp": tsAt(2, 300*time.Millisecond)},
			"cm1", toolUse{"c1", "Bash", input})
		d.add(map[string]any{"type": "user", "promptId": "p1", "isSidechain": true, "agentId": "A1", "timestamp": tsAt(2, 600*time.Millisecond),
			"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "c1", "is_error": false, "content": "fine"}}}})
		return d.String()
	}
	tl := session(t, 1, parent, mk(map[string]any{"command": "make"}))
	var post *Event
	for i := range tl.Events {
		if tl.Events[i].Kind == PostToolUse && tl.Events[i].ToolUseID == "c1" {
			post = &tl.Events[i]
		}
	}
	if post == nil || !post.Supported() || len(post.Assumed) != 1 || post.Assumed[0] != SidechainResponseAbsent {
		t.Fatalf("a foreground subagent call: %+v", post)
	}
	for _, e := range tl.Events {
		if e.ToolUseID != "c1" && len(e.Assumed) != 0 {
			t.Errorf("%s %s assumed %v", e.Kind, e.ToolUseID, e.Assumed)
		}
	}
	tl = session(t, 1, parent, mk(map[string]any{"command": "make", "run_in_background": true}))
	for _, e := range tl.Events {
		if e.Kind == PostToolUse && e.ToolUseID == "c1" && (e.Supported() || e.Gap != ResponseUnreadable) {
			t.Errorf("a background launch of a subagent: unsupported %v gap %q", e.Unsupported, e.Gap)
		}
	}
}

// TestREPLAY19b_AnotherSessionsLinesInAFileAreNotThisSessions: a resumed file
// that carries an earlier session's lines is that session's evidence as well,
// and each session's timeline holds its own events only.
func TestREPLAY19b_AnotherSessionsLinesInAFileAreNotThisSessions(t *testing.T) {
	a, b := newDoc(), newDoc()
	turnOne(a)
	turnOne(b)
	earlier := strings.ReplaceAll(strings.ReplaceAll(b.String(), `"sessionId":"s1"`, `"sessionId":"s2"`), `"t1"`, `"u1"`)
	mixed := earlier + a.String()
	tl := session(t, 1, mixed)
	if got, want := kinds(tl), "Prompt:- Pre:t1 Post:t1 Stop:-"; got != want || tl.OtherSession != 4 {
		t.Errorf("session s1 of a file that names two: %s (%d from another session)", got, tl.OtherSession)
	}
	m, err := Inspect(strings.NewReader(mixed))
	if err != nil || len(m.Sessions) != 2 || !m.Sessions[0].Main || !m.Sessions[1].Main {
		t.Errorf("inspect: %+v %v", m.Sessions, err)
	}
}

// resumedDoc is a transcript of two turns whose first n lines are turn one:
// the same lines, at the same times, as another file's.
func turnOne(d *doc) {
	d.promptLine("p1")
	d.uses("m1", bashUse("t1", "make"))
	d.results("", okResult("t1"))
	d.ends("m2", "one")
}

func turnTwo(d *doc) {
	d.promptLine("p2")
	d.uses("m3", bashUse("t2", "make test"))
	d.results("", failResult("t2"))
	d.ends("m4", "two")
}

// TestREPLAY19_AResumedFileRepeatsTurnsAndAddsAfterThem: a resumed file that
// repeats turn one and goes on to turn two merges with the first file into one
// timeline in which each event of turn one is there once and turn two follows
// it.
func TestREPLAY19_AResumedFileRepeatsTurnsAndAddsAfterThem(t *testing.T) {
	a, b := newDoc(), newDoc()
	turnOne(a)
	turnOne(b)
	turnTwo(b)
	tl := session(t, 2, a.String(), b.String())
	want := "Prompt:- Pre:t1 Post:t1 Stop:- Prompt:- Pre:t2 PostFailure:t2 Stop:-"
	if got := kinds(tl); got != want {
		t.Fatalf("merged:\n got %s\nwant %s", got, want)
	}
	if tl.Copies != 4 {
		t.Errorf("%d events dropped as copies, want the 4 of turn one", tl.Copies)
	}
	if !tl.TaintFrom.IsZero() || !tl.Complete {
		t.Errorf("taint %+v (%s), complete %v", tl.TaintFrom, tl.TaintReason, tl.Complete)
	}
	// The listing order of the files changes nothing, and neither does the
	// order the rule gives them: the same events, once each.
	rev := session(t, 2, b.String(), a.String())
	if kinds(rev) != kinds(tl) {
		t.Errorf("files given in the other order: %s", kinds(rev))
	}
}

// TestREPLAY20_DisagreeingCopiesAreNotTrusted: two copies of one call whose
// results (or inputs) differ are both Unsupported, and the timeline is tainted
// from the call on; a copy that is only shorter is not a disagreement.
func TestREPLAY20_DisagreeingCopiesAreNotTrusted(t *testing.T) {
	mk := func(res func(string) toolResult, cmd string) string {
		d := newDoc()
		d.promptLine("p1")
		d.uses("m1", bashUse("k1", cmd))
		d.results("", res("k1"))
		d.uses("m2", bashUse("k2", "make"))
		d.results("", okResult("k2"))
		d.ends("m3", "done")
		return d.String()
	}
	check := func(t *testing.T, tl SessionTimeline, conflict bool) {
		t.Helper()
		for _, e := range tl.Events {
			bad := conflict && (e.ToolUseID == "k1")
			if bad && (e.Supported() || e.Gap != CopiesDisagree) {
				t.Errorf("%s k1: unsupported %v gap %q, want copies_disagree", e.Kind, e.Unsupported, e.Gap)
			}
			if !bad && e.ToolUseID == "k1" && !e.Supported() {
				t.Errorf("%s k1 unsupported though the copies agree: %v", e.Kind, e.Unsupported)
			}
		}
		if conflict != (tl.Conflicts == 1) || conflict && tl.TaintFrom.IsZero() {
			t.Errorf("conflicts %d, taint %+v (%s)", tl.Conflicts, tl.TaintFrom, tl.TaintReason)
		}
		if conflict {
			if tl.TaintFrom.Pos.Line != 2 || tl.TaintReason != string(CopiesDisagree) {
				t.Errorf("taint from %+v for %q, want the call's use at line 2", tl.TaintFrom, tl.TaintReason)
			}
			for _, e := range tl.Events {
				if e.Kind != UserPromptSubmit && e.Taint.IsZero() {
					t.Errorf("%s %s after the conflict is not tainted", e.Kind, e.ToolUseID)
				}
			}
		}
	}
	t.Run("results differ", func(t *testing.T) {
		check(t, session(t, 2, mk(okResult, "make"), mk(failResult, "make")), true)
	})
	t.Run("inputs differ", func(t *testing.T) {
		check(t, session(t, 2, mk(okResult, "make"), mk(okResult, "make -j")), true)
	})
	t.Run("copies agree", func(t *testing.T) {
		a := mk(okResult, "make")
		check(t, session(t, 2, a, a+`{"type":"last-prompt","sessionId":"s1"}`+"\n"), false)
	})
	t.Run("a copy cut before the result", func(t *testing.T) {
		d := newDoc()
		d.promptLine("p1")
		d.uses("m1", bashUse("k1", "make"))
		short := d.String()
		tl := session(t, 2, short, mk(okResult, "make"))
		check(t, tl, false)
		// The short copy ends mid-turn, so it has a Stop of unknown boundary:
		// that is a gap, named as the Stop's, and not a disagreement.
		if tl.TaintReason != string(StopNotEndTurn) {
			t.Errorf("taint reason %q, want the cut copy's unknown Stop", tl.TaintReason)
		}
		var post int
		for _, e := range tl.Events {
			if e.Kind == PostToolUse && e.ToolUseID == "k1" {
				post++
			}
		}
		if post != 1 || tl.Missing != 0 {
			t.Errorf("the complete copy's result is %d times in the timeline, %d calls missing", post, tl.Missing)
		}
	})
}

// TestREPLAY21_AnOrderNobodyCanEstablishTaints: where the order of two files
// is a coin toss, or contradicts the calls, the merge names it and taints from
// there; it never picks one silently.
func TestREPLAY21_AnOrderNobodyCanEstablishTaints(t *testing.T) {
	t.Run("two files, one instant", func(t *testing.T) {
		// A write in one file and a gate run in the other, at the very same
		// instant: the verdict at a later Stop depends on which came first.
		a, b := newDoc(), newDoc()
		a.promptLine("p1")
		a.uses("m1", writeUse("w1", "/repo/x.go"))
		a.results("", okResult("w1"))
		a.ends("m9", "done")
		b.promptLine("p1")
		b.uses("m2", bashUse("g1", "go test ./..."))
		b.results("", okResult("g1"))
		tl := session(t, 2, a.String(), b.String())
		if tl.TaintFrom.IsZero() || tl.TaintReason != string(OrderAmbiguous) || tl.OrderGaps == 0 {
			t.Fatalf("taint %+v (%s), order gaps %d", tl.TaintFrom, tl.TaintReason, tl.OrderGaps)
		}
		last := tl.Events[len(tl.Events)-1]
		if last.Kind != Stop || last.Taint.IsZero() {
			t.Errorf("the Stop after the tie is %s with taint %+v", last.Kind, last.Taint)
		}
	})
	t.Run("a result never merges before its use", func(t *testing.T) {
		// The second file's copy of the call is stamped before the first's, and
		// its result sits between the two stamps. Whichever copy the merge
		// keeps, the call's use is yielded before its result.
		use := func(at int) map[string]any {
			return map[string]any{"type": "assistant", "timestamp": tsAt(at, 0), "message": map[string]any{"id": "m1", "role": "assistant", "stop_reason": "tool_use",
				"content": []any{map[string]any{"type": "tool_use", "id": "r1", "name": "Bash", "input": map[string]any{"command": "make"}}}}}
		}
		a, b := newDoc(), newDoc()
		a.promptLine("p1")
		a.add(use(50))
		b.promptLine("p1")
		b.add(use(40))
		b.add(map[string]any{"type": "user", "promptId": "p1", "timestamp": tsAt(45, 0), "toolUseResult": map[string]any{"stdout": "fine"},
			"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "r1", "is_error": false, "content": "fine"}}}})
		for _, order := range [][]string{{a.String(), b.String()}, {b.String(), a.String()}} {
			tl := session(t, 2, order...)
			pre, post := -1, -1
			for i, e := range tl.Events {
				switch {
				case e.Kind == PreToolUse && e.ToolUseID == "r1":
					pre = i
				case e.Kind == PostToolUse && e.ToolUseID == "r1":
					post = i
				}
			}
			if pre < 0 || post < 0 || pre > post {
				t.Errorf("use at %d, result at %d in %s", pre, post, kinds(tl))
			}
		}
	})
	t.Run("a result merged first is refused", func(t *testing.T) {
		// The guard behind the property above: should a result ever reach the
		// front before its call's use, it is not fed, and it is a gap.
		m := &merger{ix: &index{ids: map[digest16]*callState{hash16([]byte("r1")): {}}}, turns: map[turnKey]bool{}}
		e := Event{Kind: PostToolUse, ToolUseID: "r1", SessionID: "s1", Payload: []byte("{}")}
		if m.call(&e); e.Supported() || e.Gap != ResultBeforeUse || e.Payload != nil {
			t.Errorf("a result before its use: unsupported %v gap %q", e.Unsupported, e.Gap)
		}
	})
	t.Run("a file that ends in a cut line", func(t *testing.T) {
		// A copy of turn one that ends in a line cut short (one that names
		// itself a sidechain's, so it moves no turn's end) and a later file with
		// turn two. What the cut line held is not known, so everything merged
		// after the copy's last event is tainted, and nothing before it is.
		a, c := newDoc(), newDoc()
		turnOne(a)
		c.promptLine("p2")
		c.uses("m3", bashUse("t2", "make"))
		c.results("", okResult("t2"))
		c.ends("m4", "two")
		cut := a.String() + `{"isSidechain":true,"type":"user","message":{"cont` + "\n"
		late := strings.NewReplacer(`"timestamp":"2026-10-29T10:00:`, `"timestamp":"2026-10-29T11:00:`).Replace(c.String())
		tl := session(t, 3, a.String(), cut, late)
		if tl.TaintFrom.IsZero() || tl.TaintReason != "unreadable_line" || tl.TaintFrom.File != 1 || tl.TaintFrom.Pos.Line != 5 {
			t.Fatalf("taint %+v (%s)", tl.TaintFrom, tl.TaintReason)
		}
		for _, e := range tl.Events {
			second := e.File == 2
			if second != !e.Taint.IsZero() {
				t.Errorf("%s %s of file %d has taint %+v", e.Kind, e.ToolUseID, e.File, e.Taint)
			}
		}
		if n := len(tl.Events); n != 8 {
			t.Errorf("%d events: %s", n, kinds(tl))
		}
	})
	t.Run("a file whose clock goes backward", func(t *testing.T) {
		a, b := newDoc(), newDoc()
		a.promptLine("p1")
		a.uses("m1", bashUse("a1", "make"))
		a.results("", okResult("a1"))
		b.promptLine("p1")
		b.add(map[string]any{"type": "assistant", "timestamp": tsAt(30, 0), "message": map[string]any{"id": "m5", "role": "assistant", "stop_reason": "tool_use",
			"content": []any{map[string]any{"type": "tool_use", "id": "b1", "name": "Bash", "input": map[string]any{"command": "make"}}}}})
		b.add(map[string]any{"type": "user", "promptId": "p1", "timestamp": tsAt(20, 0), "toolUseResult": map[string]any{"stdout": "fine"},
			"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "b1", "is_error": false, "content": "fine"}}}})
		tl := session(t, 2, a.String(), b.String())
		if tl.TaintFrom.IsZero() || tl.OrderGaps == 0 {
			t.Errorf("a regression in a session of two files taints %+v (%s)", tl.TaintFrom, tl.TaintReason)
		}
		// By itself the same file is a plain transcript: file order is its order.
		if one := session(t, 1, b.String()); one.OrderGaps != 0 {
			t.Errorf("a regression within one file is an order gap: %d", one.OrderGaps)
		}
	})
	t.Run("a subagent outside its parent's call", func(t *testing.T) {
		parent := agentCallDoc("A1", "Explore").String()
		after := childDoc("A1", "Explore", 2*time.Second+300*time.Millisecond, 2*time.Second+600*time.Millisecond).String() // 4.3s: after the result at 3s
		tl := session(t, 1, parent, after)
		if tl.TaintFrom.IsZero() || tl.TaintReason != string(SubagentOutsideCall) {
			t.Errorf("a subagent after its parent's result: taint %+v (%s)", tl.TaintFrom, tl.TaintReason)
		}
		before := childDoc("A1", "Explore", -500*time.Millisecond, 600*time.Millisecond).String()
		tl = session(t, 1, parent, before)
		if tl.TaintFrom.IsZero() || tl.TaintReason != string(SubagentOutsideCall) {
			t.Errorf("a subagent before its parent's call: taint %+v (%s)", tl.TaintFrom, tl.TaintReason)
		}
		// Its clock puts it before the call; the call that started it comes first.
		k := kinds(tl)
		if strings.Index(k, "Pre:c1") < strings.Index(k, "Pre:ag1") {
			t.Errorf("a subagent's declaration placed before the parent's Agent call: %s", k)
		}
	})
}

// TestREPLAY22_TheCallIDCapGradesTheSessionPartial: past the cap on the digests
// held, a call cannot be recognised if a later file repeats it, so it is not
// fed and the summary says so.
func TestREPLAY22_TheCallIDCapGradesTheSessionPartial(t *testing.T) {
	d := newDoc()
	d.promptLine("p1")
	for i := range 4 {
		d.uses(fmt.Sprintf("m%d", i), bashUse(fmt.Sprintf("c%d", i), "make"))
		d.results("", okResult(fmt.Sprintf("c%d", i)))
	}
	d.ends("mz", "done")
	o := mopt()
	o.MaxCallIDs = 2
	tl, err := BuildSession(synthSession, files(1, d.String()), o)
	if err != nil {
		t.Fatal(err)
	}
	if !tl.CapHit || tl.Untracked != 2 || tl.CallIDs != 2 {
		t.Fatalf("cap hit %v, untracked %d, tracked %d", tl.CapHit, tl.Untracked, tl.CallIDs)
	}
	for _, e := range tl.Events {
		over := e.ToolUseID == "c2" || e.ToolUseID == "c3"
		if over != (e.Gap == SessionIDCap) || over && e.Supported() {
			t.Errorf("%s %s: gap %q", e.Kind, e.ToolUseID, e.Gap)
		}
	}
	if tl.TaintFrom.Pos.Line != 6 || tl.TaintReason != string(SessionIDCap) {
		t.Errorf("taint %+v (%s), want from the first call past the cap", tl.TaintFrom, tl.TaintReason)
	}
}

// TestREPLAY23_TheFileOrderDependsOnBytesOnly: the rule of OrderFiles gives the
// same order whatever the files are called or listed in, puts a session's own
// files before its subagents', and drops a byte-identical copy.
func TestREPLAY23_TheFileOrderDependsOnBytesOnly(t *testing.T) {
	a, b := newDoc(), newDoc()
	turnOne(a)
	turnOne(b)
	turnTwo(b)
	// The subagent's file begins before either, and b's has a line before a's.
	child := `{"type":"attachment","isSidechain":true,"agentId":"A1","sessionId":"s1","timestamp":"2026-10-29T09:00:00.000Z"}` + "\n" +
		childDoc("A1", "Explore", 300*time.Millisecond, 600*time.Millisecond).String()
	early := `{"type":"queue-operation","sessionId":"s1","timestamp":"2026-10-29T09:30:00.000Z"}` + "\n"
	texts := []string{early + b.String(), child, a.String(), a.String()}
	var metas []FileMeta
	for _, s := range texts {
		m, err := Inspect(strings.NewReader(s))
		if err != nil {
			t.Fatal(err)
		}
		metas = append(metas, m)
	}
	order, dups := OrderFiles(metas, synthSession)
	digests := func(order []int) string {
		var s []string
		for _, i := range order {
			s = append(s, metas[i].Digest[:8])
		}
		return strings.Join(s, ",")
	}
	if len(order) != 3 || len(dups) != 1 {
		t.Fatalf("order %v duplicates %v", order, dups)
	}
	if !metas[order[0]].Sessions[0].Main || !metas[order[1]].Sessions[0].Main || metas[order[2]].Sessions[0].Main {
		t.Errorf("a subagent file is not last: %v", order)
	}
	// Among a session's own files, the one that begins first comes first.
	if order[0] != 0 || order[1] != 2 || order[2] != 1 {
		t.Errorf("order %v, want the earlier-beginning file (0), then 2, then the subagent's (1)", order)
	}
	// Any permutation of the listing gives the same files in the same order.
	perm := [][]int{{3, 2, 1, 0}, {1, 0, 3, 2}, {2, 3, 0, 1}}
	for _, p := range perm {
		var pm []FileMeta
		for _, i := range p {
			pm = append(pm, metas[i])
		}
		o2, d2 := OrderFiles(pm, synthSession)
		var got []int
		for _, i := range o2 {
			got = append(got, p[i])
		}
		if len(d2) != 1 {
			t.Errorf("permutation %v: %d duplicates", p, len(d2))
		}
		if g, w := func() string {
			var s []string
			for _, i := range o2 {
				s = append(s, pm[i].Digest[:8])
			}
			return strings.Join(s, ",")
		}(), digests(order); g != w {
			t.Errorf("permutation %v: order %s, want %s", p, g, w)
		}
	}
	if m := metas[1]; m.Sessions[0].ID != synthSession || m.Sessions[0].Main {
		t.Errorf("the subagent file is %+v", m.Sessions)
	}
}

var _ = history.Pos{}

// TestREPLAY22b_TheFileCapDropsTheRestAndTaintsAll: a session of more files
// than the merge reads (MaxSessionFiles) is read for the first of them in
// order, the rest are counted and not read, and the whole timeline is tainted,
// because what is in the files not read could be anywhere in it.
func TestREPLAY22b_TheFileCapDropsTheRestAndTaintsAll(t *testing.T) {
	d := newDoc()
	turnOne(d)
	docs := make([]string, MaxSessionFiles+3)
	for i := range docs {
		docs[i] = d.String()
	}
	tl := session(t, len(docs), docs...)
	if tl.FilesDropped != 3 || len(tl.Files) != MaxSessionFiles {
		t.Fatalf("dropped %d, read %d files", tl.FilesDropped, len(tl.Files))
	}
	if tl.TaintReason != "session_file_cap" || tl.TaintFrom.IsZero() {
		t.Errorf("taint %+v (%s)", tl.TaintFrom, tl.TaintReason)
	}
	for _, e := range tl.Events {
		if e.Taint.IsZero() {
			t.Errorf("%s %s is not tainted", e.Kind, e.ToolUseID)
		}
	}
}

package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The timeline puts every call the session made, main agent and subagents, on
// one list in seq order, keeps a failure apart from a call that never ran,
// and says whether each failure succeeded later. These tests build runs in
// code, as chains_test.go does, and each names the break it catches.

type tlCall struct {
	seq     int64
	id      string
	tool    string
	program string
	digest  string
	agent   string
	typ     string
}

func tlDecl(c tlCall) store.Declaration {
	d := store.Declaration{
		Seq:          c.seq,
		RecordedAtMS: 1_700_000_000_000 + c.seq*1000,
		ToolUseID:    c.id,
		ToolName:     c.tool,
		SessionID:    "s1",
		Shape:        shape.Shape{VerbClass: "execute", Digest: c.digest},
	}
	if c.program != "" {
		p := c.program
		d.Shape.Program = &p
	}
	if c.agent != "" {
		a, t := c.agent, c.typ
		d.AgentID, d.AgentType = &a, &t
	}
	return d
}

func tlExec(id, outcome string, exit int) store.Execution {
	e := store.Execution{ToolUseID: id, Outcome: outcome}
	if exit != 0 {
		code := exit
		e.ExitCode = &code
	}
	return e
}

// tlRun gives each execution without a seq one in the order it is listed,
// after every declaration's: the order results were recorded in is the order
// a test writes them down, unless it says otherwise.
func tlRun(calls []tlCall, execs ...store.Execution) *store.Run {
	run := &store.Run{}
	for _, c := range calls {
		run.Declarations = append(run.Declarations, tlDecl(c))
	}
	for i := range execs {
		if execs[i].Seq == nil {
			seq := int64(1000 + i)
			execs[i].Seq = &seq
		}
	}
	run.Executions = execs
	return run
}

func tlExecAt(id, outcome string, exit int, seq int64) store.Execution {
	e := tlExec(id, outcome, exit)
	e.Seq = &seq
	return e
}

func tlByID(t *testing.T, tl Timeline, id string) TimelineCall {
	t.Helper()
	for _, c := range tl.Calls {
		if c.ToolUseID == id {
			return c
		}
	}
	t.Fatalf("no call %q on the timeline: %+v", id, tl.Calls)
	return TimelineCall{}
}

// T1, T2: one list across agents, in seq order, with each agent named.
// Break: build per agent, or sort by anything but seq, and the order a reader
// sees is not the order the session ran in.
func TestTimeline_OneListAcrossAgentsInSeqOrder(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 4, id: "sub2", tool: "Read", agent: "bbbb2222", typ: "Explore"},
		{seq: 1, id: "main1", tool: "Bash", program: "ls"},
		{seq: 3, id: "sub1", tool: "Read", agent: "aaaa1111", typ: "Explore"},
		{seq: 2, id: "main2", tool: "Edit"},
	})
	tl := buildTimeline(run, nil)

	var got []string
	for _, c := range tl.Calls {
		got = append(got, c.ToolUseID)
	}
	if strings.Join(got, ",") != "main1,main2,sub1,sub2" {
		t.Fatalf("order = %v, want main1,main2,sub1,sub2 (seq order across agents)", got)
	}
	if tl.Counts.Subagents != 2 {
		t.Errorf("subagents = %d, want 2", tl.Counts.Subagents)
	}
	if a, b := agentLabel(tlByID(t, tl, "sub1").Agent), agentLabel(tlByID(t, tl, "sub2").Agent); a == b {
		t.Errorf("two subagents of one type render the same label %q; the id prefix must tell them apart", a)
	}
	if got := agentLabel(&TimelineAgent{ID: "agent-cafe0001", Type: "Explore"}); got != "Explore·cafe" {
		t.Errorf("label = %q, want Explore·cafe: an agent- prefix would make every label read agen", got)
	}
	if got := agentLabel(tlByID(t, tl, "main1").Agent); got != "main" {
		t.Errorf("main agent label = %q, want main", got)
	}
}

// T3-T7: five groups, never merged. Break: fold denied into failed, or a
// missing execution record into never-ran, and the timeline asserts what the
// record cannot know.
func TestTimeline_GroupsAreKeptApart(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 1, id: "ok", tool: "Bash"},
		{seq: 2, id: "failed", tool: "Bash"},
		{seq: 3, id: "interrupted", tool: "Bash"},
		{seq: 4, id: "denied", tool: "Bash"},
		{seq: 5, id: "norecord", tool: "Bash"},
		{seq: 6, id: "v1", tool: "Bash"},
	},
		tlExec("ok", store.ExecOK, 0),
		tlExec("failed", store.ExecFailed, 2),
		tlExec("interrupted", store.ExecInterrupted, 0),
		tlExec("v1", "", 0),
	)
	run.Terminals = []store.Terminal{{ToolUseID: "dropped"}}
	tl := buildTimeline(run, map[string]bool{"denied": true})

	for id, want := range map[string]string{
		"ok":          GroupOK,
		"failed":      GroupFailed,
		"interrupted": GroupInterrupted,
		"denied":      GroupNeverRan,
		"norecord":    GroupUnknown,
		"v1":          GroupUnknown,
		"dropped":     GroupUnknown,
	} {
		if got := tlByID(t, tl, id).Group; got != want {
			t.Errorf("%s: group = %q, want %q", id, got, want)
		}
	}
	if c := tlByID(t, tl, "failed"); c.ExitCode == nil || *c.ExitCode != 2 {
		t.Errorf("failed call lost its exit code: %+v", c.ExitCode)
	}
	if c := tlByID(t, tl, "dropped"); c.Seq != nil {
		t.Errorf("a dropped call has no position in the ordered stream; got seq %d", *c.Seq)
	}
	n := tl.Counts
	if n.OK != 1 || n.Failed != 1 || n.Interrupted != 1 || n.NeverRan != 1 || n.Unknown != 3 {
		t.Errorf("counts = %+v, want ok 1, failed 1, interrupted 1, never ran 1, unknown 3", n)
	}
}

// T8: a call with no prompt id is still on the timeline, in its seq position.
// Break: build from the chains' prompt groups and it disappears.
func TestTimeline_UnattributedCallsKeepTheirPlace(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 1, id: "a", tool: "Bash"},
		{seq: 2, id: "noprompt", tool: "Bash"},
		{seq: 3, id: "b", tool: "Bash"},
	})
	p := "p1"
	run.Declarations[0].PromptID = &p
	run.Declarations[2].PromptID = &p
	tl := buildTimeline(run, nil)
	if len(tl.Calls) != 3 || tl.Calls[1].ToolUseID != "noprompt" {
		t.Fatalf("the unattributed call is missing or out of place: %+v", tl.Calls)
	}
}

// L1-L8: how a failure was followed up.
func TestTimeline_LaterSuccess(t *testing.T) {
	cases := []struct {
		name  string
		calls []tlCall
		execs []store.Execution
		want  *LaterSuccess
	}{{
		name: "L1 same command succeeds later",
		calls: []tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "x", tool: "Edit", digest: "e1"},
			{seq: 3, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
		},
		execs: []store.Execution{tlExec("f", store.ExecFailed, 1), tlExec("x", store.ExecOK, 0), tlExec("s", store.ExecOK, 0)},
		want:  &LaterSuccess{Kind: LaterSameCommand, Seq: 3},
	}, {
		name: "L2 same program, another command line",
		calls: []tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: "d2"},
		},
		execs: []store.Execution{tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0)},
		want:  &LaterSuccess{Kind: LaterSameProgram, Seq: 2},
	}, {
		name: "L3 a later failure is skipped for the success after it",
		calls: []tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "f2", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 3, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
		},
		execs: []store.Execution{tlExec("f", store.ExecFailed, 1), tlExec("f2", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0)},
		want:  &LaterSuccess{Kind: LaterSameCommand, Seq: 3},
	}, {
		name: "L3b a same-command success beats an earlier same-program one",
		calls: []tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "p", tool: "Bash", program: "pytest", digest: "d2"},
			{seq: 3, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
		},
		execs: []store.Execution{tlExec("f", store.ExecFailed, 1), tlExec("p", store.ExecOK, 0), tlExec("s", store.ExecOK, 0)},
		want:  &LaterSuccess{Kind: LaterSameCommand, Seq: 3},
	}, {
		name: "L4 no later success",
		calls: []tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "make", digest: "d1"},
			{seq: 2, id: "o", tool: "Bash", program: "ls", digest: "d9"},
		},
		execs: []store.Execution{tlExec("f", store.ExecFailed, 2), tlExec("o", store.ExecOK, 0)},
		want:  nil,
	}, {
		name: "L5 an earlier success does not count",
		calls: []tlCall{
			{seq: 1, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
		},
		execs: []store.Execution{tlExec("s", store.ExecOK, 0), tlExec("f", store.ExecFailed, 1)},
		want:  nil,
	}, {
		name: "L6 the same digest under another tool is another call",
		calls: []tlCall{
			{seq: 1, id: "f", tool: "Bash", digest: "d1"},
			{seq: 2, id: "s", tool: "WebFetch", digest: "d1"},
		},
		execs: []store.Execution{tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0)},
		want:  nil,
	}, {
		name: "L7 a subagent's success counts and is named",
		calls: []tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: "d1", agent: "cafe0001", typ: "general-purpose"},
		},
		execs: []store.Execution{tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0)},
		want:  &LaterSuccess{Kind: LaterSameCommand, Seq: 2, Agent: &TimelineAgent{ID: "cafe0001", Type: "general-purpose"}},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tl := buildTimeline(tlRun(tc.calls, tc.execs...), nil)
			got := tlByID(t, tl, "f").Later
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("later = %+v, want none", got)
			case tc.want != nil && got == nil:
				t.Fatalf("later = none, want %+v", tc.want)
			case tc.want != nil:
				if got.Kind != tc.want.Kind || got.Seq != tc.want.Seq {
					t.Fatalf("later = %s at %d, want %s at %d", got.Kind, got.Seq, tc.want.Kind, tc.want.Seq)
				}
				if (tc.want.Agent == nil) != (got.Agent == nil) ||
					(got.Agent != nil && got.Agent.ID != tc.want.Agent.ID) {
					t.Fatalf("later agent = %+v, want %+v", got.Agent, tc.want.Agent)
				}
			}
		})
	}
}

// L8: only a failed call is followed up. Break: annotate every row and a
// denied call reads as if it had been retried.
func TestTimeline_OnlyFailedCallsAreFollowedUp(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 1, id: "denied", tool: "Bash", program: "rm", digest: "d1"},
		{seq: 2, id: "norecord", tool: "Bash", program: "rm", digest: "d1"},
		{seq: 3, id: "stopped", tool: "Bash", program: "rm", digest: "d1"},
		{seq: 4, id: "s", tool: "Bash", program: "rm", digest: "d1"},
	}, tlExec("stopped", store.ExecInterrupted, 0), tlExec("s", store.ExecOK, 0))
	tl := buildTimeline(run, map[string]bool{"denied": true})
	for _, id := range []string{"denied", "norecord", "stopped", "s"} {
		if c := tlByID(t, tl, id); c.Later != nil {
			t.Errorf("%s (group %s) carries a later marker: %+v", id, c.Group, c.Later)
		}
	}
}

// C1: the failed count splits into its three follow-ups and they add up.
func TestTimeline_FailedCountsAddUp(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 1, id: "a", tool: "Bash", program: "pytest", digest: "d1"},
		{seq: 2, id: "b", tool: "Bash", program: "eslint", digest: "d2"},
		{seq: 3, id: "c", tool: "Bash", program: "make", digest: "d3"},
		{seq: 4, id: "a2", tool: "Bash", program: "pytest", digest: "d1"},
		{seq: 5, id: "b2", tool: "Bash", program: "eslint", digest: "d4"},
	},
		tlExec("a", store.ExecFailed, 1), tlExec("b", store.ExecFailed, 1), tlExec("c", store.ExecFailed, 1),
		tlExec("a2", store.ExecOK, 0), tlExec("b2", store.ExecOK, 0),
	)
	n := buildTimeline(run, nil).Counts
	if n.Failed != 3 || n.SameCommand != 1 || n.SameProgram != 1 || n.NoLater != 1 {
		t.Fatalf("counts = %+v, want failed 3 = 1 same command + 1 same program + 1 none", n)
	}
}

// P1: equality is computed inside the report and only its result leaves.
// Break: add the digest to TimelineCall and a report hands out the value the
// per-install key exists to keep on this machine.
func TestTimeline_JSONCarriesNoDigest(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "deadbeefdigest"},
		{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: "deadbeefdigest"},
	}, tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0))
	body, err := json.Marshal(buildTimeline(run, nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "deadbeefdigest") || strings.Contains(string(body), `"digest"`) {
		t.Fatalf("the timeline's JSON carries a digest:\n%s", body)
	}
}

// Text: the counts lead, each failure says how it was followed up, and the
// listing is capped with the remainder counted rather than cut.
func TestTimeline_Text(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
		{seq: 2, id: "g", tool: "Bash", program: "make", digest: "d2", agent: "cafe0001", typ: "general-purpose"},
		{seq: 3, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
	}, tlExec("f", store.ExecFailed, 1), tlExec("g", store.ExecFailed, 2), tlExec("s", store.ExecOK, 0))
	var b bytes.Buffer
	writeTimeline(&b, buildTimeline(run, nil))
	out := b.String()
	for _, want := range []string{
		"timeline: 3 calls (2 main agent, 1 from 1 subagent)",
		"failed       2  (1 same command ok, recorded after; 0 same program ok, recorded after; 1 no later success recorded)",
		"failed (exit 1)",
		"→ same command ok at 3, recorded after",
		"general-purpose·cafe",
		"failed (exit 2)",
		"→ no later success recorded",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text is missing %q:\n%s", want, out)
		}
	}
}

func TestTimeline_TextIsCappedAndSaysSo(t *testing.T) {
	var calls []tlCall
	for i := int64(1); i <= timelineRows+7; i++ {
		calls = append(calls, tlCall{seq: i, id: "c" + string(rune('a'+i%26)) + strings.Repeat("x", int(i%5)), tool: "Read"})
	}
	var b bytes.Buffer
	writeTimeline(&b, buildTimeline(tlRun(calls), nil))
	if !strings.Contains(b.String(), "7 more calls, see --json") {
		t.Fatalf("a capped listing must count what it left out:\n...%s", tail(b.String(), 300))
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// A report without --timeline renders exactly what it did before.
func TestTimeline_TextIsOptIn(t *testing.T) {
	run := tlRun([]tlCall{{seq: 1, id: "f", tool: "Bash"}}, tlExec("f", store.ExecFailed, 1))
	rep := &Report{Sessions: []Session{{SessionID: "s1", Timeline: buildTimeline(run, nil)}}}
	var plain, with bytes.Buffer
	if err := Text(&plain, rep); err != nil {
		t.Fatal(err)
	}
	if err := Text(&with, rep, WithTimeline()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "timeline:") {
		t.Errorf("the default render shows the timeline:\n%s", plain.String())
	}
	if !strings.Contains(with.String(), "timeline: 1 call") {
		t.Errorf("--timeline did not render it:\n%s", with.String())
	}
}

// #36 review 1: an execution record with no declaration and no terminal is a
// call the session made. Break: build the undeclared rows from Dropped() alone
// and it vanishes, and the timeline says "failed 0" beside a report whose
// failed-calls count says 1.
func TestTimeline_AnExecutionWithNoDeclarationIsOnTheList(t *testing.T) {
	run := tlRun([]tlCall{{seq: 1, id: "ok", tool: "Bash", program: "ls", digest: "d1"}},
		tlExec("ok", store.ExecOK, 0), tlExec("orphan", store.ExecFailed, 2))
	run.Executions[1].ToolName = "Bash"
	tl := buildTimeline(run, nil)

	c := tlByID(t, tl, "orphan")
	if c.Group != GroupFailed || c.ExitCode == nil || *c.ExitCode != 2 || c.ToolName != "Bash" {
		t.Errorf("the orphan execution lost what its record says: %+v", c)
	}
	if c.Seq != nil || !c.AgentUnknown || c.Agent != nil {
		t.Errorf("an undeclared call has no position and no known agent: %+v", c)
	}
	if got, want := tl.Counts.Failed, BuildSilentFailures(run, Account{}).Failed; got != want || got != 1 {
		t.Errorf("timeline failed = %d, report failed calls = %d; both must be 1", got, want)
	}
}

// #36 review 3: a dropped declaration whose execution landed. Break: render
// it as the main agent, or throw its execution away, and the row both names
// an agent the record does not and hides a failure it does.
func TestTimeline_ADroppedCallKeepsItsExecutionAndNoAgent(t *testing.T) {
	run := tlRun(nil, tlExec("dropped", store.ExecFailed, 2))
	run.Terminals = []store.Terminal{{ToolUseID: "dropped"}, {ToolUseID: "bare"}}
	tl := buildTimeline(run, nil)

	c := tlByID(t, tl, "dropped")
	if c.Group != GroupFailed || c.Outcome != store.ExecFailed || c.ExitCode == nil || *c.ExitCode != 2 {
		t.Errorf("the dropped call's execution was thrown away: %+v", c)
	}
	if !c.AgentUnknown {
		t.Errorf("a dropped call's agent is unknown, not the main agent: %+v", c)
	}
	if c.LaterChecked || c.Later != nil {
		t.Errorf("a call with no declaration has no command to compare, so it is not checked: %+v", c)
	}
	if b := tlByID(t, tl, "bare"); b.Group != GroupUnknown || b.Outcome != LinkOutcomeNoRecord {
		t.Errorf("a dropped call with no execution: %+v, want unknown / %s", b, LinkOutcomeNoRecord)
	}
	n := tl.Counts
	if n.MainAgent != 0 || n.AgentUnknown != 2 || n.Failed != 1 || n.NotChecked != 1 || n.NoLater != 0 {
		t.Errorf("counts = %+v, want 0 main, 2 agent unknown, 1 failed not checked", n)
	}

	var b bytes.Buffer
	writeTimeline(&b, tl)
	out := b.String()
	for _, row := range strings.Split(out, "\n") {
		if strings.Contains(row, "failed (exit 2)") {
			if strings.Contains(row, "main") || !strings.Contains(row, "unknown") {
				t.Errorf("the dropped row names an agent: %q", row)
			}
			if !strings.Contains(row, "no declaration recorded") || !strings.Contains(row, "not checked") {
				t.Errorf("the dropped row does not say what it lacks: %q", row)
			}
		}
	}
	if !strings.Contains(out, "2 calls with no declaration recorded") {
		t.Errorf("the undeclared calls are not accounted for:\n%s", out)
	}
}

// #36 review 2: "later" is when the result was recorded, not when the call
// was declared. Break: compare declaration seqs and both directions go wrong.
func TestTimeline_LaterIsByWhenTheResultWasRecorded(t *testing.T) {
	// Declared failure first, success second; the success's result landed
	// first. It is not a later success.
	early := tlRun([]tlCall{
		{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
		{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
	}, tlExecAt("s", store.ExecOK, 0, 10), tlExecAt("f", store.ExecFailed, 1, 11))
	if c := tlByID(t, buildTimeline(early, nil), "f"); c.Later != nil || !c.LaterChecked {
		t.Errorf("a success recorded before the failure counts as later: %+v", c)
	}

	// Declared success first, failure second; the success's result landed
	// after the failure's. It is one.
	late := tlRun([]tlCall{
		{seq: 1, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
		{seq: 2, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
	}, tlExecAt("f", store.ExecFailed, 1, 10), tlExecAt("s", store.ExecOK, 0, 11))
	c := tlByID(t, buildTimeline(late, nil), "f")
	if c.Later == nil || c.Later.Kind != LaterSameCommand || c.Later.Seq != 1 {
		t.Errorf("a success recorded after the failure is missed: %+v", c.Later)
	}

	// Of two, the FIRST recorded after wins, whatever the declaration order.
	two := tlRun([]tlCall{
		{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
		{seq: 2, id: "s2", tool: "Bash", program: "pytest", digest: "d1"},
		{seq: 3, id: "s3", tool: "Bash", program: "pytest", digest: "d1"},
	}, tlExecAt("f", store.ExecFailed, 1, 10), tlExecAt("s3", store.ExecOK, 0, 11), tlExecAt("s2", store.ExecOK, 0, 12))
	if c := tlByID(t, buildTimeline(two, nil), "f"); c.Later == nil || c.Later.Seq != 3 {
		t.Errorf("later = %+v, want the success recorded first after the failure (row 3)", c.Later)
	}

	// A failure record with no position cannot be followed by anything.
	nopos := tlRun([]tlCall{
		{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
		{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
	}, tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0))
	nopos.Executions[0].Seq = nil
	tl := buildTimeline(nopos, nil)
	if c := tlByID(t, tl, "f"); c.LaterChecked || c.Later != nil {
		t.Errorf("a failure with no position was compared: %+v", c)
	}
	if n := tl.Counts; n.NotChecked != 1 || n.NoLater != 0 {
		t.Errorf("counts = %+v, want the unpositioned failure not checked", n)
	}
}

// #36 review round 2: a matching success whose record has no seq (spilled
// when the append lock timed out) cannot be placed, so it may be the later
// one; with nothing placed found the failure is not checked, never "no later
// success recorded". A placed later success still answers. Break: skip the
// unplaced success as a candidate and the row claims there was none; or let
// it override a placed one and a real follow-up is thrown away.
func TestTimeline_AnUnplacedSuccessIsNotNoSuccess(t *testing.T) {
	unplaced := func(digest string, placed bool) *store.Run {
		calls := []tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: digest},
		}
		execs := []store.Execution{tlExecAt("f", store.ExecFailed, 1, 10), tlExec("s", store.ExecOK, 0)}
		if placed {
			calls = append(calls, tlCall{seq: 3, id: "p", tool: "Bash", program: "pytest", digest: "d9"})
			execs = append(execs, tlExecAt("p", store.ExecOK, 0, 11))
		}
		run := tlRun(calls, execs...)
		run.Executions[1].Seq = nil
		return run
	}
	for _, digest := range []string{"d1", "d2"} {
		tl := buildTimeline(unplaced(digest, false), nil)
		if c := tlByID(t, tl, "f"); c.LaterChecked || c.Later != nil {
			t.Errorf("digest %s: an unplaced success is read as no later success: %+v", digest, c)
		}
		if n := tl.Counts; n.NotChecked != 1 || n.NoLater != 0 {
			t.Errorf("digest %s: counts = %+v, want the failure not checked", digest, n)
		}
		var b bytes.Buffer
		writeTimeline(&b, tl)
		if out := b.String(); strings.Contains(out, "→ no later success") || !strings.Contains(out, "0 no later success recorded, 1 not checked") {
			t.Errorf("digest %s: the text claims no later success:\n%s", digest, out)
		}
	}
	c := tlByID(t, buildTimeline(unplaced("d1", true), nil), "f")
	if !c.LaterChecked || c.Later == nil || c.Later.Kind != LaterSameProgram || c.Later.Seq != 3 {
		t.Errorf("a placed later success is lost beside an unplaced one: %+v", c)
	}
}

// #36 review 4: the same-program tier makes no claim about arguments, and is
// not offered where the program does not name what ran. Break: pair `git
// status` with a failed `git push`, or print "different arguments" for a
// line that may differ only by a space.
func TestTimeline_SameProgramClaimsNoMore(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 1, id: "push", tool: "Bash", program: "git", digest: "d1"},
		{seq: 2, id: "status", tool: "Bash", program: "git", digest: "d2"},
		{seq: 3, id: "f", tool: "Bash", program: "pytest", digest: "d3"},
		{seq: 4, id: "s", tool: "Bash", program: "pytest", digest: "d4"},
	}, tlExec("push", store.ExecFailed, 1), tlExec("status", store.ExecOK, 0),
		tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0))
	tl := buildTimeline(run, nil)
	if c := tlByID(t, tl, "push"); c.Later != nil {
		t.Errorf("git status is offered as a later success of git push: %+v", c.Later)
	}
	var b bytes.Buffer
	writeTimeline(&b, tl)
	out := b.String()
	if !strings.Contains(out, "→ same program ok at 4, recorded after") {
		t.Errorf("the same-program row is missing:\n%s", out)
	}
	if strings.Contains(out, "argument") {
		t.Errorf("the text claims something about arguments a digest cannot show:\n%s", out)
	}
}

// #36 review 5: agent_type and agent_id come from the payload. Break: print
// them raw and an escape sequence reaches the terminal; cut the id by byte
// and a character is split.
func TestTimeline_AgentLabelIsPrintable(t *testing.T) {
	got := agentLabel(&TimelineAgent{ID: "agent-\x07a\u00e9\u00e9\u00e9\u00e9", Type: "Expl\x1bore\u202e\n\x07"})
	if got != "Explore·a\u00e9\u00e9\u00e9" {
		t.Errorf("label = %q, want Explore·a\u00e9\u00e9\u00e9", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("label %q is not valid UTF-8: the id was cut inside a character", got)
	}
}

// #36 review 6, 7, 9: the legend names what the unknown group holds; "no
// later success" says it is about the record; the time column says UTC and
// the date is shown; a main agent that made no calls is not named as a
// participant.
func TestTimeline_TextSaysWhatItKnows(t *testing.T) {
	run := tlRun([]tlCall{
		{seq: 1, id: "v1", tool: "Bash", agent: "cafe0001", typ: "Explore"},
		{seq: 2, id: "f", tool: "Bash", program: "make", digest: "d1", agent: "cafe0001", typ: "Explore"},
	}, tlExec("v1", "", 0), tlExec("f", store.ExecFailed, 2))
	var b bytes.Buffer
	writeTimeline(&b, buildTimeline(run, nil))
	out := b.String()
	for _, want := range []string{
		"timeline: 2 calls (0 main agent, 2 from 1 subagent)",
		"outcome unobserved: it ran",
		"→ no later success recorded",
		"UTC",
		"2023-11-14 (UTC)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "main agent +") {
		t.Errorf("the header names a main agent that made no calls:\n%s", out)
	}
}

// A date line is printed again where the date changes.
func TestTimeline_TextMarksEachNewDate(t *testing.T) {
	run := tlRun([]tlCall{{seq: 1, id: "a", tool: "Read"}, {seq: 2, id: "b", tool: "Read"}})
	run.Declarations[1].RecordedAtMS += 86_400_000
	var b bytes.Buffer
	writeTimeline(&b, buildTimeline(run, nil))
	if !strings.Contains(b.String(), "2023-11-14 (UTC)") || !strings.Contains(b.String(), "2023-11-15 (UTC)") {
		t.Errorf("a timeline across midnight must show both dates:\n%s", b.String())
	}
}

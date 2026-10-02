package report

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	// One spelling for every enum in the JSON: later.kind and the counts'
	// keys are snake case, and so is the group.
	body, err := json.Marshal(tlByID(t, tl, "denied"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"group":"never_ran"`) {
		t.Errorf("a denied call's group is not never_ran:\n%s", body)
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
	}, {
		name: "L7b a subagent's same-program success is named too",
		calls: []tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: "d2", agent: "cafe0001", typ: "general-purpose"},
		},
		execs: []store.Execution{tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0)},
		want:  &LaterSuccess{Kind: LaterSameProgram, Seq: 2, Agent: &TimelineAgent{ID: "cafe0001", Type: "general-purpose"}},
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
		{seq: 4, id: "i", tool: "Bash", program: "sleep", digest: "d3"},
	}, tlExec("f", store.ExecFailed, 1), tlExec("g", store.ExecFailed, 2), tlExec("s", store.ExecOK, 0),
		tlExec("i", store.ExecInterrupted, 0))
	var b bytes.Buffer
	writeTimeline(&b, buildTimeline(run, nil))
	out := b.String()
	for _, want := range []string{
		"timeline: 4 calls (3 main agent, 1 from 1 subagent)",
		"interrupted  1\n",
		"calls from agents running at once interleave by when each was recorded, not when it started",
		"failed       2  (1 same command ok, recorded after; 0 same program ok, recorded after; 1 no later success of the same command or program recorded)",
		"a fix made with a different command, or a corrected Edit, is not detected",
		"failed (exit 1)",
		"→ same command ok at 3, recorded after",
		"general-purpose·cafe",
		"failed (exit 2)",
		"→ no later success of the same command recorded",
		// #36 review round 3, smaller 8: seq is rashomon's own position, so
		// the reader is told where the id that finds a call is.
		"    rows omit tool_use_id; --json carries it for every call\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text is missing %q:\n%s", want, out)
		}
	}
}

// Break: drop the stop after the trailer and every row is still printed
// beneath a line saying they were not.
func TestTimeline_TextIsCappedAndSaysSo(t *testing.T) {
	var calls []tlCall
	for i := int64(1); i <= timelineRows+7; i++ {
		calls = append(calls, tlCall{seq: i, id: fmt.Sprintf("c%03d", i), tool: "Read"})
	}
	tl := buildTimeline(tlRun(calls), nil)
	if len(tl.Calls) != timelineRows+7 {
		t.Fatalf("calls = %d, want %d: the ids must be distinct", len(tl.Calls), timelineRows+7)
	}
	var b bytes.Buffer
	writeTimeline(&b, tl)
	out := b.String()
	if !strings.Contains(out, "7 more calls, see --json") {
		t.Fatalf("a capped listing must count what it left out:\n...%s", tail(out, 300))
	}
	if rows := strings.Count(out, " Read "); rows != timelineRows {
		t.Errorf("rows = %d, want %d: the listing is not capped", rows, timelineRows)
	}
	if strings.Contains(out, fmt.Sprintf(" %d  ", timelineRows+1)) {
		t.Errorf("row %d is printed past the cap:\n...%s", timelineRows+1, tail(out, 300))
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

// #36 review round 3, fix 1 and smaller 5: one tool_use_id with two
// execution records. The call is failed when ANY of its records failed, and
// its exit code and "recorded after" position come from the failed record,
// whatever order the store holds them in. The failed count agrees with the
// report's per call: two failed records are one failed call here. Break:
// take the outcome from the highest-seq record alone, and a failure followed
// by an ok record of the same id reads ok; read the exit code or position
// from another record, and the row carries the wrong code or measures "later"
// from a record the outcome did not come from.
func TestTimeline_TwoRecordsForOneID(t *testing.T) {
	cases := []struct {
		name string
		// The two records of "f"; "s", a same-command success, lands at 11.
		recs  []store.Execution
		later bool
	}{
		// ok, then failed: the success at 11 was recorded before the failure.
		{"ok then failed", []store.Execution{tlExecAt("f", store.ExecOK, 0, 10), tlExecAt("f", store.ExecFailed, 2, 12)}, false},
		// failed, then ok: the failure is still the call's, and the success
		// at 11 was recorded after it.
		{"failed then ok", []store.Execution{tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("f", store.ExecOK, 0, 12)}, true},
		// Two failures: the last one is the call's, and the success at 11 is
		// followed by it, not after it.
		{"failed then failed", []store.Execution{tlExecAt("f", store.ExecFailed, 1, 10), tlExecAt("f", store.ExecFailed, 2, 12)}, false},
	}
	for _, tc := range cases {
		for _, order := range []string{"ascending", "descending"} {
			t.Run(tc.name+"/"+order, func(t *testing.T) {
				recs := []store.Execution{tc.recs[0], tc.recs[1]}
				if order == "descending" {
					recs = []store.Execution{tc.recs[1], tc.recs[0]}
				}
				recs = append(recs, tlExecAt("s", store.ExecOK, 0, 11))
				run := tlRun([]tlCall{
					{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
					{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
				}, recs...)
				tl := buildTimeline(run, nil)
				c := tlByID(t, tl, "f")
				if c.Group != GroupFailed || c.Outcome != store.ExecFailed {
					t.Errorf("group = %s / %s, want failed: a call any of whose records failed is failed", c.Group, c.Outcome)
				}
				if tl.Counts.Failed != 1 || tl.Counts.OK != 1 {
					t.Errorf("counts = %+v, want 1 failed and 1 ok: the count is per call", tl.Counts)
				}
				if c.ExitCode == nil || *c.ExitCode != 2 {
					t.Errorf("exit code = %v, want 2, from the record the outcome came from", c.ExitCode)
				}
				if !c.LaterChecked {
					t.Fatalf("not checked: %+v", c)
				}
				if tc.later && (c.Later == nil || c.Later.Kind != LaterSameCommand || c.Later.Seq != 2) {
					t.Errorf("later = %+v, want same command at 2: the success was recorded after the failed record", c.Later)
				}
				if !tc.later && c.Later != nil {
					t.Errorf("later = %+v: a success recorded before the failed record is not after it", c.Later)
				}
			})
		}
	}

	// #36 review round 4, item 1: a failed call's own ok record is a success
	// of the same command, recorded where it was. It has no row of its own to
	// point at -- the row says failed -- so recorded after a failure it
	// matches, that failure is not checked, never "no later success". Break:
	// look only at ok rows, and both pages below say no later success beside
	// an ok record of the same command written after the failure.
	for name, run := range map[string]*store.Run{
		// One call: failed at 10, ok at 12.
		"its own": tlRun([]tlCall{{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"}},
			tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("f", store.ExecOK, 0, 12)),
		// g failed at 5; f, the same command, failed at 10 and was ok at 11.
		"another's": tlRun([]tlCall{
			{seq: 1, id: "g", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
		}, tlExecAt("g", store.ExecFailed, 1, 5), tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("f", store.ExecOK, 0, 11)),
	} {
		tl := buildTimeline(run, nil)
		for _, c := range tl.Calls {
			if c.Group != GroupFailed || c.LaterChecked || c.Later != nil {
				t.Errorf("%s ok record: %s = %s, later %+v, checked %v; want failed and not checked", name, c.ToolUseID, c.Group, c.Later, c.LaterChecked)
			}
		}
		var b bytes.Buffer
		writeTimeline(&b, tl)
		if out := b.String(); strings.Contains(out, "→ no later success") {
			t.Errorf("%s ok record: a row reads no later success:\n%s", name, out)
		}
	}
	// The failure's digest is its outcome record's, not its last record's:
	// f failed at 10 running d1 and was ok at 12 running d9, and s ran d1 at
	// 11. Break: take the digest from the record at 12 and s is not the same
	// command, while f's own ok record is.
	own := tlRun([]tlCall{
		{seq: 1, id: "f", tool: "Bash", digest: "d1"},
		{seq: 2, id: "s", tool: "Bash", digest: "d1"},
	}, tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("s", store.ExecOK, 0, 11), tlExecAt("f", store.ExecOK, 0, 12))
	own.Executions[2].ExecutedDigest = "d9"
	if c := tlByID(t, buildTimeline(own, nil), "f"); c.Later == nil || c.Later.Kind != LaterSameCommand || c.Later.Seq != 2 {
		t.Errorf("later = %+v, checked %v; want same command at 2: the digest is the outcome record's", c.Later, c.LaterChecked)
	}
	// Recorded before the failure, the ok record is no later success.
	before := tlRun([]tlCall{{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"}},
		tlExecAt("f", store.ExecOK, 0, 10), tlExecAt("f", store.ExecFailed, 2, 12))
	if c := tlByID(t, buildTimeline(before, nil), "f"); !c.LaterChecked || c.Later != nil {
		t.Errorf("an ok record before the failure leaves it unchecked: %+v", c)
	}

	// #36 review round 5, fix 1: each half of the ok-record match on its own.
	// Break any one of them and the case naming it goes the other way.
	nilSeq := tlRun([]tlCall{{seq: 1, id: "f", tool: "Edit", digest: "d1"}},
		tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("f", store.ExecOK, 0, 12))
	nilSeq.Executions[1].Seq = nil
	ranFailed := tlRun([]tlCall{{seq: 1, id: "f", tool: "Edit", digest: "d2"}},
		tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("f", store.ExecOK, 0, 12))
	ranFailed.Executions[0].ExecutedDigest = "d1"
	ranFailed.Executions[1].ExecutedDigest = "d1"
	for _, tc := range []struct {
		name    string
		run     *store.Run
		checked bool
	}{
		// No program tier: the ok record matches by command alone.
		{"same command, no program", tlRun([]tlCall{{seq: 1, id: "f", tool: "Edit", digest: "d1"}},
			tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("f", store.ExecOK, 0, 12)), false},
		// Another command line of a single-purpose program matches by program.
		{"same program", tlRun([]tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "g", tool: "Bash", program: "pytest", digest: "d2"},
		}, tlExecAt("g", store.ExecFailed, 1, 5), tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("g", store.ExecOK, 0, 11)), false},
		// Not where the program has no program tier: git d2 says nothing
		// about git d1.
		{"same program, no tier", tlRun([]tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "git", digest: "d1"},
			{seq: 2, id: "g", tool: "Bash", program: "git", digest: "d2"},
		}, tlExecAt("g", store.ExecFailed, 1, 5), tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("g", store.ExecOK, 0, 11)), true},
		// An ok record with no position may be the later one.
		{"no position", nilSeq, false},
		// The LAST ok record is weighed: ok at 5 is before the failure, ok
		// at 12 after it.
		{"ok, failed, ok", tlRun([]tlCall{{seq: 1, id: "f", tool: "Edit", digest: "d1"}},
			tlExecAt("f", store.ExecOK, 0, 5), tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("f", store.ExecOK, 0, 12)), false},
		// An unknown digest matches nothing, not every other unknown one.
		{"both digests empty", tlRun([]tlCall{
			{seq: 1, id: "f", tool: "Edit"},
			{seq: 2, id: "g", tool: "Edit"},
		}, tlExecAt("g", store.ExecFailed, 1, 5), tlExecAt("f", store.ExecFailed, 2, 10), tlExecAt("g", store.ExecOK, 0, 12)), true},
		// The ok record's digest is the one it ran, not the declared one.
		{"ran the failed command", ranFailed, false},
	} {
		c := tlByID(t, buildTimeline(tc.run, nil), "f")
		if c.Group != GroupFailed || c.Later != nil || c.LaterChecked != tc.checked {
			t.Errorf("%s: %s, later %+v, checked %v; want failed, no later, checked %v",
				tc.name, c.Group, c.Later, c.LaterChecked, tc.checked)
		}
	}
}

// #36 review round 3, smaller 1: undeclared rows have no declaration, but
// their execution records have positions, and they read in that order: by the
// seq of the record each outcome is read from (the last failed one, otherwise
// the last), those with no position last, ties by id. Break: sort by
// tool_use_id and they come out in an order that means nothing; sort by the
// last record and ee, failed at 5 and ok at 50, sits where its row's outcome
// was not recorded.
func TestTimeline_UndeclaredCallsFollowTheirResults(t *testing.T) {
	run := tlRun([]tlCall{{seq: 1, id: "d", tool: "Bash"}},
		tlExecAt("zz", store.ExecOK, 0, 20),
		tlExecAt("aa", store.ExecFailed, 1, 30),
		tlExecAt("cc", store.ExecOK, 0, 20),
		tlExec("bb", store.ExecOK, 0),
		tlExecAt("ee", store.ExecFailed, 1, 5),
		tlExecAt("ee", store.ExecOK, 0, 50),
		tlExecAt("aa", store.ExecFailed, 4, 25),
	)
	run.Executions[3].Seq = nil
	run.Terminals = []store.Terminal{{ToolUseID: "mm"}}
	tl := buildTimeline(run, nil)
	var got []string
	for _, c := range tl.Calls {
		got = append(got, c.ToolUseID)
	}
	if strings.Join(got, ",") != "d,ee,cc,zz,aa,bb,mm" {
		t.Errorf("order = %v, want d,ee,cc,zz,aa,bb,mm: declared first, then by the outcome record's seq, unpositioned last, ties by id", got)
	}
	// aa failed at 25 with exit 4 and at 30 with exit 1: the row's exit code
	// is the outcome record's, the last failed one.
	if c := tlByID(t, tl, "aa"); c.ExitCode == nil || *c.ExitCode != 1 {
		t.Errorf("aa: exit code = %v, want 1, from the record its outcome is read from", c.ExitCode)
	}
	var b bytes.Buffer
	writeTimeline(&b, tl)
	if !strings.Contains(b.String(), "listed last in the order their results were recorded: agent, program, and the declaration's position and time unknown") {
		t.Errorf("the legend does not say what an undeclared row lacks:\n%s", b.String())
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
		if out := b.String(); strings.Contains(out, "→ no later success") || !strings.Contains(out, "0 no later success of the same command or program recorded, 1 not checked") {
			t.Errorf("digest %s: the text claims no later success:\n%s", digest, out)
		}
	}
	c := tlByID(t, buildTimeline(unplaced("d1", true), nil), "f")
	if !c.LaterChecked || c.Later == nil || c.Later.Kind != LaterSameProgram || c.Later.Seq != 3 {
		t.Errorf("a placed later success is lost beside an unplaced one: %+v", c)
	}
}

// #36 review round 3, fix 1: the same command is the command that RAN. A
// PreToolUse hook can rewrite a call's input, and the execution record's
// digest is then the one that counts. Break: compare declared digests only and
// a success that ran something else reads as a re-run of the failure -- while
// the same page lists that call as executed differently from declared.
func TestTimeline_SameCommandIsTheCommandThatRan(t *testing.T) {
	run := func(executed string) *store.Run {
		r := tlRun([]tlCall{
			{seq: 1, id: "f", tool: "Bash", digest: "d1"},
			{seq: 2, id: "s", tool: "Bash", digest: "d1"},
		}, tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0))
		r.Executions[1].ExecutedDigest = executed
		return r
	}
	if c := tlByID(t, buildTimeline(run("d9"), nil), "f"); c.Later != nil || !c.LaterChecked {
		t.Errorf("a success rewritten to run something else is the same command: %+v", c.Later)
	}
	if c := tlByID(t, buildTimeline(run("d1"), nil), "f"); c.Later == nil || c.Later.Kind != LaterSameCommand {
		t.Errorf("a success that ran the declared command is lost: %+v", c.Later)
	}

	// The other way round: declared differently, but ran the failed command.
	r := tlRun([]tlCall{
		{seq: 1, id: "f", tool: "Bash", digest: "d1"},
		{seq: 2, id: "s", tool: "Bash", digest: "d2"},
	}, tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0))
	r.Executions[1].ExecutedDigest = "d1"
	if c := tlByID(t, buildTimeline(r, nil), "f"); c.Later == nil || c.Later.Kind != LaterSameCommand {
		t.Errorf("a success that ran the failed command is not the same command: %+v", c.Later)
	}
}

// #36 review round 4, item 2: the record keeps no executed program, so a call
// a PreToolUse hook rewrote may have run another program than its declared
// one. Either side rewritten, the same-program tier cannot be judged, and with
// nothing else found the failure is not checked. Break: compare the declared
// programs and a failed `pytest -q` reads as followed up by a "pytest" row
// that ran `echo passed`.
func TestTimeline_ARewrittenCallIsNotTheSameProgram(t *testing.T) {
	run := func(rewritten string) *store.Run {
		r := tlRun([]tlCall{
			{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"},
			{seq: 2, id: "s", tool: "Bash", program: "pytest", digest: "d1"},
		}, tlExecAt("f", store.ExecFailed, 1, 10), tlExecAt("s", store.ExecOK, 0, 11))
		for i := range r.Executions {
			if r.Executions[i].ToolUseID == rewritten {
				r.Executions[i].ExecutedDigest = "d9"
			}
		}
		return r
	}
	for _, rewritten := range []string{"s", "f"} {
		c := tlByID(t, buildTimeline(run(rewritten), nil), "f")
		if c.Later != nil || c.LaterChecked {
			t.Errorf("%s rewritten: later = %+v, checked = %v; want not checked", rewritten, c.Later, c.LaterChecked)
		}
	}
	// A rewritten success recorded before the failure is no later one, and
	// leaves nothing in doubt.
	early := run("s")
	early.Executions[1].Seq = new(int64)
	if c := tlByID(t, buildTimeline(early, nil), "f"); c.Later != nil || !c.LaterChecked {
		t.Errorf("an earlier rewritten success leaves the failure unchecked: %+v", c)
	}
}

// #36 review round 3, fix 2: a success whose declaration was lost -- a lock
// timeout, a paused pre hook, a failing PreToolUse hook -- has no program and
// no row, but may be the same command. Recorded after the failure or at no
// known position, it leaves the failure not checked; recorded before, it is
// no later success. Break: skip it for its empty digest and the failure reads
// "no later success" beside the success that answers it.
func TestTimeline_AnUndeclaredSuccessIsNotNoSuccess(t *testing.T) {
	run := func(pos *int64) *store.Run {
		r := tlRun([]tlCall{{seq: 1, id: "f", tool: "Bash", program: "pytest", digest: "d1"}},
			tlExecAt("f", store.ExecFailed, 1, 10), tlExec("orphan", store.ExecOK, 0))
		r.Executions[1].ToolName = "Bash"
		r.Executions[1].ExecutedDigest = "d1"
		r.Executions[1].Seq = pos
		return r
	}
	later, earlier := int64(11), int64(9)
	for name, pos := range map[string]*int64{"later": &later, "unplaced": nil} {
		tl := buildTimeline(run(pos), nil)
		if c := tlByID(t, tl, "f"); c.LaterChecked || c.Later != nil {
			t.Errorf("%s: an undeclared success is read as no later success: %+v", name, c)
		}
		if n := tl.Counts; n.NotChecked != 1 || n.NoLater != 0 {
			t.Errorf("%s: counts = %+v, want the failure not checked", name, n)
		}
	}
	if c := tlByID(t, buildTimeline(run(&earlier), nil), "f"); !c.LaterChecked || c.Later != nil {
		t.Errorf("an undeclared success recorded before the failure is not a later one: %+v", c)
	}
	// Another tool's undeclared success is no candidate at all.
	other := run(&later)
	other.Executions[1].ToolName = "Edit"
	if c := tlByID(t, buildTimeline(other, nil), "f"); !c.LaterChecked || c.Later != nil {
		t.Errorf("an undeclared success of another tool leaves the failure unchecked: %+v", c)
	}

	// #36 review round 4, fix 1: only the declaration is lost; the execution
	// record says what ran. A known, different digest rules the success out
	// where no program tier is offered (git, an Edit), and cannot where one is
	// (pytest), nor where the digest is unknown. Break: append undeclared
	// entries with no digest and every one of them leaves the failure
	// unchecked.
	for _, tc := range []struct {
		program, executed string
		checked           bool
	}{
		{"git", "d2", true},
		{"", "d2", true},
		{"pytest", "d2", false},
		{"git", "", false},
	} {
		r := run(&later)
		r.Declarations[0].Shape.Program = nil
		if tc.program != "" {
			r.Declarations[0].Shape.Program = &tc.program
		}
		r.Executions[1].ExecutedDigest = tc.executed
		if c := tlByID(t, buildTimeline(r, nil), "f"); c.LaterChecked != tc.checked || c.Later != nil {
			t.Errorf("program %q, undeclared success ran %q: later %+v, checked %v; want checked %v",
				tc.program, tc.executed, c.Later, c.LaterChecked, tc.checked)
		}
	}
	// A record with no tool_name may be a success of the same tool. Break:
	// skip it for its tool name and the failure reads "no later success".
	unknown := run(&later)
	unknown.Executions[1].ToolName = ""
	if c := tlByID(t, buildTimeline(unknown, nil), "f"); c.LaterChecked || c.Later != nil {
		t.Errorf("an undeclared success with no tool name is ruled out: %+v", c)
	}

	// #36 review round 5, item 1: an undeclared call that failed and then
	// succeeded is failed, but its ok record is a success whose declaration
	// was lost all the same, and the same rule weighs it. Break: apply the
	// rule to ok rows only, and the failure reads "no later success" beside
	// an ok record it cannot rule out.
	for _, tc := range []struct {
		name, program, tool, executed string
	}{
		{"unknown digest", "pytest", "Bash", ""},
		{"other digest under a program tier", "pytest", "Bash", "d2"},
		{"no tool name", "git", "", "dx"},
	} {
		r := tlRun([]tlCall{{seq: 1, id: "f", tool: "Bash", program: tc.program, digest: "d1"}},
			tlExecAt("f", store.ExecFailed, 1, 10), tlExecAt("u", store.ExecFailed, 1, 5), tlExecAt("u", store.ExecOK, 0, 12))
		for i := 1; i < 3; i++ {
			r.Executions[i].ToolName = tc.tool
		}
		r.Executions[1].ExecutedDigest = "d1"
		r.Executions[2].ExecutedDigest = tc.executed
		tl := buildTimeline(r, nil)
		if u := tlByID(t, tl, "u"); u.Group != GroupFailed {
			t.Fatalf("%s: u = %s, want failed", tc.name, u.Group)
		}
		if c := tlByID(t, tl, "f"); c.LaterChecked || c.Later != nil {
			t.Errorf("%s: an undeclared failed call's ok record is ruled out: later %+v, checked %v", tc.name, c.Later, c.LaterChecked)
		}
	}

	// A record with no tool_name was digested under no tool name, and the
	// digest covers the name: the same `git push` digests differently under
	// "" than under Bash, so it rules nothing out. Break: rule an unnamed
	// record out on its digest, and the failure reads "no later success"
	// beside the same command run again.
	key := []byte("timeline-test-key")
	push := json.RawMessage(`{"command":"git push"}`)
	named, unnamed := shape.Derive("Bash", push, key), shape.Derive("", push, key)
	gitRun := tlRun([]tlCall{{seq: 1, id: "f", tool: "Bash", program: "git", digest: named.Digest}},
		tlExecAt("f", store.ExecFailed, 1, 10), tlExecAt("u", store.ExecOK, 0, 12))
	gitRun.Executions[1].ExecutedDigest = unnamed.Digest
	if named.Digest == unnamed.Digest {
		t.Fatalf("the digest does not cover the tool name: %s", named.Digest)
	}
	if c := tlByID(t, buildTimeline(gitRun, nil), "f"); c.LaterChecked || c.Later != nil {
		t.Errorf("an unnamed record of the same git push is ruled out: later %+v, checked %v", c.Later, c.LaterChecked)
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

// #36 review round 4, item 3: the marker names what was looked for. For a
// program whose next word names what ran, only the same command is; for a
// single-purpose one, the same program too. Break: print "or program" on a
// failed `go vet` and the row says a later `go` success was looked for, beside
// the `go test` row that passed.
func TestTimeline_TheMarkerSaysWhatWasChecked(t *testing.T) {
	for _, tc := range []struct{ program, want string }{
		{"go", "  → no later success of the same command recorded"},
		{"", "  → no later success of the same command recorded"},
		{"pytest", "  → no later success of the same command or program recorded"},
	} {
		calls := []tlCall{{seq: 1, id: "f", tool: "Bash", program: tc.program, digest: "d1"}}
		execs := []store.Execution{tlExec("f", store.ExecFailed, 1)}
		if tc.program == "go" {
			calls = append(calls, tlCall{seq: 2, id: "s", tool: "Bash", program: "go", digest: "d2"})
			execs = append(execs, tlExec("s", store.ExecOK, 0))
		}
		c := tlByID(t, buildTimeline(tlRun(calls, execs...), nil), "f")
		if got := laterLabel(c); got != tc.want {
			t.Errorf("program %q: marker = %q, want %q", tc.program, got, tc.want)
		}
	}
	var b bytes.Buffer
	writeTimeline(&b, buildTimeline(tlRun([]tlCall{{seq: 1, id: "f", tool: "Bash", program: "go", digest: "d1"}},
		tlExec("f", store.ExecFailed, 1)), nil))
	if !strings.Contains(b.String(), "(only a later run of the same command, or of the same program for single-purpose programs, is looked for; "+
		"for wrappers and multi-command programs such as git, go, make, npm, python and sudo only the same command is; "+
		"a fix made with a different command, or a corrected Edit, is not detected)\n") {
		t.Errorf("the legend does not say which programs get only the same command:\n%s", b.String())
	}
}

// #36 review round 3, decision 2: a wrapper or a versioned interpreter is
// the first word, so it is the program, and it names nothing about what ran.
// Every name in the list is walked, named here so that dropping one from the
// list fails, and the suffixed names of one. Break: leave one out and `sudo
// ls` follows up a failed `sudo systemctl restart nginx`, or `ssh prod uptime`
// a failed `ssh prod systemctl restart nginx`, as "same program ok".
func TestTimeline_AWrapperIsNotTheProgram(t *testing.T) {
	listed := []string{
		"git", "gh", "go", "cargo", "make",
		"npm", "npx", "pnpm", "yarn", "bun", "deno",
		"pip", "pip3", "uv", "poetry",
		"docker", "podman", "kubectl", "helm", "terraform",
		"aws", "gcloud", "az", "dotnet", "mvn", "gradle",
		"brew", "apt", "apt-get", "systemctl",
		"python", "python3", "node", "nodejs", "ruby", "perl",
		"bash", "sh", "zsh",
		"pipx", "uvx", "bunx", "pnpx", "bundle", "pipenv",
		"conda", "nix", "direnv", "mise",
		"sudo", "doas", "su", "runuser", "chroot", "ssh",
		"env", "timeout", "gtimeout", "time", "gtime", "nohup",
		"nice", "ionice", "setsid", "flock", "strace", "parallel",
		"xargs", "watch", "stdbuf", "unbuffer", "xvfb-run",
		"exec", "command",
	}
	if len(listed) != len(subcommandPrograms) {
		t.Errorf("the test walks %d names, the list holds %d: walk every one", len(listed), len(subcommandPrograms))
	}
	programs := append([]string{"python3.12", "pip3.11", "node18", "python3.13t", "python3.12d", "pythonw", "python.exe", "python3.exe"}, listed...)
	for _, program := range programs {
		run := tlRun([]tlCall{
			{seq: 1, id: "f", tool: "Bash", program: program, digest: "d1"},
			{seq: 2, id: "s", tool: "Bash", program: program, digest: "d2"},
			{seq: 3, id: "same", tool: "Bash", program: program, digest: "d1"},
		}, tlExec("f", store.ExecFailed, 1), tlExec("s", store.ExecOK, 0))
		if c := tlByID(t, buildTimeline(run, nil), "f"); c.Later != nil || !c.LaterChecked {
			t.Errorf("%s: another %s line is offered as a later success: %+v", program, program, c.Later)
		}
		// The same line is still the same command.
		run.Executions = append(run.Executions, tlExecAt("same", store.ExecOK, 0, 2000))
		if c := tlByID(t, buildTimeline(run, nil), "f"); c.Later == nil || c.Later.Kind != LaterSameCommand {
			t.Errorf("%s: the same line run again is not the same command: %+v", program, c.Later)
		}
	}
	// A name that only ends in digits is not a versioned interpreter.
	for _, program := range []string{"pytest", "b2", "gpg2", "show", "w"} {
		if subcommandProgram(program) {
			t.Errorf("%s is treated as a subcommand program", program)
		}
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

// #36 review round 3, smaller 2: tool_name comes from the payload as
// agent_type does, and is printed on every row. Break: print it raw and an
// escape sequence or a bidi override reaches the terminal, declared row or
// undeclared.
func TestTimeline_ToolNameIsPrintable(t *testing.T) {
	const raw = "Ba\x1b[2Jsh\u202e\x07"
	run := tlRun([]tlCall{{seq: 1, id: "d", tool: raw}}, tlExec("d", store.ExecOK, 0), tlExec("u", store.ExecFailed, 1))
	run.Executions[1].ToolName = raw
	var b bytes.Buffer
	writeTimeline(&b, buildTimeline(run, nil))
	out := b.String()
	if strings.ContainsAny(out, "\x1b\u202e\x07") {
		t.Errorf("a control character in tool_name reaches the text:\n%q", out)
	}
	if strings.Count(out, "Ba[2Jsh") != 2 {
		t.Errorf("the tool name is not shown, made printable, on both rows:\n%s", out)
	}
}

// The same for the report's other listings of a payload's names: the by-tool
// counts, the --chain rows, the dropped-call ids, the "executed differently
// from declared" list on every default report, and the timeline's program.
// Each shows the name, made printable. Break: print one raw and a control
// character reaches the terminal; blank it and the listing names nothing.
func TestText_ToolNameIsPrintableInEveryListing(t *testing.T) {
	const raw, clean = "Ba\x1b[2Jsh\u202e", "Ba[2Jsh"
	for name, write := range map[string]func(*bytes.Buffer){
		"by tool":   func(b *bytes.Buffer) { b.WriteString(byName(map[string]int{raw: 1})) },
		"chain row": func(b *bytes.Buffer) { writeLink(b, Link{Seq: 1, ToolName: raw, VerbClass: "execute"}, false) },
		"dropped id": func(b *bytes.Buffer) {
			writeChainTail(b, Chains{Dropped: []Link{{ToolUseID: raw}}}, true, false)
		},
		"rewritten id":   func(b *bytes.Buffer) { writeRewritten(b, []Rewritten{{ToolUseID: raw, ToolName: "Edit"}}) },
		"rewritten tool": func(b *bytes.Buffer) { writeRewritten(b, []Rewritten{{ToolUseID: "toolu_1", ToolName: raw}}) },
		"rewritten program": func(b *bytes.Buffer) {
			writeRewritten(b, []Rewritten{{ToolUseID: "toolu_1", ToolName: "Bash", Program: raw}})
		},
		"timeline program": func(b *bytes.Buffer) {
			seq := int64(1)
			writeTimelineCall(b, TimelineCall{Seq: &seq, ToolName: "Bash", Program: raw, Group: GroupOK, Outcome: store.ExecOK})
		},
	} {
		var b bytes.Buffer
		write(&b)
		if strings.ContainsAny(b.String(), "\x1b\u202e") {
			t.Errorf("%s: a control character reaches the text:\n%q", name, b.String())
		}
		if !strings.Contains(b.String(), clean) {
			t.Errorf("%s: the name is not shown, made printable:\n%q", name, b.String())
		}
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
		"→ no later success of the same command recorded",
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

// A date line is printed again where the date changes. And the header's
// conditional lines stay out of a run with nothing to say them about: no
// failed call, no interrupted call, no subagent. Break: print them always and
// a caveat sits under "failed 0".
func TestTimeline_TextMarksEachNewDate(t *testing.T) {
	run := tlRun([]tlCall{{seq: 1, id: "a", tool: "Read"}, {seq: 2, id: "b", tool: "Read"}})
	run.Declarations[1].RecordedAtMS += 86_400_000
	var b bytes.Buffer
	writeTimeline(&b, buildTimeline(run, nil))
	if !strings.Contains(b.String(), "2023-11-14 (UTC)") || !strings.Contains(b.String(), "2023-11-15 (UTC)") {
		t.Errorf("a timeline across midnight must show both dates:\n%s", b.String())
	}
	for _, absent := range []string{"only a later run", "interrupted", "interleave"} {
		if strings.Contains(b.String(), absent) {
			t.Errorf("a run with no failed, interrupted or subagent call prints %q:\n%s", absent, b.String())
		}
	}
}

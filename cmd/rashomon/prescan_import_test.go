package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/replay"
)

// B2c: a session merged from all its files, replayed through the live recorder,
// summarised through verify. Helpers first; the tests (TestREPLAY24_ and on)
// follow.

const subagentFixtureDir = "../../test/fixtures/claude-code/2.1.312-subagents"

type nopCloser struct{ *strings.Reader }

func (nopCloser) Close() error { return nil }

// textSource is a file held in memory.
func textSource(name, text string, main bool) replay.Source {
	return replay.Source{Name: name, Main: main,
		Open: func() (io.ReadSeekCloser, error) { return nopCloser{strings.NewReader(text)}, nil }}
}

func fileSource(path string, main bool) replay.Source {
	return replay.Source{Name: path, Main: main, Open: func() (io.ReadSeekCloser, error) { return os.Open(path) }}
}

// runMerged replays a session's files into a fresh world.
func runMerged(t *testing.T, sid string, srcs []replay.Source, opt replay.MergeOptions) (replayed, *replayResult, replay.MergeSummary) {
	t.Helper()
	w := newWorld(t)
	res := &replayResult{}
	var ms replay.MergeSummary
	err := w.replayEvents(res, replayConfig{KeepOutput: true}, func(y func(replay.Event) error) error {
		var err error
		ms, err = replay.WalkSession(sid, srcs, opt, y)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	out := replayed{root: w.root, world: w, outputs: res.Outputs}
	for _, s := range res.Stops {
		if s.Evaluated {
			out.verdicts = append(out.verdicts, s.Verdict)
		}
	}
	return out, res, ms
}

func subagentFixtureSources(t *testing.T) (sid string, srcs []replay.Source) {
	t.Helper()
	srcs = append(srcs, fileSource(subagentFixtureDir+"/transcript.jsonl", true))
	subs, err := filepath.Glob(subagentFixtureDir + "/*/subagents/agent-*.jsonl")
	if err != nil || len(subs) != 2 {
		t.Fatalf("subagent files %v: %v", subs, err)
	}
	for _, s := range subs {
		srcs = append(srcs, fileSource(s, false))
	}
	return "00000000-0000-4000-8000-000000000001", srcs
}

// TestREPLAY25_TheSubagentFixtureReplaysAsLive replays the 2.1.312 session
// with two subagents of different types twice, through the live entry points
// into two worlds with one clock: run A feeds the payloads Claude Code sent,
// run B the merged timeline of the parent's transcript and the two subagent
// files. The recovered agent_id and agent_type are in the records exactly as
// live, so there are zero differences, in order, field by field, outputs
// included, by the normalisation of TestREPLAY10_ and nothing else.
// SubagentStart and SubagentStop are in the captured payloads and in no run:
// Rashomon installs neither (install.Events), and no entry point reads them.
func TestREPLAY25_TheSubagentFixtureReplaysAsLive(t *testing.T) {
	sid, srcs := subagentFixtureSources(t)
	captured := readCaptured(t, "2.1.312-subagents")
	var path string
	for _, s := range captured {
		path = capturedPath(s, path)
	}
	opt := replay.MergeOptions{Options: replay.Options{TranscriptPath: path}}
	tl, err := replay.BuildSession(sid, srcs, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !tl.TaintFrom.IsZero() || tl.Gaps != 0 || tl.OrderGaps != 0 {
		t.Fatalf("the merged fixture is tainted from %+v (%s)", tl.TaintFrom, tl.TaintReason)
	}
	var wantLinks = map[string]string{"a0000000000000001": "Explore", "a0000000000000002": "general-purpose"}
	for _, l := range tl.Agents {
		if l.Status != replay.LinkOK || wantLinks[l.AgentID] != l.Type {
			t.Errorf("link %+v", l)
		}
		delete(wantLinks, l.AgentID)
	}
	if len(wantLinks) != 0 {
		t.Errorf("links not made: %v", wantLinks)
	}

	// Run A: the captured payloads the recorder is installed for.
	var hooks []step
	var start step
	for _, s := range captured {
		switch s.event {
		case install.EventSessionStart:
			start = s
		case install.EventSessionEnd, "SubagentStart", "SubagentStop":
			if s.event != install.EventSessionEnd && contains(install.Events(), s.event) {
				t.Fatalf("%s is an installed event", s.event)
			}
		default:
			hooks = append(hooks, s)
		}
	}
	if len(hooks) != len(tl.Events) {
		t.Fatalf("%d captured hook payloads and %d merged events", len(hooks), len(tl.Events))
	}
	steps := []replayStep{{start.event, start.raw, tl.Events[0].At}}
	ids := []string{""}
	for i, s := range hooks {
		e := tl.Events[i]
		if s.event != e.Kind.String() || capturedTool(s) != e.ToolUseID {
			t.Fatalf("captured step %d is %s %q and the merged event is %s %q", i, s.event, capturedTool(s), e.Kind, e.ToolUseID)
		}
		steps = append(steps, replayStep{s.event, s.raw, e.At})
		ids = append(ids, e.ToolUseID)
	}
	a := runLive(t, steps, ids)
	b, res, _ := runMerged(t, sid, srcs, opt)
	if len(res.Skipped) != 0 {
		t.Fatalf("events not fed: %+v", res.Skipped)
	}
	ra, rb := dumpStore(t, a.root), dumpStore(t, b.root)
	diffs := storeDiffs(ra, rb, 20)
	if len(diffs) > 0 {
		t.Errorf("the merged subagent session records differently than the captured payloads (%d records in A, %d in B):\n  %s",
			len(ra), len(rb), strings.Join(diffs, "\n  "))
	}
	if len(a.outputs) != len(b.outputs) {
		t.Fatalf("%d outputs in A, %d in B", len(a.outputs), len(b.outputs))
	}
	for i := range a.outputs {
		x, y := a.outputs[i], b.outputs[i]
		if x.Event != y.Event || x.ToolUseID != y.ToolUseID || x.Exit != y.Exit || x.Stderr != y.Stderr || x.Stdout != y.Stdout {
			t.Errorf("output %d (%s %s): A %+v, B %+v", i, x.Event, x.ToolUseID, x, y)
		}
	}
	// The pair is in the records, and not vacuously: the declarations of the
	// two subagent calls carry the id and the type, those of the main agent
	// carry neither.
	got := map[string][2]any{}
	for _, d := range recordsOf(rb, "records.ndjson", "declaration") {
		got[d["tool_use_id"].(string)] = [2]any{d["agent_id"], d["agent_type"]}
	}
	want := map[string][2]any{
		"toolu_fixture01": {nil, nil}, "toolu_fixture02": {nil, nil}, "toolu_fixture04": {nil, nil},
		"toolu_fixture03": {"a0000000000000001", "Explore"}, "toolu_fixture05": {"a0000000000000002", "general-purpose"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("declarations' agent_id and agent_type:\n got %v\nwant %v", got, want)
	}
	if kindCount(ra, "declaration") != 5 || kindCount(ra, "execution") != 5 {
		t.Errorf("run A recorded %d declarations and %d executions: the comparison is vacuous", kindCount(ra, "declaration"), kindCount(ra, "execution"))
	}
	t.Logf("2.1.312-subagents: %d records in each store, %d outputs, 0 differences after the two-item normalisation", len(ra), len(a.outputs))
}

func capturedPath(s step, prev string) string {
	if prev != "" {
		return prev
	}
	return strings.TrimSpace(strings.Split(strings.Split(string(s.raw), `"transcript_path":"`)[1], `"`)[0])
}

// agentTree builds a parent transcript with one Agent call, answered by a line
// naming agentID (none for ""), and the subagent's transcript, whose one call
// is a test run; times are the parent's lines' plus offsets.
func agentTree(agentID, subType string) (parent *tdoc) { return agentTreeFor("", agentID, subType) }

// agentTreeFor is agentTree for session sid.
func agentTreeFor(sid, agentID, subType string) (parent *tdoc) {
	d := newTdoc()
	d.sid = sid
	d.promptLine("p1")
	d.uses("m1", tUse{"ag1", "Agent", map[string]any{"description": "d", "prompt": "x", "subagent_type": subType}})
	tur := map[string]any{"status": "completed"}
	if agentID != "" {
		tur["agentId"], tur["agentType"] = agentID, subType
	}
	d.results("", tResult{id: "ag1", text: "done", tur: tur})
	d.uses("m2", tBash("t1", "python3 -m unittest"))
	d.results("", tOK("t1"))
	d.ends("m3", "done")
	return d
}

func tOffset(line int, d time.Duration) string {
	return tAt(line).Add(d).Format("2006-01-02T15:04:05.000Z")
}

// subChild is the subagent's transcript: one test run inside the parent's
// Agent call (lines 2 to 3), by agent agentID, its lines attributed to attr.
func subChild(agentID, attr string) *tdoc { return subChildFor("", agentID, attr) }

func subChildFor(sid, agentID, attr string) *tdoc {
	c := newTdoc()
	c.sid = sid
	c.prompt = "p1"
	side := func(m map[string]any, at time.Duration) map[string]any {
		m["isSidechain"], m["agentId"], m["timestamp"] = true, agentID, tOffset(2, at)
		return m
	}
	use := side(map[string]any{}, 300*time.Millisecond)
	if attr != "" {
		use["attributionAgent"] = attr
	}
	c.usesWith("", use, "cm1", tBash("c1", "python3 -m unittest"))
	// A subagent's result line carries no toolUseResult.
	c.resultsWith("", side(map[string]any{}, 600*time.Millisecond), tResult{id: "c1", text: "fine"})
	return c
}

// TestREPLAY25b_ALinkThatIsNotUnambiguousKeepsTheSubagentUnsupported: each
// way the parent link can fail, through the recorder. The subagent's call is
// not fed, its declaration is not in the store, the session's Stop is
// evaluated and not Verified, naming the call that tainted it, and no run of
// the subagent counts. The control, the same tree with the link whole, feeds
// the call with the id and the type.
func TestREPLAY25b_ALinkThatIsNotUnambiguousKeepsTheSubagentUnsupported(t *testing.T) {
	two := func() *tdoc {
		d := agentTree("A1", "Explore")
		return d
	}
	unreadable := func() *tdoc {
		d := agentTree("A1", "Explore")
		d.lines[2] = d.lines[2][:len(d.lines[2])/2]
		return d
	}
	cases := []struct {
		name   string
		parent *tdoc
		child  *tdoc
		status replay.LinkStatus
	}{
		{"missing link", agentTree("", "Explore"), subChild("A1", "Explore"), replay.LinkNoParent},
		{"another agent named", agentTree("A9", "Explore"), subChild("A1", "Explore"), replay.LinkNoParent},
		{"two parents claim one agent", func() *tdoc {
			d := two()
			// the Stop line moves down: a second Agent call claims A1 too.
			d.lines = d.lines[:5]
			d.uses("m4", tUse{"ag2", "Agent", map[string]any{"subagent_type": "Plan"}})
			d.results("", tResult{id: "ag2", text: "done", tur: map[string]any{"agentId": "A1"}})
			d.ends("m5", "done")
			return d
		}(), subChild("A1", "Explore"), replay.LinkAmbiguous},
		{"the parent's result is in an unreadable line", unreadable(), subChild("A1", "Explore"), replay.LinkNoParent},
		{"attributionAgent disagrees with the link", agentTree("A1", "Explore"), subChild("A1", "Plan"), replay.LinkAttributionMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srcs := []replay.Source{textSource("parent", c.parent.String(), true), textSource("child", c.child.String(), false)}
			rep, res, ms := runMerged(t, tSession, srcs, replay.MergeOptions{Options: replay.Options{TranscriptPath: tPath}})
			if len(ms.Agents) != 1 || ms.Agents[0].Status != c.status {
				t.Fatalf("links %+v, want %s", ms.Agents, c.status)
			}
			for _, d := range recordsOf(dumpStore(t, rep.root), "records.ndjson", "declaration") {
				if d["tool_use_id"] == "c1" {
					t.Errorf("the subagent's call was recorded: %v", d)
				}
			}
			skipped := 0
			for _, s := range res.Skipped {
				if s.ToolUseID == "c1" {
					skipped++
					if s.Reasons[0] != replay.SidechainAgentType {
						t.Errorf("skipped for %v, want sidechain_agent_type", s.Reasons)
					}
				}
			}
			if skipped != 2 {
				t.Errorf("%d events of the subagent's call skipped, want its declaration and its execution", skipped)
			}
			if ms.TaintFrom.IsZero() {
				t.Fatal("the unsupported call taints nothing")
			}
			if len(res.Stops) != 1 || !res.Stops[0].Evaluated || res.Stops[0].Verified || res.Stops[0].Downgrade == "" {
				t.Errorf("the Stop is %+v: want evaluated, not Verified, with the reason", res.Stops)
			}
		})
	}
	t.Run("control: the link whole", func(t *testing.T) {
		srcs := []replay.Source{textSource("parent", agentTree("A1", "Explore").String(), true), textSource("child", subChild("A1", "Explore").String(), false)}
		rep, res, ms := runMerged(t, tSession, srcs, replay.MergeOptions{Options: replay.Options{TranscriptPath: tPath}})
		if len(res.Skipped) != 0 || !ms.TaintFrom.IsZero() || len(res.Stops) != 1 || !res.Stops[0].Verified {
			t.Fatalf("skipped %+v taint %+v stops %+v", res.Skipped, ms.TaintFrom, res.Stops)
		}
		for _, d := range recordsOf(dumpStore(t, rep.root), "records.ndjson", "declaration") {
			if d["tool_use_id"] == "c1" && (d["agent_id"] != "A1" || d["agent_type"] != "Explore") {
				t.Errorf("the declaration carries %v / %v", d["agent_id"], d["agent_type"])
			}
		}
	})
}

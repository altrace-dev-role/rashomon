package report

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Item 8 of the outside review (report.md :225 to :231): each session's
// report starts with four lines, the last Stop's strict claims not green
// (the spec's flagged claims, whose decision no record keeps), notes by
// pattern, when checks were off, and how far to trust the record; the
// subagent list is capped; the tool-id lists print under --ids.

// rh8Render is sess rendered alone through Text with opts.
func rh8Render(t *testing.T, sess Session, opts ...TextOption) string {
	t.Helper()
	var b bytes.Buffer
	if err := Text(&b, &Report{Sessions: []Session{sess}}, opts...); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// rh8Story is the lines after "  story" up to the next line of two-space
// indent: the session's first screen.
func rh8Story(t *testing.T, out string) []string {
	t.Helper()
	_, after, ok := strings.Cut(out, "\n  story\n")
	if !ok {
		t.Fatalf("no story in:\n%s", out)
	}
	var lines []string
	for _, l := range strings.Split(after, "\n") {
		if !strings.HasPrefix(l, "    ") {
			break
		}
		lines = append(lines, l)
	}
	return lines
}

// rh8Line is the one story line that starts with prefix; it fails on none
// or more than one.
func rh8Line(t *testing.T, story []string, prefix string) string {
	t.Helper()
	var got []string
	for _, l := range story {
		if strings.HasPrefix(l, prefix) {
			got = append(got, l)
		}
	}
	if len(got) != 1 {
		t.Fatalf("%d story lines start %q, want 1:\n%s", len(got), prefix, strings.Join(story, "\n"))
	}
	return got[0]
}

// rh8Autosteer is an Autosteer with a Stop read and every list empty.
func rh8Autosteer() *Autosteer {
	return &Autosteer{Stop: &AutosteerStop{Seq: 40, Row: "verified"}, Gates: []AutosteerGate{},
		Delivered: []AutosteerNote{}, Findings: []AutosteerFinding{}, Checks: []AutosteerCheck{},
		Unknown: []AutosteerUnknown{}, NotGreen: []AutosteerClaim{}}
}

// TestRH8_LeadLinesFirst (item 8, report.md :225): the story opens with the
// four lead lines in their order, before the last Stop's line, and the
// strict-claims and checks-off lines say what the record holds in their
// own words: a claim by class and verdict with its evidence seqs, "no gate"
// where its gate has no row, which window judged it, and "last Stop only";
// the gaps and an unrecorded start or end. Break: print the lead lines after
// the last Stop line, or drop any part of a claim.
func TestRH8_LeadLinesFirst(t *testing.T) {
	sess := Session{SessionID: "s1", Coverage: Coverage{State: "complete", StartRecorded: true, EndRecorded: true},
		Autosteer: rh8Autosteer()}
	story := rh8Story(t, rh8Render(t, sess))
	if len(story) < 5 {
		t.Fatalf("story has %d lines, want the four lead lines and the last Stop's:\n%s", len(story), strings.Join(story, "\n"))
	}
	for i, prefix := range []string{"    strict claims not green", "    notes delivered by pattern: ", "    checks off: ", "    trust: ", "    last Stop: "} {
		if !strings.HasPrefix(story[i], prefix) {
			t.Errorf("story line %d is %q, want it to start %q", i+1, story[i], prefix)
		}
	}

	stop := func(f ...AutosteerClaim) *Autosteer { a := rh8Autosteer(); a.NotGreen = f; return a }
	for _, c := range []struct {
		name string
		a    *Autosteer
		want string
	}{
		{"not read", nil, "    strict claims not green: not read"},
		{"no Stop", &Autosteer{}, "    strict claims not green: no Stop recorded"},
		{"none listed", stop(), "    strict claims not green, last Stop only: none"},
		{"one with its seqs, own turn", stop(AutosteerClaim{Class: "tests_pass", Verdict: "stale", Evidence: []int64{3, 5}}),
			"    strict claims not green, last Stop only: tests_pass stale (evidence #3, #5; done flag read the Stop's turn)"},
		{"no gate row, earlier window", stop(AutosteerClaim{Class: "done", Verdict: "red", NoGate: 1, EarlierWindow: true}),
			"    strict claims not green, last Stop only: done red (no evidence seq; no gate for 1; done flag read the turns since the last edit)"},
		{"verdict words", stop(AutosteerClaim{Class: "build_ok", Verdict: "masked_inconclusive", Evidence: []int64{9}, NoGate: 2}),
			"    strict claims not green, last Stop only: build_ok masked inconclusive (evidence #9; no gate for 2; done flag read the Stop's turn)"},
		{"capped at four", stop(
			AutosteerClaim{Class: "a", Verdict: "red", Evidence: []int64{1}}, AutosteerClaim{Class: "b", Verdict: "red", Evidence: []int64{2}},
			AutosteerClaim{Class: "c", Verdict: "red", Evidence: []int64{3}}, AutosteerClaim{Class: "d", Verdict: "red", Evidence: []int64{4}},
			AutosteerClaim{Class: "e", Verdict: "red", Evidence: []int64{5}}),
			"    strict claims not green, last Stop only: a red (evidence #1; done flag read the Stop's turn); b red (evidence #2; done flag read the Stop's turn); " +
				"c red (evidence #3; done flag read the Stop's turn); d red (evidence #4; done flag read the Stop's turn); and 1 more"},
	} {
		s := sess
		s.Autosteer = c.a
		if got := rh8Line(t, rh8Story(t, rh8Render(t, s)), "    strict claims not green"); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
	omitted := stop()
	omitted.ClaimsOmitted = 3
	s := sess
	s.Autosteer = omitted
	if got, want := rh8Line(t, rh8Story(t, rh8Render(t, s)), "    strict claims not green"),
		"    strict claims not green, last Stop only: none; 3 more not kept"; got != want {
		t.Errorf("claims the checkpoint did not keep:\n got %q\nwant %q", got, want)
	}

	from, to := int64(1_800_000_000_000), int64(1_800_000_060_000)
	for _, c := range []struct {
		name string
		cov  Coverage
		gaps []store.Gap
		want string
	}{
		{"none", Coverage{State: "complete", StartRecorded: true, EndRecorded: true}, nil,
			"    checks off: none in the record; steering off: not read"},
		{"start and end unrecorded", Coverage{State: "partial"}, nil,
			"    checks off: before the first record (its start was not recorded); after the last record (its end was not recorded); steering off: not read"},
		{"paused and evicted", Coverage{State: "partial", StartRecorded: true, EndRecorded: true},
			[]store.Gap{{Reason: store.GapPaused, FromUnixMS: from, ToUnixMS: to}, {Reason: "evicted", RemovedRecords: 4, FromUnixMS: from, ToUnixMS: to}},
			fmt.Sprintf("    checks off: paused %s to %s; evicted, records removed %s to %s; steering off: not read", stamp(from), stamp(to), stamp(from), stamp(to))},
	} {
		s := sess
		s.Coverage, s.Gaps = c.cov, c.gaps
		if got := rh8Line(t, rh8Story(t, rh8Render(t, s)), "    checks off: "); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

// TestRH8b_ActedIgnoredNotMeasured (item 8, conditions 379.2d): the notes
// line counts the notes delivered by pattern, and acted on and ignored read
// "not measured", never a number: no stored label says which note was acted
// on before item 17. Break: print a count for either.
func TestRH8b_ActedIgnoredNotMeasured(t *testing.T) {
	two := rh8Autosteer()
	two.Delivered = []AutosteerNote{{Title: "task check", Seq: 3}, {Title: "known fix", Seq: 5}, {Title: "task check", Seq: 9}}
	for _, c := range []struct {
		name string
		a    *Autosteer
		want string
	}{
		{"not read", nil, "    notes delivered by pattern: not read; acted on: not measured; ignored: not measured"},
		{"no Stop", &Autosteer{}, "    notes delivered by pattern: no Stop recorded; acted on: not measured; ignored: not measured"},
		{"none", rh8Autosteer(), "    notes delivered by pattern: none; acted on: not measured; ignored: not measured"},
		{"two patterns", two, "    notes delivered by pattern: 2 task checks, 1 known fix; acted on: not measured; ignored: not measured"},
	} {
		sess := Session{SessionID: "s1", Autosteer: c.a}
		if got := rh8Line(t, rh8Story(t, rh8Render(t, sess)), "    notes delivered by pattern: "); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

// TestRH8c_TrustLineOnlyPinnedClauses (item 8, note2.trust): the trust line
// is the coverage state, the gaps, the skipped records and where the claims
// were read, each a clause this test pins, and nothing else. Break: drop or
// add a clause.
func TestRH8c_TrustLineOnlyPinnedClauses(t *testing.T) {
	gap := store.Gap{Reason: store.GapPaused}
	for _, c := range []struct {
		name    string
		cov     Coverage
		gaps    []store.Gap
		skipped int
		a       *Autosteer
		want    string
	}{
		{"complete", Coverage{State: "complete", StartRecorded: true, EndRecorded: true}, nil, 0, rh8Autosteer(),
			"    trust: coverage complete; no gaps; no skipped records; claims read at the last Stop only"},
		{"partial, one each", Coverage{State: "partial"}, []store.Gap{gap}, 1, rh8Autosteer(),
			"    trust: coverage partial; 1 gap; 1 skipped record; claims read at the last Stop only"},
		{"counts, not read", Coverage{State: "partial"}, []store.Gap{gap, gap}, 3, nil,
			"    trust: coverage partial; 2 gaps; 3 skipped records; claims not read"},
		{"no Stop", Coverage{}, nil, 0, &Autosteer{},
			"    trust: coverage unknown; no gaps; no skipped records; claims not read"},
	} {
		sess := Session{SessionID: "s1", Coverage: c.cov, Gaps: c.gaps, SkippedRecords: c.skipped, Autosteer: c.a}
		if got := rh8Line(t, rh8Story(t, rh8Render(t, sess)), "    trust: "); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

// TestRH8e_SubagentListCapped (item 8, report.md :231): the subagent list
// names six and then says how many more and that --json lists them all;
// six or fewer print whole. Break: drop the cap, or the more line.
func TestRH8e_SubagentListCapped(t *testing.T) {
	subs := func(n int) []SubagentSummary {
		var out []SubagentSummary
		for i := range n {
			out = append(out, SubagentSummary{AgentID: fmt.Sprintf("agent-%02d", i+1), AgentType: "general"})
		}
		return out
	}
	row := func(i int) string {
		return fmt.Sprintf("    agent-%02d (general): 0 declarations, 0 executions, 0 Bash\n", i)
	}
	tail := "    (these calls do not appear in the main transcript)\n"

	out := rh8Render(t, Session{SessionID: "s1", Subagents: subs(9)})
	want := "  subagents: 9\n" + row(1) + row(2) + row(3) + row(4) + row(5) + row(6) + "    and 3 more; --json lists them all\n" + tail
	if !strings.Contains(out, want) {
		t.Errorf("nine subagents: want the block\n%s\nin:\n%s", want, out)
	}
	for i := 7; i <= 9; i++ {
		if strings.Contains(out, fmt.Sprintf("agent-%02d", i)) {
			t.Errorf("agent-%02d listed past the cap", i)
		}
	}

	out = rh8Render(t, Session{SessionID: "s1", Subagents: subs(6)})
	want = "  subagents: 6\n" + row(1) + row(2) + row(3) + row(4) + row(5) + row(6) + tail
	if !strings.Contains(out, want) || strings.Contains(out, "more; --json") {
		t.Errorf("six subagents print whole with no more line:\n%s", out)
	}
}

// TestRH8f_ToolIdsBehindIdsFlag (item 8, report.md :231): without --ids the
// calls declared without an execution and the calls run differently from
// how they were declared say their counts and that --ids lists them, and
// print no tool-use id; with --ids both lists print as before. The chain
// view lists its ids under --chain alone. Break: list the ids without the
// flag, or hide them with it.
func TestRH8f_ToolIdsBehindIdsFlag(t *testing.T) {
	sess := Session{SessionID: "s1",
		Declarations: Declarations{WithoutExecution: []Unexecuted{{ToolUseID: "toolu_x1", PermissionMode: "default"}, {ToolUseID: "toolu_x2", PermissionMode: "plan"}}},
		Destinations: Destinations{ExecutedNotAsDeclared: 1, Rewritten: []Rewritten{{ToolUseID: "toolu_r1", ToolName: "Bash", Program: "git", VerbClass: "write"}}}}
	var rows bytes.Buffer
	writeRewritten(&rows, sess.Destinations.Rewritten)

	plain := rh8Render(t, sess)
	for _, id := range []string{"toolu_x1", "toolu_x2", "toolu_r1"} {
		if strings.Contains(plain, id) {
			t.Errorf("without --ids the text lists %s:\n%s", id, plain)
		}
	}
	for _, want := range []string{"  without execution: 2; ids with --ids\n", "  executed differently from declared: 1\n    ids with --ids\n"} {
		if !strings.Contains(plain, want) {
			t.Errorf("without --ids, want %q in:\n%s", want, plain)
		}
	}

	ids := rh8Render(t, sess, WithIDs())
	for _, want := range []string{"  without execution: toolu_x1 (default), toolu_x2 (plan)\n", "  executed differently from declared: 1\n" + rows.String()} {
		if !strings.Contains(ids, want) {
			t.Errorf("with --ids, want %q in:\n%s", want, ids)
		}
	}
	if strings.Contains(ids, "ids with --ids") {
		t.Errorf("with --ids the text still says to pass it:\n%s", ids)
	}

	// The chain view names a call by its seq; the one id it prints is a call
	// that ran with no declaration recorded, where the id is all it knows.
	chained := Session{SessionID: "s1", Chains: Chains{Prompts: []Chain{{PromptID: "p1",
		Links: []Link{{Seq: 1, ToolUseID: "toolu_c0", ToolName: "Bash", VerbClass: "read"}}}},
		Dropped: []Link{{ToolUseID: "toolu_c1"}}}}
	alone, both := rh8Render(t, chained, WithChain()), rh8Render(t, chained, WithChain(), WithIDs())
	if !strings.Contains(alone, "toolu_c1") || alone != both {
		t.Errorf("the chain view lists its ids under --chain alone, and --ids moves nothing in it:\n--chain:\n%s\n--chain --ids:\n%s", alone, both)
	}
	if !strings.Contains(alone, "\n      1  Bash (read)") || strings.Contains(alone, "toolu_c0") {
		t.Errorf("the chain view names the declared call by its seq and never by its id toolu_c0:\n%s", alone)
	}
}

package history

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// line builds one transcript line from a header and content blocks. The
// version is the supported fixture version unless the test says otherwise.
type line struct {
	session, cwd, version string
	sidechain             bool
	agentID               string
	blocks                []map[string]any
	toolUseResult         any
}

func (l line) String() string {
	m := map[string]any{"type": "assistant", "sessionId": l.session, "cwd": l.cwd, "version": l.version,
		"timestamp": "2026-10-29T10:00:00Z", "isSidechain": l.sidechain,
		"message": map[string]any{"content": l.blocks}}
	if l.agentID != "" {
		m["agentId"] = l.agentID
	}
	if l.toolUseResult != nil {
		m["toolUseResult"] = l.toolUseResult
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func use(id, name string, input map[string]any) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	return map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}
}

func result(id string, isError bool, text string) map[string]any {
	return map[string]any{"type": "tool_result", "tool_use_id": id, "is_error": isError, "content": text}
}

func at(blocks ...map[string]any) line {
	return line{session: "s1", cwd: "/repo", version: "2.1.280", blocks: blocks}
}

func readAll(t *testing.T, lim Limits, lines ...string) ([]Call, Stats) {
	t.Helper()
	var calls []Call
	st, err := Read(strings.NewReader(strings.Join(lines, "\n")+"\n"), lim, func(c Call) {
		c.OutputHead = append([]byte(nil), c.OutputHead...)
		c.OutputTail = append([]byte(nil), c.OutputTail...)
		calls = append(calls, c)
	})
	if err != nil {
		t.Fatal(err)
	}
	return calls, st
}

func bash(cmd string) map[string]any { return map[string]any{"command": cmd} }

// TestHIST1_RealTranscript reads the 2.1.280 fixture, a real Claude Code
// transcript: eleven calls, in order, each Verified, one failed by its exit
// code and one refused by a settings rule.
func TestHIST1_RealTranscript(t *testing.T) {
	f, err := os.Open("../../test/fixtures/claude-code/2.1.280/transcript.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var calls []Call
	st, err := Read(f, DefaultLimits, func(c Call) { calls = append(calls, c) })
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		tool    string
		outcome Outcome
	}{
		{"Write", OK}, {"Read", OK}, {"Edit", OK}, {"Bash", OK}, {"Bash", Failed}, {"Bash", Denied},
		{"Grep", OK}, {"Glob", OK}, {"ToolSearch", OK}, {"WebFetch", OK}, {"Bash", OK},
	}
	if len(calls) != len(want) {
		t.Fatalf("read %d calls, want %d", len(calls), len(want))
	}
	for i, c := range calls {
		if c.Seq != i || c.ToolName != want[i].tool || c.Outcome != want[i].outcome || c.Grade != Verified {
			t.Errorf("call %d = seq %d %s %s %s, want %s %s verified", i, c.Seq, c.ToolName, c.Outcome, c.Grade,
				want[i].tool, want[i].outcome)
		}
		if c.SessionID != "00000000-0000-4000-8000-000000000001" || c.CWD == "" || c.Version != "2.1.280" {
			t.Errorf("call %d header = %q %q %q", i, c.SessionID, c.CWD, c.Version)
		}
	}
	if c := calls[4]; c.ExitCode == nil || *c.ExitCode != 1 || !strings.HasPrefix(string(c.OutputHead), "Exit code 1\n") {
		t.Errorf("the failed cat = exit %v, head %q", c.ExitCode, c.OutputHead)
	}
	if st.Uses != 11 || st.Results != 11 || st.Grades[Verified] != 11 || st.Undecodable != 0 {
		t.Errorf("stats = %+v", st)
	}
}

// TestHIST2_Outcomes: an error result's outcome is read from an anchored
// "Exit code N"; 124, 126 and 127 are classes of their own, never a plain
// failure; a denial is never a failure; and a Bash error naming no code is
// Partial, since a replay cannot tell its exit status.
func TestHIST2_Outcomes(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		isError bool
		text    string
		outcome Outcome
		code    int // -1: none
		grade   Grade
		reason  Reason
	}{
		{"ok", "Bash", false, "fine", OK, -1, Verified, ReasonNone},
		{"exit 1", "Bash", true, "Exit code 1\nboom", Failed, 1, Verified, ReasonNone},
		{"exit with colon", "Bash", true, "Exit code 2: make", Failed, 2, Verified, ReasonNone},
		{"exit run into text", "Bash", true, "Exit code 2make: *** Stop.", Failed, 2, Verified, ReasonNone},
		{"timeout", "Bash", true, "Exit code 124", Timeout, 124, Verified, ReasonNone},
		{"126", "Bash", true, "Exit code 126\npermission denied", NotRun, 126, Verified, ReasonNone},
		{"127", "Bash", true, "Exit code 127\nnot found", NotRun, 127, Verified, ReasonNone},
		{"negative", "Bash", true, "Exit code -1", Failed, -1 << 31, Verified, ReasonNone},
		{"not anchored", "Bash", true, "warning\nExit code 1", ToolError, -1, Partial, NoExitCode},
		{"too many digits", "Bash", true, "Exit code 1234567890", ToolError, -1, Partial, NoExitCode},
		{"no digits", "Bash", true, "Exit code x", ToolError, -1, Partial, NoExitCode},
		{"edit error", "Edit", true, "<tool_use_error>String to replace not found", ToolError, -1, Verified, ReasonNone},
		{"denied", "Bash", true, "The user doesn't want to proceed with this tool use. The tool use was rejected", Denied, -1, Verified, ReasonNone},
		{"settings deny", "Bash", true, "Permission to use Bash with command rm has been denied.", Denied, -1, Verified, ReasonNone},
		{"denial text, not an error", "Bash", false, "The user doesn't want to proceed with this tool use. The tool use was rejected", OK, -1, Verified, ReasonNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls, _ := readAll(t, DefaultLimits,
				at(use("a", tc.tool, bash("x"))).String(),
				at(result("a", tc.isError, tc.text)).String())
			if len(calls) != 1 {
				t.Fatalf("%d calls", len(calls))
			}
			c := calls[0]
			if c.Outcome != tc.outcome || c.Grade != tc.grade || c.Reason != tc.reason {
				t.Errorf("= %s %s %s, want %s %s %s", c.Outcome, c.Grade, c.Reason, tc.outcome, tc.grade, tc.reason)
			}
			switch {
			case tc.code == -1 && c.ExitCode != nil:
				t.Errorf("exit code %d, want none", *c.ExitCode)
			case tc.code == -1<<31 && (c.ExitCode == nil || *c.ExitCode != -1):
				t.Errorf("exit code %v, want -1", c.ExitCode)
			case tc.code >= 0 && (c.ExitCode == nil || *c.ExitCode != tc.code):
				t.Errorf("exit code %v, want %d", c.ExitCode, tc.code)
			}
		})
	}
}

// TestHIST3_Order: calls are emitted in tool_use order whatever order their
// results arrive in, and a call with no result is Missing, never ok.
func TestHIST3_Order(t *testing.T) {
	calls, st := readAll(t, DefaultLimits,
		at(use("a", "Bash", bash("1")), use("b", "Bash", bash("2")), use("c", "Bash", bash("3"))).String(),
		at(result("c", false, "3"), result("a", false, "1")).String(),
	)
	got := []string{}
	for _, c := range calls {
		got = append(got, c.ToolUseID+":"+c.Outcome.String()+":"+c.Grade.String()+":"+c.Reason.String())
	}
	want := []string{"a:ok:verified:", "b:none:missing:no_result", "c:ok:verified:"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if st.Grades[Missing] != 1 || st.Outcomes[OutcomeNone] != 1 {
		t.Errorf("stats = %+v", st)
	}
}

// TestHIST4_Grades: what makes a call less than Verified.
func TestHIST4_Grades(t *testing.T) {
	noVersion := at(use("a", "Bash", bash("x")))
	noVersion.version = ""
	unknown := at(use("a", "Bash", bash("x")))
	unknown.version = "9.9.9"
	noCWD := at(use("a", "Bash", bash("x")))
	noCWD.cwd = ""
	ok := at(result("a", false, "fine")).String()
	cases := []struct {
		name   string
		lines  []string
		grade  Grade
		reason Reason
	}{
		{"no version", []string{noVersion.String(), ok}, Unsupported, NoVersion},
		{"unsupported version", []string{unknown.String(), ok}, Unsupported, UnsupportedVersion},
		{"no cwd", []string{noCWD.String(), ok}, Partial, NoCWD},
		{"unreadable line between", []string{at(use("a", "Bash", bash("x"))).String(), "{not json", ok}, Partial, LineSkipped},
		{"unreadable line before is harmless", []string{"{not json", at(use("a", "Bash", bash("x"))).String(), ok}, Verified, ReasonNone},
		{"conflicting results", []string{at(use("a", "Bash", bash("x"))).String(),
			at(result("a", false, "fine"), result("a", true, "Exit code 1")).String()}, Unsupported, ResultConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls, _ := readAll(t, DefaultLimits, tc.lines...)
			if len(calls) != 1 || calls[0].Grade != tc.grade || calls[0].Reason != tc.reason {
				t.Fatalf("calls = %+v, want one %s %s", calls, tc.grade, tc.reason)
			}
		})
	}
}

// TestHIST5_Duplicates: one id is one call. A repeated tool_use is counted
// and dropped, an agreeing repeated result is counted, and a result for no
// call read here is an orphan.
func TestHIST5_Duplicates(t *testing.T) {
	calls, st := readAll(t, DefaultLimits,
		at(use("a", "Bash", bash("x"))).String(),
		at(use("a", "Bash", bash("x"))).String(),
		at(result("a", false, "fine")).String(),
		at(result("a", false, "fine")).String(),
		at(result("z", false, "fine")).String(),
	)
	if len(calls) != 1 || calls[0].Grade != Verified {
		t.Fatalf("calls = %+v", calls)
	}
	if st.Uses != 2 || st.DuplicateUses != 1 || st.DuplicateResults != 1 || st.OrphanResults != 1 {
		t.Errorf("stats = %+v", st)
	}
}

// TestHIST6_Fields: a sidechain's agent, a background run, an interruption,
// and the session id are carried on the call.
func TestHIST6_Fields(t *testing.T) {
	side := at(use("a", "Bash", map[string]any{"command": "go test ./...", "run_in_background": true}))
	side.sidechain, side.agentID = true, "agent-7"
	main := at(use("b", "Bash", bash("sleep 9")))
	main.agentID = "ignored-off-a-sidechain"
	interrupted := at(result("b", true, "Exit code 130"))
	interrupted.toolUseResult = map[string]any{"interrupted": true}
	calls, _ := readAll(t, DefaultLimits, side.String(), at(result("a", false, "started")).String(), main.String(), interrupted.String())
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
	if c := calls[0]; !c.Sidechain || c.AgentID != "agent-7" || !c.Backgrounded || c.SessionID != "s1" {
		t.Errorf("sidechain call = %+v", c)
	}
	if c := calls[1]; c.Sidechain || c.AgentID != "" || c.Backgrounded || c.Outcome != Interrupted {
		t.Errorf("interrupted call = %+v", c)
	}
}

// TestHIST7_Limits: each bound in Limits degrades a grade rather than growing
// memory, and says which bound it was.
func TestHIST7_Limits(t *testing.T) {
	t.Run("outstanding", func(t *testing.T) {
		lim := DefaultLimits
		lim.MaxOutstanding = 2
		calls, _ := readAll(t, lim,
			at(use("a", "Bash", bash("1"))).String(),
			at(use("b", "Bash", bash("2"))).String(),
			at(use("c", "Bash", bash("3"))).String(),
			at(result("b", false, "2"), result("c", false, "3"), result("a", false, "late")).String(),
		)
		if len(calls) != 3 || calls[0].Grade != Missing || calls[0].Reason != Evicted ||
			calls[1].Grade != Verified || calls[2].Grade != Verified {
			t.Fatalf("calls = %+v", calls)
		}
	})
	t.Run("queued bytes", func(t *testing.T) {
		lim := DefaultLimits
		lim.MaxQueuedBytes = 64
		big := strings.Repeat("y", 80)
		calls, _ := readAll(t, lim,
			at(use("a", "Bash", bash("1"))).String(),
			at(use("b", "Bash", bash(big))).String(),
			at(result("b", false, "2"), result("a", false, "1")).String(),
		)
		if len(calls) != 2 || calls[0].Reason != Evicted || calls[1].Grade != Verified {
			t.Fatalf("calls = %+v", calls)
		}
	})
	t.Run("input size", func(t *testing.T) {
		lim := DefaultLimits
		lim.MaxInputBytes = 16
		calls, _ := readAll(t, lim,
			at(use("a", "Write", map[string]any{"file_path": "/repo/x", "content": strings.Repeat("z", 64)})).String(),
			at(result("a", false, "ok")).String(),
		)
		if len(calls) != 1 || calls[0].ToolInput != nil || calls[0].Grade != Partial || calls[0].Reason != InputTooLarge {
			t.Fatalf("calls = %+v", calls)
		}
	})
	t.Run("calls per file", func(t *testing.T) {
		lim := DefaultLimits
		lim.MaxCallsPerFile = 1
		calls, _ := readAll(t, lim,
			at(use("a", "Bash", bash("1")), use("b", "Bash", bash("2"))).String(),
			at(result("a", false, "1"), result("b", false, "2")).String(),
		)
		if len(calls) != 2 || calls[0].Grade != Verified || calls[1].Grade != Partial ||
			calls[1].Reason != OverCap || calls[1].Outcome != OK {
			t.Fatalf("calls = %+v", calls)
		}
	})
}

// TestHIST8_HeadTail: the head and tail of a result's joined text, built
// without the join, against the join itself, for every bound up to the
// text's length.
func TestHIST8_HeadTail(t *testing.T) {
	for _, parts := range [][]string{nil, {""}, {"abc"}, {"ab", "cd"}, {"", "x", ""}, {"hello", "", "world!"}} {
		joined := strings.Join(parts, "\n")
		for h := 0; h <= len(joined)+1; h++ {
			for tl := 0; tl <= len(joined)+1; tl++ {
				head, tail := headTail(parts, h, tl)
				wantHead := joined[:min(h, len(joined))]
				wantTail := ""
				if len(joined) > h {
					wantTail = joined[len(joined)-min(tl, len(joined)-h):]
				}
				if string(head) != wantHead || string(tail) != wantTail {
					t.Fatalf("headTail(%q, %d, %d) = %q, %q; want %q, %q", parts, h, tl, head, tail, wantHead, wantTail)
				}
			}
		}
	}
}

// TestHIST9_ContentShapes: a result's content as an array of blocks reads as
// the plain string does, and a line whose content is a plain string (a
// prompt) holds no calls.
func TestHIST9_ContentShapes(t *testing.T) {
	arr := at(map[string]any{"type": "tool_result", "tool_use_id": "a", "is_error": true,
		"content": []map[string]any{{"type": "text", "text": "Exit code 3\nno"}, {"type": "text", "text": "more"}}})
	prompt := `{"type":"user","sessionId":"s1","cwd":"/repo","version":"2.1.280","message":{"content":"run the tests"}}`
	calls, _ := readAll(t, DefaultLimits, prompt, at(use("a", "Bash", bash("x"))).String(), arr.String())
	if len(calls) != 1 || calls[0].Outcome != Failed || *calls[0].ExitCode != 3 || string(calls[0].OutputHead) != "Exit code 3\nno\nmore" {
		t.Fatalf("calls = %+v", calls)
	}
}

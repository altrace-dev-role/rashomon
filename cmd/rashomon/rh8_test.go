package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/report/gather"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// rh8Land records one call of session "s" under prompt: its PreToolUse and,
// unless declareOnly, its PostToolUse, as landIn records them.
func (f *lwFixture) rh8Land(prompt, id, tool string, input map[string]any, declareOnly bool) {
	f.t.Helper()
	events := []string{"PreToolUse", "PostToolUse"}
	if declareOnly {
		events = events[:1]
	}
	for _, event := range events {
		b, err := json.Marshal(map[string]any{"session_id": "s", "tool_use_id": id, "prompt_id": prompt,
			"transcript_path": "/tmp/t.jsonl", "permission_mode": "default", "tool_name": tool, "cwd": f.cwd,
			"hook_event_name": event, "tool_input": input, "tool_response": map[string]any{"stdout": ""}})
		if err != nil {
			f.t.Fatal(err)
		}
		clock := func() time.Time { return f.now }
		if event == "PreToolUse" {
			h := hook.New(f.st, clock)
			h.Close(&hook.Signals{}, h.Capture(strings.NewReader(string(b))))
		} else if err := hook.NewPost(f.st, clock).Capture(strings.NewReader(string(b))); err != nil {
			f.t.Fatalf("post %s: %v", id, err)
		}
		f.now = f.now.Add(time.Minute)
	}
}

// rh8StopSaying is session "s"'s Stop with msg as the final message, as
// stopIn runs it; the Stop appends its checkpoint.
func (f *lwFixture) rh8StopSaying(msg string) verify.StopView {
	f.t.Helper()
	run, err := digest.ReadRun(f.st, "s")
	if err != nil {
		f.t.Fatal(err)
	}
	d, err := digest.BuildFrom(run, f.st, "s", "", "", f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	p := recapPayload{SessionID: "s", LastAssistantMessage: msg, Harness: harnessSignal{HookEventName: install.EventStop}, OK: true, CWD: f.cwd}
	turn, err := stopView(f.st, run, d, p, false, f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	return turn.view
}

// TestRH8d_BlockOnOneSessionReport (item 8, report.md :231): a report that
// covers one session attaches its AutoSteer block with no session named; the
// calls without a session id are not a session; a report of two sessions
// attaches none. Through the verb, `rashomon report` over the one session
// prints its last Stop, never "not read". Break: keep the early return on no
// id, or count the unattributed calls as a session.
func TestRH8d_BlockOnOneSessionReport(t *testing.T) {
	f := newLW(t)
	f.bind(lwSpec("unit"))
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	f.stop()

	for _, c := range []struct {
		name string
		ids  []string
		want []bool
	}{
		{"no session", nil, nil},
		{"one session", []string{"s"}, []bool{true}},
		{"one session and the calls without a session id", []string{"s", store.UnattributedSession}, []bool{true, false}},
		{"two sessions", []string{"s", "s2"}, []bool{false, false}},
	} {
		rep := &report.Report{}
		for _, id := range c.ids {
			rep.Sessions = append(rep.Sessions, report.Session{SessionID: id})
		}
		attachImpact(f.st, rep, "")
		for i, s := range rep.Sessions {
			if got := s.Autosteer != nil; got != c.want[i] {
				t.Errorf("%s: session %q attached %v, want %v", c.name, s.SessionID, got, c.want[i])
			}
		}
	}
	rep := &report.Report{Sessions: []report.Session{{SessionID: "s"}, {SessionID: "s2"}}}
	attachImpact(f.st, rep, "s")
	if rep.Sessions[0].Autosteer == nil || rep.Sessions[1].Autosteer != nil {
		t.Errorf("--session s attaches s alone: %v, %v", rep.Sessions[0].Autosteer != nil, rep.Sessions[1].Autosteer != nil)
	}

	var o, e bytes.Buffer
	if code := run([]string{"report"}, strings.NewReader(""), &o, &e); code != 0 {
		t.Fatalf("report: exit %d: %s", code, e.String())
	}
	if out := o.String(); strings.Contains(out, "last Stop: not read") || !strings.Contains(out, "\n    last Stop: ") {
		t.Errorf("report over one session does not print its last Stop:\n%s", out)
	}
}

// TestRH8g_FlaggedOfJoinsTheCheckpointGates (item 8): flaggedOf keeps the
// claims the done flag reads as unbacked, in the checkpoint's order, each
// with the seq its gate's row rests on (the last green run of a green gate,
// the last run otherwise), ascending and once, "no gate" counted for a gate
// the checkpoint keeps no row for, and the window mark on each. Break: keep a
// backed claim, take the other seq of a row, or drop the count.
func TestRH8g_FlaggedOfJoinsTheCheckpointGates(t *testing.T) {
	seq := func(n int64) *int64 { return &n }
	cp := &verify.Checkpoint{
		Gates: []verify.CheckpointGate{
			{Gate: "spec:unit", State: verify.Stale, LastRunSeq: seq(7), LastGreenSeq: seq(3)},
			{Gate: "spec:lint", State: verify.Green, LastRunSeq: seq(9), LastGreenSeq: seq(8)},
			{Gate: "spec:build", State: verify.Red, LastRunSeq: seq(5)},
			{Gate: "spec:new", State: verify.Absent},
		},
		Claims: []verify.ClaimVerdict{
			{Class: verify.ClaimTestsPass, Strict: true, Verdict: verify.Verdict(verify.Stale), Gates: []verify.SlotID{"spec:unit"}},
			{Class: verify.ClaimLintClean, Strict: true, Verdict: verify.Verdict(verify.Green), Gates: []verify.SlotID{"spec:lint"}},
			{Class: verify.ClaimBuildOK, Strict: false, Verdict: verify.Verdict(verify.Red), Gates: []verify.SlotID{"spec:build"}},
			{Class: verify.ClaimScanClean, Strict: true, Verdict: verify.NoGateKnown},
			{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(verify.Red),
				Gates: []verify.SlotID{"spec:build", "spec:unit", "spec:lint", "spec:gone", "spec:new", "spec:build"}},
		},
	}
	for _, earlier := range []bool{false, true} {
		got := flaggedOf(cp, earlier)
		want := []report.AutosteerClaim{
			{Class: "tests_pass", Verdict: "stale", Evidence: []int64{7}, EarlierWindow: earlier},
			{Class: "done", Verdict: "red", Evidence: []int64{5, 7, 8}, NoGate: 1, EarlierWindow: earlier},
		}
		if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
			t.Errorf("earlier %v:\n got %+v\nwant %+v", earlier, got, want)
		}
	}
	if got := flaggedOf(&verify.Checkpoint{}, false); got == nil || len(got) != 0 {
		t.Errorf("a checkpoint with no claim gives %#v, want an empty non-nil list", got)
	}
}

// TestRH8h_FlaggedClaimsFromTheCheckpoint (item 8): the replay reads no
// final message, so the report's flagged claims are the checkpoint's, which
// the live Stop wrote. "All tests pass." after a green go test and a later
// edit is a strict tests_pass claim on a stale gate: flagged, its evidence
// the go test call's seq, judged on its own turn; with a later turn that only
// reads, judged on an earlier turn's window. Break: read v.Scan.Claims, or
// drop the window mark.
func TestRH8h_FlaggedClaimsFromTheCheckpoint(t *testing.T) {
	for _, c := range []struct {
		name    string
		later   bool
		earlier bool
	}{
		{"the Stop's own turn", false, false},
		{"a later turn that only reads", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newLW(t)
			f.bind(lwSpec("unit"))
			a := f.file("a.go", "package a\n")
			f.rh8Land("p1", "toolu_e1", "Edit", lwEdit(a), false)
			f.rh8Land("p1", "toolu_g", "Bash", lwBash("go test ./..."), false)
			f.rh8Land("p1", "toolu_e2", "Edit", lwEdit(a), false)
			if c.later {
				f.rh8Land("p2", "toolu_r", "Bash", lwBash("ls"), false)
			}
			f.rh8StopSaying("All tests pass.")

			cp, err := gather.LastStop(f.st, "s")
			if err != nil || cp == nil {
				t.Fatalf("premise: a checkpoint (err %v)", err)
			}
			var claim *verify.ClaimVerdict
			for i := range cp.Claims {
				if cp.Claims[i].Class == verify.ClaimTestsPass && cp.Claims[i].Strict {
					claim = &cp.Claims[i]
				}
			}
			if claim == nil || claim.Verdict != verify.Verdict(verify.Stale) || len(claim.Gates) != 1 {
				t.Fatalf("premise: the checkpoint keeps one strict tests_pass claim on one stale gate: %+v", cp.Claims)
			}
			var row *verify.CheckpointGate
			for i := range cp.Gates {
				if cp.Gates[i].Gate == string(claim.Gates[0]) {
					row = &cp.Gates[i]
				}
			}
			run, err := digest.ReadRun(f.st, "s")
			if err != nil {
				t.Fatal(err)
			}
			var goTest []int64
			for _, d := range run.Declarations {
				if d.ToolUseID == "toolu_g" {
					goTest = append(goTest, d.Seq)
				}
			}
			for _, x := range run.Executions {
				if x.ToolUseID == "toolu_g" && x.Seq != nil {
					goTest = append(goTest, *x.Seq)
				}
			}
			if row == nil || row.LastRunSeq == nil || !slices.Contains(goTest, *row.LastRunSeq) {
				t.Fatalf("premise: the claim's gate row rests on the go test call (seqs %v): %+v", goTest, row)
			}
			if r, err := replayLastStop(f.st, "s", nil); err != nil || len(r.View.Scan.Claims) != 0 {
				t.Fatalf("premise: the replay reads no message, so no claim (err %v, %d claims)", err, len(r.View.Scan.Claims))
			}

			got := autosteerOf(f.st, "s")
			want := []report.AutosteerClaim{{Class: "tests_pass", Verdict: "stale", Evidence: []int64{*row.LastRunSeq}, EarlierWindow: c.earlier}}
			if got == nil || fmt.Sprintf("%+v", got.Flagged) != fmt.Sprintf("%+v", want) {
				t.Errorf("flagged:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

// TestRH8i_IdsFlag (item 8): `rashomon report --ids` lists the tool-use ids
// of the calls declared without an execution, and without it the text says
// the count and that --ids lists them; --ledger prints alone and refuses it.
// Break: drop the case, the option, or the refusal.
func TestRH8i_IdsFlag(t *testing.T) {
	f := newLW(t)
	f.rh8Land("p1", "toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), false)
	f.rh8Land("p1", "toolu_x", "Bash", lwBash("make"), true)
	f.stop()

	verb := func(args ...string) (string, string, int) {
		var o, e bytes.Buffer
		code := run(append([]string{"report"}, args...), strings.NewReader(""), &o, &e)
		return o.String(), e.String(), code
	}
	js, _, code := verb("--json")
	if code != 0 || !strings.Contains(js, `"tool_use_id": "toolu_x"`) {
		t.Fatalf("premise: toolu_x is declared without an execution (exit %d):\n%s", code, js)
	}
	plain, stderr, code := verb()
	// Only this line is pinned: the declaration and transcript id lists
	// (unterminated, dropped, the six transcript sets) still print ids, and
	// whether --ids takes them too is the director's ruling, not this test's.
	if code != 0 || strings.Contains(plain, "without execution: toolu_x") || !strings.Contains(plain, "  without execution: 1; ids with --ids\n") {
		t.Errorf("report without --ids (exit %d, %s):\n%s", code, stderr, plain)
	}
	ids, stderr, code := verb("--ids")
	if code != 0 || !strings.Contains(ids, "  without execution: toolu_x (default)\n") {
		t.Errorf("report --ids (exit %d, %s):\n%s", code, stderr, ids)
	}
	if _, stderr, code := verb("--ledger", "--ids", "--session", "s"); code == 0 || !strings.Contains(stderr, "--ledger prints alone: drop --ids") {
		t.Errorf("report --ledger --ids: exit %d, stderr %q", code, stderr)
	}
}

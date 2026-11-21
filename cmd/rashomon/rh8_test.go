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
	"github.com/altrace-dev-role/rashomon/internal/recap"
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
		// The session with records is listed last. The count loop leaves the
		// last session's id behind, and a replay of a session with no records
		// reads as nil: with "s" first, a report that attached over two
		// sessions would attach nil to "s2" and still look like none.
		{"two sessions", []string{"s2", "s"}, []bool{false, false}},
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

// TestRH8g_NotGreenOfJoinsTheCheckpointGates (item 8): notGreenOf keeps the
// strict claims whose verdict is not green, in the checkpoint's order: a gate
// state other than green, or no_gate_known, a claim no gate checked. Each
// carries the seq its gate's row rests on (the last green run of a green
// gate, the last run otherwise), ascending and once, "no gate" counted for a
// gate the checkpoint keeps no row for, and the window mark. A loose claim is
// not kept, whatever its verdict. Break: keep a green claim or a loose
// no_gate_known one, drop the strict no_gate_known one, take the other seq of
// a row, or drop the count.
func TestRH8g_NotGreenOfJoinsTheCheckpointGates(t *testing.T) {
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
			{Class: verify.ClaimTypecheckClean, Strict: false, Verdict: verify.NoGateKnown},
			{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(verify.Red),
				Gates: []verify.SlotID{"spec:build", "spec:unit", "spec:lint", "spec:gone", "spec:new", "spec:build"}},
		},
	}
	for _, earlier := range []bool{false, true} {
		got := notGreenOf(cp, earlier)
		want := []report.AutosteerClaim{
			{Class: "tests_pass", Verdict: "stale", Evidence: []int64{7}, EarlierWindow: earlier},
			{Class: "scan_clean", Verdict: "no_gate_known", Evidence: []int64{}, EarlierWindow: earlier},
			{Class: "done", Verdict: "red", Evidence: []int64{5, 7, 8}, NoGate: 1, EarlierWindow: earlier},
		}
		if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
			t.Errorf("earlier %v:\n got %+v\nwant %+v", earlier, got, want)
		}
	}
	if got := notGreenOf(&verify.Checkpoint{}, false); got == nil || len(got) != 0 {
		t.Errorf("a checkpoint with no claim gives %#v, want an empty non-nil list", got)
	}
}

// TestRH8h_ClaimsNotGreenFromTheCheckpoint (item 8): the replay reads no
// final message, so the report's strict claims not green are the
// checkpoint's, which the live Stop wrote. "All tests pass." after a green go
// test and a later edit is a strict tests_pass claim on a stale gate: listed,
// its evidence the go test call's seq, "done flag read the Stop's turn";
// with a later turn that only reads, "done flag read the turns since the last
// edit". Break: read v.Scan.Claims, or drop the window mark.
func TestRH8h_ClaimsNotGreenFromTheCheckpoint(t *testing.T) {
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
			if got == nil || fmt.Sprintf("%+v", got.NotGreen) != fmt.Sprintf("%+v", want) {
				t.Errorf("strict claims not green:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

// TestRH8k_ClaimsNotGreenNameWhatTheDoneFlagReadNotItsDecision (item 8): the lead
// line and its key list the last Stop's strict claims whose verdict is not
// green: a gate state other than green (recap.ClaimUnbacked, the done flag's
// claim term), or no_gate_known, a claim no gate checked, which the flag reads
// as backed (D79). So the list is not the flag's decision: the flag also reads
// the row, the row's sentence and the unmet expected kinds, it reads a
// no_gate_known claim as backed, and no record keeps what it decided. Four
// Stops, each the view the done flag reads and the checkpoint the same Stop
// writes, where the two differ: E1 a verified row with a strict stale claim,
// E2 a row whose sentence is silent and E4 a strict claim no gate checked, the
// flag quiet and the claim listed; E3 an unmet expected kind with every claim
// green, the flag firing and no claim listed. E5 and E6 are E4 through a live
// Stop and the report's replay of its checkpoint, E6 with the done window set
// (a later turn only reads) beside the quiet flag (card 8b). The line and the
// key name what they read and never say flagged. Break: name the line or the
// key "flagged", drop the no_gate_known claim, or lose the window mark between
// the Stop and the replay.
func TestRH8k_ClaimsNotGreenNameWhatTheDoneFlagReadNotItsDecision(t *testing.T) {
	seq := func(n int64) *int64 { return &n }
	unit := verify.GateState{Gate: "spec:unit", Kind: "test", Family: "r:spec:unit@d:repo", Source: "spec", Required: true,
		State: verify.Green, Reason: verify.OK, LastRun: &verify.RunRef{Seq: 5}, LastGreen: &verify.RunRef{Seq: 5}}
	lint := verify.GateState{Gate: "spec:lint", Kind: "lint", Family: "r:spec:lint@d:repo", Source: "spec",
		State: verify.Stale, Reason: verify.WritesAfterGreen, LastRun: &verify.RunRef{Seq: 2}, LastGreen: &verify.RunRef{Seq: 2}}
	e1 := verify.StopView{Snap: verify.Snapshot{Recording: verify.Complete, Gates: []verify.GateState{unit, lint}},
		Summary: verify.Summary{Row: verify.RowVerified}, Turn: verify.TurnCounts{Writes: 1, PathsWritten: 1, GateRuns: 1, LastWriteSeq: 4, FirstSeq: 1},
		Programs: map[string]string{"spec:unit": "go", "spec:lint": "golangci-lint"},
		Scan: verify.ClaimScan{Claims: []verify.ClaimVerdict{
			{Class: verify.ClaimLintClean, Strict: true, Verdict: verify.Verdict(verify.Stale), Gates: []verify.SlotID{"spec:lint"}}}}}
	e1cp := &verify.Checkpoint{Claims: e1.Scan.Claims, Gates: []verify.CheckpointGate{
		{Gate: "spec:unit", State: verify.Green, LastRunSeq: seq(5), LastGreenSeq: seq(5)},
		{Gate: "spec:lint", State: verify.Stale, LastRunSeq: seq(2), LastGreenSeq: seq(2)}}}

	// E2 is recap's TestIF1 view: the inferred pytest gate bound no run, so
	// its row's sentence is silent and so is the flag.
	pytest := verify.GateState{Gate: "pytest:test", Kind: "test", Family: "r:pytest:test@d:repo", Source: "learned", Required: true,
		State: verify.Absent, Reason: verify.EmptyRun}
	e2 := verify.StopView{Snap: verify.Snapshot{Recording: verify.Complete, Gates: []verify.GateState{pytest}},
		Summary: verify.Summary{Row: verify.RowUnverified}, Turn: verify.TurnCounts{GateRuns: 1, FirstSeq: 1},
		Programs: map[string]string{"pytest:test": "pytest"},
		Scan:     verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(verify.Absent)}}}}
	e2cp := &verify.Checkpoint{Claims: e2.Scan.Claims, Gates: []verify.CheckpointGate{{Gate: "pytest:test", State: verify.Absent}}}

	// E3 is recap's TestEV6 view with one unmet kind: a green done claim on a
	// verified row, its expected evidence projected into a ledger row.
	goTest := verify.GateState{Gate: "go:test", Kind: "test", Family: "r:go:test@d:repo", Source: "learned", Required: true,
		State: verify.Green, Reason: verify.OK, LastRun: &verify.RunRef{Seq: 3}, LastGreen: &verify.RunRef{Seq: 3}}
	e3 := verify.StopView{Snap: verify.Snapshot{Recording: verify.Complete, Gates: []verify.GateState{goTest}},
		Summary: verify.Summary{Row: verify.RowVerified}, Turn: verify.TurnCounts{Writes: 2, PathsWritten: 2, GateRuns: 1, LastWriteSeq: 5},
		Programs: map[string]string{"go:test": "go"},
		Scan:     verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(verify.Green)}}},
		Expected: []verify.Expected{{Kind: "real_data_run", State: verify.ObligationUnmet, Reason: "no gate declares it"}},
		Obligations: []verify.Obligation{{ID: verify.ObligationID(verify.ScopeSession, verify.ExpectedEvidence, "real_data_run"),
			Scope: verify.ScopeSession, Kind: verify.ExpectedEvidence, Source: verify.SourceTask, State: verify.ObligationUnmet, Evidence: []int64{}}}}
	e3cp := &verify.Checkpoint{Claims: e3.Scan.Claims, Gates: []verify.CheckpointGate{
		{Gate: "go:test", State: verify.Green, LastRunSeq: seq(3), LastGreenSeq: seq(3)}}}

	// E4 is a strict claim no gate checked: no gate of its kind exists, so its
	// verdict is no_gate_known, and on a row that is not verified the flag is
	// quiet only because it reads that verdict as backed (D79).
	e4 := verify.StopView{Snap: verify.Snapshot{Recording: verify.Complete},
		Summary: verify.Summary{Row: verify.RowNoGate}, Turn: verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 2, FirstSeq: 1},
		Scan: verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimTestsPass, Strict: true, Verdict: verify.NoGateKnown}}}}
	e4cp := &verify.Checkpoint{Claims: e4.Scan.Claims, Gates: []verify.CheckpointGate{}}

	for _, c := range []struct {
		name  string
		v     verify.StopView
		cp    *verify.Checkpoint
		fires bool
		list  []report.AutosteerClaim
		line  string
		key   string
	}{
		{"E1 a verified row with a strict stale claim", e1, e1cp, false,
			[]report.AutosteerClaim{{Class: "lint_clean", Verdict: "stale", Evidence: []int64{2}}},
			"    strict claims not green, last Stop only: lint_clean stale (evidence #2; done flag read the Stop's turn)",
			`"strict_claims_not_green":[{"class":"lint_clean","verdict":"stale","evidence":[2],"no_gate":0,"earlier_window":false}],"claims_omitted":0`},
		{"E2 a row whose sentence is silent", e2, e2cp, false,
			[]report.AutosteerClaim{{Class: "done", Verdict: "absent", Evidence: []int64{}}},
			"    strict claims not green, last Stop only: done absent (no evidence seq; done flag read the Stop's turn)",
			`"strict_claims_not_green":[{"class":"done","verdict":"absent","evidence":[],"no_gate":0,"earlier_window":false}],"claims_omitted":0`},
		{"E3 an unmet expected kind with every claim green", e3, e3cp, true,
			[]report.AutosteerClaim{},
			"    strict claims not green, last Stop only: none",
			`"strict_claims_not_green":[],"claims_omitted":0`},
		{"E4 a strict claim no gate checked", e4, e4cp, false,
			[]report.AutosteerClaim{{Class: "tests_pass", Verdict: "no_gate_known", Evidence: []int64{}}},
			"    strict claims not green, last Stop only: tests_pass no gate known (no evidence seq; done flag read the Stop's turn)",
			`"strict_claims_not_green":[{"class":"tests_pass","verdict":"no_gate_known","evidence":[],"no_gate":0,"earlier_window":false}],"claims_omitted":0`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, fires := recap.DoneFlag(c.v, "s", false); fires != c.fires {
				t.Fatalf("premise: the done flag fires %v on this Stop, want %v", fires, c.fires)
			}
			list := notGreenOf(c.cp, c.v.Done != nil)
			if fmt.Sprintf("%+v", list) != fmt.Sprintf("%+v", c.list) {
				t.Errorf("claims:\n got %+v\nwant %+v", list, c.list)
			}
			rh8LineAndKey(t, &report.Autosteer{Stop: &report.AutosteerStop{Seq: 9, Row: string(c.v.Summary.Row)}, NotGreen: list}, c.line, c.key)
		})
	}

	// E5 and E6 are E4 through a live Stop: a spec that declares a lint gate
	// and no test gate, an edit, and the final message "All tests pass.", a
	// strict tests_pass claim no gate checked (no file of the work tree names
	// a test command, so none is inferred). In E6 a later turn only reads, so
	// the Stop's done window (verify.StopView.Done) is the turns since the
	// edit, and the flag is quiet beside it. The list is the report's own:
	// the checkpoint the Stop wrote, through autosteerOf's replay.
	for _, c := range []struct {
		name  string
		later bool
		line  string
		key   string
	}{
		{"E5 a live Stop with a strict claim no gate checked", false,
			"    strict claims not green, last Stop only: tests_pass no gate known (no evidence seq; done flag read the Stop's turn)",
			`"strict_claims_not_green":[{"class":"tests_pass","verdict":"no_gate_known","evidence":[],"no_gate":0,"earlier_window":false}],"claims_omitted":0`},
		{"E6 the same claim after a turn that only reads, the done window set beside a quiet flag", true,
			"    strict claims not green, last Stop only: tests_pass no gate known (no evidence seq; done flag read the turns since the last edit)",
			`"strict_claims_not_green":[{"class":"tests_pass","verdict":"no_gate_known","evidence":[],"no_gate":0,"earlier_window":true}],"claims_omitted":0`},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newLW(t)
			f.bind(`{"rashomon_spec": 1, "gates": [{"id": "lint", "kind": "lint", "argv": ["golangci-lint", "run"]}]}`)
			f.rh8Land("p1", "toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), false)
			if c.later {
				f.rh8Land("p2", "toolu_r", "Bash", lwBash("ls"), false)
			}
			v := f.rh8StopSaying("All tests pass.")
			if len(v.Scan.Claims) != 1 || v.Scan.Claims[0].Class != verify.ClaimTestsPass || !v.Scan.Claims[0].Strict ||
				v.Scan.Claims[0].Verdict != verify.NoGateKnown {
				t.Fatalf("premise: the Stop reads one strict tests_pass claim no gate checked: %+v", v.Scan.Claims)
			}
			if (v.Done != nil) != c.later {
				t.Fatalf("premise: the Stop's done window is set %v, want %v", v.Done != nil, c.later)
			}
			if _, fires := recap.DoneFlag(v, "s", false); fires {
				t.Fatal("premise: the done flag fires on this Stop, want it quiet")
			}
			a := autosteerOf(f.st, "s")
			want := []report.AutosteerClaim{{Class: "tests_pass", Verdict: "no_gate_known", Evidence: []int64{}, EarlierWindow: c.later}}
			if a == nil || fmt.Sprintf("%+v", a.NotGreen) != fmt.Sprintf("%+v", want) {
				t.Fatalf("claims:\n got %+v\nwant %+v", a, want)
			}
			rh8LineAndKey(t, a, c.line, c.key)
		})
	}
}

// rh8LineAndKey checks a's strict-claims line in the text, the story's first,
// against line, and its --json against key, with no key "flagged".
func rh8LineAndKey(t *testing.T, a *report.Autosteer, line, key string) {
	t.Helper()
	var b bytes.Buffer
	if err := report.Text(&b, &report.Report{Sessions: []report.Session{{SessionID: "s", Autosteer: a}}}); err != nil {
		t.Fatal(err)
	}
	_, story, _ := strings.Cut(b.String(), "\n  story\n")
	if first, _, _ := strings.Cut(story, "\n"); first != line {
		t.Errorf("lead line:\n got %q\nwant %q", first, line)
	}
	body, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), key) || strings.Contains(string(body), `"flagged"`) {
		t.Errorf("--json:\n got %s\nwant it to hold %s and no key \"flagged\"", body, key)
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

package hook

import (
	"bytes"
	"encoding/json"
	"maps"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/gateout"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// go1Payload is a payload for one Bash call, toolu_go1 in session s, with
// extra merged in: a tool_response, or a failure event's error.
func go1Payload(t *testing.T, event, command string, extra map[string]any) []byte {
	t.Helper()
	p := map[string]any{"session_id": "s", "tool_use_id": "toolu_go1", "tool_name": "Bash", "hook_event_name": event,
		"cwd": "/tmp/project", "tool_input": map[string]string{"command": command}}
	maps.Copy(p, extra)
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// go1Post lands the post of one Bash call and returns its execution. Its
// clock stands still, as go1gGate's does, so the read is never past the
// budget however long the capture takes under load; the budget is pinned on
// its own clock (TestGO1i).
func go1Post(t *testing.T, event, command string, extra map[string]any) store.Execution {
	t.Helper()
	now := time.Now()
	return go1PostAt(t, func() time.Time { return now }, event, command, extra)
}

// go1PostAt is go1Post on the post's clock now.
func go1PostAt(t *testing.T, now func() time.Time, event, command string, extra map[string]any) store.Execution {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	p := NewPost(st, now)
	p.Close(&Signals{}, p.Capture(bytes.NewReader(go1Payload(t, event, command, extra))))
	run, err := st.ReadRun("s")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Executions) != 1 {
		t.Fatalf("read %d executions, want 1", len(run.Executions))
	}
	return run.Executions[0]
}

// streams is a tool_response carrying stdout and stderr.
func streams(stdout, stderr string) map[string]any {
	return map[string]any{"tool_response": map[string]any{"stdout": stdout, "stderr": stderr, "interrupted": false}}
}

func wantGateOutput(t *testing.T, x store.Execution, kind, parser, reason string) {
	t.Helper()
	if x.GateOutput == nil {
		t.Fatalf("gate_output is nil, want kind %q", kind)
	}
	got := ""
	if x.GateOutput.Reason != nil {
		got = *x.GateOutput.Reason
	}
	if x.GateOutput.Kind != kind || x.GateOutput.Parser != parser || got != reason {
		t.Errorf("gate_output %q parser %q reason %q, want %q %q %q", x.GateOutput.Kind, x.GateOutput.Parser, got, kind, parser, reason)
	}
}

// TestGO1_GateRunOutputIsClassified: a gate run's stdout is read and
// classified, and the three closed words are all that land (2.2.3, 2.2.10).
// Break: drop the read, and gate_output stays nil, as it was on every live
// record before this card.
func TestGO1_GateRunOutputIsClassified(t *testing.T) {
	x := go1Post(t, "PostToolUse", "go test ./...", streams("=== RUN   TestX\n--- FAIL: TestX (0.00s)\n    x_test.go:5: boom\n", ""))
	wantGateOutput(t, x, "fail_line", "go", "")
}

// TestGO1b_SummaryPassWhereTheRunnerAllowsIt: a runner whose table has a
// pass line gets summary_pass from it, and a runner that prints its summary
// on stderr (cargo) is read there when stdout says nothing. Break: skip
// stderr, and cargo's run is no_summary.
func TestGO1b_SummaryPassWhereTheRunnerAllowsIt(t *testing.T) {
	for _, tc := range []struct{ name, cmd, stdout, stderr, parser string }{
		{"pytest on stdout", "pytest -q", "...\n3 passed in 0.12s\n", "", "pytest"},
		{"cargo on stderr", "cargo test", "running 3 tests\n", "test result: ok. 3 passed; 0 failed; 0 ignored\n", "cargo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantGateOutput(t, go1Post(t, "PostToolUse", tc.cmd, streams(tc.stdout, tc.stderr)), "summary_pass", tc.parser, "")
		})
	}
}

// TestGO1c_NonGateCallHasNoGateOutput: a call the shape does not name a gate
// run is never scanned, whatever its output says. Break: read every Bash
// call's output, and a cat of a file that holds a FAIL line gets a verdict.
func TestGO1c_NonGateCallHasNoGateOutput(t *testing.T) {
	x := go1Post(t, "PostToolUse", "cat notes.txt", streams("--- FAIL: TestX (0.00s)\nok  \tpkg\t0.01s\n", ""))
	if x.GateOutput != nil {
		t.Errorf("gate_output %+v on a cat, want nil", *x.GateOutput)
	}
}

// TestWR1_AWrapperCallKeepsOnlyARunnersRead: a call of class execute that is
// no gate run (a script, a shell) is read for a runner's signature, and its
// gate_output is kept only where a runner's row read it; a launcher's build
// target (make check), whose kind reads nothing, is read the same way and
// keeps today's no_summary where no runner signed it. The go lines are
// go1.27.3's as recorded in gateout's testdata/wr1. Controls: a cat of the
// same output (TestGO1c), and a script whose output no runner signed: TAP's
// "ok 1 - adds", make's own marker. Break: drop the wrapper read, keep an
// unsigned read, or let a delegate's marker through a wrapper.
func TestWR1_AWrapperCallKeepsOnlyARunnersRead(t *testing.T) {
	const goPass, goFail = "ok  \tm\t0.004s\n", "--- FAIL: TestA (0.00s)\nFAIL\nFAIL\tm\t0.004s\nFAIL\n"
	for _, tc := range []struct{ name, cmd, stdout, kind, parser string }{
		{"script, go passed", "bash scripts/test.sh", goPass, "summary_pass", "go"},
		{"script, go failed, piped", "bash scripts/test.sh | tail -20", goFail, "summary_fail", "go"},
		{"./script, go passed", "./scripts/test.sh", goPass, "summary_pass", "go"},
		// go signed stdout and printed no outcome; an empty stderr read
		// after it leaves stdout's words, not the wrapper's.
		{"script, go ran, its summary cut", "bash scripts/test.sh", "=== RUN   TestA\n--- PASS: TestA (0.00s)\n", "no_summary", "go"},
		{"make check, go passed", "make check", "go test ./...\n" + goPass, "summary_pass", "go"},
		{"make check, nothing signed", "make check", "gofmt -l .\n", "no_summary", "make"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantGateOutput(t, go1Post(t, "PostToolUse", tc.cmd, streams(tc.stdout, "")), tc.kind, tc.parser, "")
		})
	}
	fail := map[string]any{"error": "Exit code 1\n" + goFail, "is_interrupt": false}
	wantGateOutput(t, go1Post(t, FailureEvent, "bash scripts/test.sh", fail), "summary_fail", "go", "")
	for _, out := range []string{"done\n", "1..2\nok 1 - adds\nok 2 - subtracts\n", "make: *** [test] Error 2\n"} {
		if x := go1Post(t, "PostToolUse", "bash scripts/test.sh", streams(out, "")); x.GateOutput != nil {
			t.Errorf("gate_output %+v on a script that printed %q, want nil", *x.GateOutput, out)
		}
	}
}

// TestWR1_TheDisclosureNamesTheWrapperRead (D4): the consent screen says the
// output of a command rashomon does not recognize is read in memory to find a
// runner's summary, the read TestWR1_AWrapperCallKeepsOnlyARunnersRead pins
// (the words are CONSENT-SHORT1's, 6737784c), and that such a command keeps
// gate_output's closed word where a runner's line is in its output, with or
// without a summary: that test's "summary cut" case keeps no_summary, its
// unsigned cases keep nothing. The STORED paragraph is found by
// disclose.StoredHeading, and a paragraph not found fails the test.
// Break: ship the read or the kept word without its words.
func TestWR1_TheDisclosureNamesTheWrapperRead(t *testing.T) {
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ owner, head, clause string }{
		{disclose.OwnerVerify, "READ IN MEMORY, NEVER STORED: ", "and of a command rashomon does not recognize, to find a test runner's summary in it"},
		{disclose.OwnerRecord, disclose.StoredHeading, "for a gate run, or another command whose output holds a line a test runner prints about its run, one closed word that classifies its output (a passing or failing summary, a failure line, an empty run, "},
		{disclose.OwnerRecord, disclose.StoredHeading, "or none found), and not the output, which is read in memory"},
	} {
		found := false
		for _, p := range inv.Paragraphs {
			found = found || p.Owner == c.owner && strings.HasPrefix(p.Text, c.head) && strings.Contains(p.Text, c.clause)
		}
		if !found {
			t.Errorf("no %s paragraph says %s... %q", c.owner, c.head, c.clause)
		}
	}
}

// TestGO1d_OverCapIsUnreadTooLarge: output past gateout.MaxScanBytes is
// unread, too_large, and reading it allocates about the cap, not the
// response: a 4 MiB stdout decodes at most 1 MiB. Break: size the buffer by
// the span alone, and the read allocates the whole response.
func TestGO1d_OverCapIsUnreadTooLarge(t *testing.T) {
	over := strings.Repeat("a", gateout.MaxScanBytes) + "\n"
	wantGateOutput(t, go1Post(t, "PostToolUse", "go test ./...", streams(over, "")), "unread", "", "too_large")

	in, err := hookio.Intake(bytes.NewReader(go1Payload(t, "PostToolUse", "go test ./...", streams(strings.Repeat("ok\n", 4<<20/3), ""))))
	if err != nil {
		t.Fatal(err)
	}
	far := time.Now().Add(time.Hour)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out := gateOutput(in, false, "test", "go", far, time.Now)
	runtime.ReadMemStats(&after)
	if out.Kind != "unread" {
		t.Errorf("a 4 MiB stdout reads %q, want unread", out.Kind)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > gateout.MaxScanBytes+64<<10 {
		t.Errorf("reading a 4 MiB stdout allocated %d bytes, want at most the %d-byte cap and 64 KiB", got, gateout.MaxScanBytes)
	}

	// Under the cap, a buffer sized by the encoded span is outgrown only by
	// invalid UTF-8, each byte of which decodes to three: unread, decode,
	// not too_large.
	bad, err := hookio.Intake(strings.NewReader("{\"tool_name\":\"Bash\",\"tool_response\":{\"stdout\":\"\xff\xff\xff\xff\"}}"))
	if err != nil {
		t.Fatal(err)
	}
	var src [2]jsonscan.Span
	if err := jsonscan.Field(bad.Raw, bad.Spans[hookio.ToolResponse], streamKeys, src[:]); err != nil {
		t.Fatal(err)
	}
	if got := readOutput(bad.Raw, src, false, "go", make([]byte, 0, src[0].Len)); got.Reason == nil || *got.Reason != "decode" {
		t.Errorf("an invalid UTF-8 stdout under the cap reads %+v, want unread, decode", got)
	}
}

// TestGO1e_FailureEventReadsErrorAfterLineOne: a failure event carries no
// tool_response; its error is read after its first line, which is the exit
// code's (exitCode). Break: read line one too, and a first line that says
// FAIL turns a passing summary into summary_fail.
func TestGO1e_FailureEventReadsErrorAfterLineOne(t *testing.T) {
	fail := map[string]any{"error": "Exit code 1\n--- FAIL: TestX (0.00s)\nFAIL\tpkg\t0.01s\n", "is_interrupt": false}
	x := go1Post(t, FailureEvent, "go test ./...", fail)
	wantGateOutput(t, x, "summary_fail", "go", "")
	if x.ExitCode == nil || *x.ExitCode != 1 {
		t.Errorf("exit_code %v, want 1 from line one", x.ExitCode)
	}
	lineOne := map[string]any{"error": "FAIL\tline-one\nok  \tpkg\t0.01s\n", "is_interrupt": false}
	wantGateOutput(t, go1Post(t, FailureEvent, "go test ./...", lineOne), "summary_pass", "go", "")
}

// go1gGate is the one gate the evaluator reads after a post of cmd with the
// stdout out: the pipe's masked run, live.
func go1gGate(t *testing.T, cmd, out string) (verify.GateState, store.Execution) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	now := time.Now()
	clock := func() time.Time { return now }
	// The recording is complete only beside a verified post-phase coverage
	// record, which a test's own Close cannot write (no installed entry), so
	// the post is captured without one and the record is the steer fixture's
	// (newST).
	cov := store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion, RecordedAtMS: now.UnixMilli(),
		SessionID: "s", Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}
	if err := st.AppendCoverage(cov); err != nil {
		t.Fatal(err)
	}
	h := New(st, clock)
	h.Close(&Signals{}, h.Capture(bytes.NewReader(go1Payload(t, "PreToolUse", cmd, nil))))
	p := NewPost(st, clock)
	if err := p.Capture(bytes.NewReader(go1Payload(t, "PostToolUse", cmd, streams(out, "")))); err != nil {
		t.Fatal(err)
	}
	run, err := st.ReadRun("s")
	if err != nil {
		t.Fatal(err)
	}
	snap := verify.Evaluate(run, nil, nil, verify.Point{Seq: p.landedSeq, SessionID: "s"}, now)
	if snap.Recording != verify.Complete || len(snap.Gates) != 1 || len(run.Executions) != 1 {
		t.Fatalf("recording %s with %d gates, want complete and the one masked run", snap.Recording, len(snap.Gates))
	}
	return snap.Gates[0], run.Executions[0]
}

// go1gFrom is the provenance word of the gate's last run, empty where the
// exit status decided it or there is no run.
func go1gFrom(g verify.GateState) string {
	if g.LastRun == nil {
		return ""
	}
	return g.LastRun.OutcomeFrom.String()
}

// TestGO1g_MaskedPipeReadsTheSummary: PG2's sentence, live. A pipe that drops
// the summary's neighbours (head) hides go test's status, and the evaluator
// still reads the summary line the post classified: masked_inconclusive with
// a success line, not without one. Break: leave gate_output unwritten, and
// the reason is no success line.
func TestGO1g_MaskedPipeReadsTheSummary(t *testing.T) {
	g, _ := go1gGate(t, "go test ./... | head -5", "ok  \tpkg\t0.01s\n")
	if g.State != verify.MaskedInconclusive || g.Reason != verify.MaskedWithSuccessLine {
		t.Errorf("gate %s %s, want %s %s", g.State, g.Reason, verify.MaskedInconclusive, verify.MaskedWithSuccessLine)
	}
}

// TestMO1_ATailedRunnerIsAttributed: the run the card names, live. `python3
// -m pytest -q 2>&1 | tail -N` is attributed to pytest, not to python3: the
// record names the runner and the keeping stage, the post classifies the
// output by the pytest table, and the evaluator takes the outcome from the
// summary line it read, saying so (outcome_from). Break: key gate_output by
// the program, and the output is unattributable and the run stays masked.
func TestMO1_ATailedRunnerIsAttributed(t *testing.T) {
	const cmd = "python3 -m pytest -q 2>&1 | tail -5"
	g, x := go1gGate(t, cmd, "..\n3 passed in 0.12s\n")
	if x.RunnerName == nil || *x.RunnerName != "python3 -m pytest" || x.MaskedBy == nil || *x.MaskedBy != "tail" {
		t.Errorf("runner_name %v, masked_by %v, want python3 -m pytest and tail", x.RunnerName, x.MaskedBy)
	}
	wantGateOutput(t, x, "summary_pass", "pytest", "")
	if g.State != verify.Green || go1gFrom(g) != verify.OutcomeFromOutput {
		t.Errorf("gate %s from %q, want green from %s", g.State, go1gFrom(g), verify.OutcomeFromOutput)
	}
	// The same run under a stage that drops the summary stays masked.
	g, x = go1gGate(t, "python3 -m pytest -q 2>&1 | head -5", "..\n3 passed in 0.12s\n")
	if x.MaskedBy == nil || *x.MaskedBy != "head" || g.State != verify.MaskedInconclusive || go1gFrom(g) != "" {
		t.Errorf("head: masked_by %v, gate %s from %q, want head, masked_inconclusive, no provenance", x.MaskedBy, g.State, go1gFrom(g))
	}
	// A failing summary through tail is red from the output.
	g, _ = go1gGate(t, cmd, "..F\n1 failed, 2 passed in 0.12s\n")
	if g.State != verify.Red || g.Reason != verify.FailedOutput || go1gFrom(g) != verify.OutcomeFromOutput {
		t.Errorf("failing summary: gate %s %s from %q, want red failed_output from output_summary", g.State, g.Reason, go1gFrom(g))
	}
}

// TestMO1_GoAndCargoReadThroughTail: go's bare FAIL and cargo's error lines
// are failing output through a keeping stage, by the table word.
func TestMO1_GoAndCargoReadThroughTail(t *testing.T) {
	for _, c := range []struct{ cmd, out, runner string }{
		{"go test ./... 2>&1 | tail -20", "--- FAIL: TestX (0.00s)\nFAIL\nFAIL\tpkg\t0.01s\n", "go"},
		{"cargo test 2>&1 | tail -20", "test result: FAILED. 0 passed; 1 failed\nerror: test failed, to rerun pass `--lib`\n", "cargo"},
	} {
		g, x := go1gGate(t, c.cmd, c.out)
		wantGateOutput(t, x, "summary_fail", c.runner, "")
		if g.State != verify.Red || go1gFrom(g) != verify.OutcomeFromOutput {
			t.Errorf("%s: gate %s from %q, want red from output_summary", c.cmd, g.State, go1gFrom(g))
		}
	}
}

// TestGO1h_TheBufferIsClearedBeforeReturn: readOutput clears the buffer it
// decoded into before it returns, so no later panic value or heap read finds
// a byte of the output. Break: drop the clear, and the sentinel is still in
// the buffer.
func TestGO1h_TheBufferIsClearedBeforeReturn(t *testing.T) {
	in, err := hookio.Intake(strings.NewReader(`{"tool_name":"Bash","tool_response":{"stdout":"SENTINEL-go1h\nok  \tpkg\t0.01s\n"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var src [2]jsonscan.Span
	if err := jsonscan.Field(in.Raw, in.Spans[hookio.ToolResponse], streamKeys, src[:]); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 0, 256)
	if got := readOutput(in.Raw, src, false, "go", buf); got.Kind != "summary_pass" {
		t.Fatalf("read %q, want summary_pass", got.Kind)
	}
	if bytes.Contains(buf[:cap(buf)], []byte("SENTINEL")) {
		t.Error("the buffer still holds the output after readOutput returned")
	}
}

// TestGO1i_TheReadStopsAtTheBudget: a gate run's output is read until 50 ms
// after its capture starts, the tech spec's figure (2.2.3), and not after.
// The post's clock reads the capture's start first and then a later instant
// on every read: at the budget's end the run is classified, and a printed
// status is the outcome; a nanosecond past it the run is unread, budget, and
// the printed status is not read either. The clock is the post's own
// (NewPost), so no load moves the result. Break: drop either read's deadline
// check, or move the budget off 50 ms.
func TestGO1i_TheReadStopsAtTheBudget(t *testing.T) {
	const budget = 50 * time.Millisecond
	const pass, printed = "ok  \tpkg\t0.01s\n", "--- FAIL: TestAdd (0.00s)\nFAIL\nFAIL\tpkg\t0.012s\nFAIL\n2\n"
	for _, tc := range []struct {
		name, cmd, out       string
		after                time.Duration
		kind, parser, reason string
	}{
		{"at the budget", "go test ./...", pass, budget, "summary_pass", "go", ""},
		{"past the budget", "go test ./...", pass, budget + time.Nanosecond, "unread", "", "budget"},
		{"a printed status at the budget", "go test ./... ; echo $?", printed, budget, gateout.KindPrintedFail, gateout.ParserExitStatus, ""},
		{"a printed status past the budget", "go test ./... ; echo $?", printed, budget + time.Nanosecond, "unread", "", "budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, reads := time.Now(), 0
			clock := func() time.Time {
				if reads++; reads == 1 {
					return start
				}
				return start.Add(tc.after)
			}
			wantGateOutput(t, go1PostAt(t, clock, "PostToolUse", tc.cmd, streams(tc.out, "")), tc.kind, tc.parser, tc.reason)
		})
	}
}

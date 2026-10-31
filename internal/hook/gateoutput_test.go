package hook

import (
	"bytes"
	"encoding/json"
	"maps"
	"runtime"
	"strings"
	"testing"
	"time"

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

// go1Post lands the post of one Bash call and returns its execution.
func go1Post(t *testing.T, event, command string, extra map[string]any) store.Execution {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	p := NewPost(st, time.Now)
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
	return g.LastRun.OutcomeFrom
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

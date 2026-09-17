package acceptance

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// Injection points, mirroring internal/fault.
const (
	pointHookStart  = "hook.start"
	pointHookParsed = "hook.parsed"
	pointStoreWrite = "store.write"
)

// Faults that panic rather than return an error. A fault that returns an error
// exercises error handling; only a panic reaches the exit code that blocks a
// tool call.
var panickingFaults = []string{
	"nil_map_write",
	"nil_pointer_deref",
	"index_out_of_range",
	"send_on_closed_channel",
	"plain_panic",
}

var injectionPoints = []string{pointHookStart, pointHookParsed, pointStoreWrite}

// TestH1Control is the assertion the rest of H-1 rests on.
//
// Every other case here claims "this did not exit 2". That claim is worth
// nothing unless the harness can observe a 2 when one really happens, and a
// harness that always reports 0 would make the entire file pass while the panic
// barrier was missing. This runs a binary that panics with no recovery at all.
func TestH1Control_HarnessObservesExitTwo(t *testing.T) {
	cmd := exec.Command(panickerBin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()

	if got := cmd.ProcessState.ExitCode(); got != 2 {
		t.Fatalf("control fixture exited %d, want 2; until this passes, every exit-code assertion in this package is vacuous", got)
	}
	if !strings.Contains(stderr.String(), "panic:") {
		t.Errorf("control fixture did not print a panic; it may not be panicking for the reason we think")
	}
}

// TestH1InjectionIsCompiledIn guards against the other way this file goes
// vacuous: a binary built without the fault tag injects nothing, so every fault
// case below would pass by never having faulted.
func TestH1InjectionIsCompiledIn(t *testing.T) {
	home := t.TempDir()
	res := runHook(t, home, defaultPayload().build(t), "ATTEST_FAULT="+pointHookStart+":plain_panic")

	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}
	if got := recordsOfType(readRecords(t, home, testSession, "records.ndjson"), "declaration"); len(got) != 0 {
		t.Fatalf("a fault at %s still produced %d declaration(s); the binary under test was built without fault injection", pointHookStart, len(got))
	}
}

// TestH1_NoFaultBlocksTheToolCall is the contract: whatever goes wrong inside
// this program, the agent's tool call proceeds.
func TestH1_NoFaultBlocksTheToolCall(t *testing.T) {
	for _, fault := range panickingFaults {
		for _, point := range injectionPoints {
			t.Run(fault+"@"+point, func(t *testing.T) {
				home := t.TempDir()
				res := runHook(t, home, defaultPayload().build(t), "ATTEST_FAULT="+point+":"+fault)

				if res.exitCode != 0 {
					t.Fatalf("exit code %d, want 0 (2 would block the tool call)", res.exitCode)
				}
				assertNoTraceback(t, res)

				// The fault must have actually fired, and the run must admit it
				// could not measure itself. Exit 0 with a cheerfully complete
				// record would be the worse bug.
				session := testSession
				if point == pointHookStart {
					session = "unattributed" // the payload was never parsed
				}
				if got := recordsOfType(readRecords(t, home, session, "records.ndjson"), "declaration"); len(got) != 0 {
					t.Errorf("declaration was written despite a fault at %s", point)
				}
				assertCoverage(t, home, session, "unverified", "internal_error")
			})
		}
	}
}

// TestH1_GoroutinePanicIsContained is separated because its correct outcome is
// different. A panic in a spawned goroutine cannot be recovered by the goroutine
// that started it -- the runtime kills the process -- so containment has to
// happen inside the goroutine itself. When it works, the handler carries on and
// records a complete, verified run.
func TestH1_GoroutinePanicIsContained(t *testing.T) {
	for _, point := range injectionPoints {
		t.Run(point, func(t *testing.T) {
			home := t.TempDir()
			res := runHook(t, home, defaultPayload().build(t), "ATTEST_FAULT="+point+":goroutine_panic")

			if res.exitCode != 0 {
				t.Fatalf("exit code %d, want 0; a goroutine panic escaped its recover", res.exitCode)
			}
			assertNoTraceback(t, res)

			if got := recordsOfType(readRecords(t, home, testSession, "records.ndjson"), "declaration"); len(got) != 1 {
				t.Errorf("got %d declarations, want 1: a contained goroutine panic should not cost the record", len(got))
			}
			assertCoverage(t, home, testSession, "verified", "")
		})
	}
}

func assertNoTraceback(t *testing.T, res result) {
	t.Helper()

	// Hook stdout and stderr are written to Claude Code's debug log, so a
	// traceback here is both a panic that escaped and a content leak: panic
	// values routinely carry the string the program was holding.
	for _, marker := range []string{"panic:", "goroutine ", "runtime error:"} {
		if strings.Contains(res.stderr, marker) {
			t.Errorf("stderr contains %q, which reaches Claude Code's debug log: %q", marker, res.stderr)
		}
	}
	if res.stdout != "" {
		t.Errorf("hook wrote to stdout, which Claude Code parses as control output: %q", res.stdout)
	}
}

func assertCoverage(t *testing.T, home, session, wantState, wantReason string) {
	t.Helper()

	recs := recordsOfType(readRecords(t, home, session, "coverage.ndjson"), "coverage")
	if len(recs) != 1 {
		t.Fatalf("got %d coverage records for session %q, want 1", len(recs), session)
	}

	if got, _ := recs[0].fields["state"].(string); got != wantState {
		t.Errorf("coverage state is %q, want %q", got, wantState)
	}

	reason := recs[0].fields["reason"]
	if wantReason == "" {
		if reason != nil {
			t.Errorf("coverage reason is %v, want null", reason)
		}
		return
	}
	if got, _ := reason.(string); got != wantReason {
		t.Errorf("coverage reason is %v, want %q", reason, wantReason)
	}
}

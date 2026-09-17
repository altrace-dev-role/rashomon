package acceptance

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// Injection points, mirroring internal/fault.
const (
	pointHookStart            = "hook.start"
	pointHookParsed           = "hook.parsed"
	pointStoreWrite           = "store.write"
	pointHookAfterDeclaration = "hook.after_declaration"
	pointSettingsOpened       = "settings.write.opened"
	pointSettingsBeforeRename = "settings.write.before_rename"
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

var injectionPoints = []string{pointHookStart, pointHookParsed, pointStoreWrite, pointHookAfterDeclaration}

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
	e := newEnv(t)
	res := e.hook(defaultPayload().build(t), "ATTEST_FAULT="+pointHookStart+":plain_panic")

	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}
	if got := e.declarations(testSession); len(got) != 0 {
		t.Fatalf("a fault at %s still produced %d declaration(s); the binary under test was built without fault injection", pointHookStart, len(got))
	}
}

// TestH1_NoFaultBlocksTheToolCall is the contract: whatever goes wrong inside
// this program, the agent's tool call proceeds.
func TestH1_NoFaultBlocksTheToolCall(t *testing.T) {
	for _, fault := range panickingFaults {
		for _, point := range injectionPoints {
			t.Run(fault+"@"+point, func(t *testing.T) {
				e := newEnv(t)
				e.watched(testSession)
				res := e.hook(defaultPayload().build(t), "ATTEST_FAULT="+point+":"+fault)

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
				decls := e.declarations(session)
				if point == pointHookAfterDeclaration {
					// The declaration landed before the fault; what must be
					// true is that it was still closed and the run still
					// admits the failure.
					if len(decls) != 1 {
						t.Errorf("got %d declarations, want 1", len(decls))
					}
					if terms := e.terminals(session); len(terms) != 1 || terms[0].str("outcome") != "error" {
						t.Errorf("terminal records %v, want one with outcome error", terms)
					}
				} else if len(decls) != 0 {
					t.Errorf("declaration was written despite a fault at %s", point)
				}
				assertCallCoverage(t, e, session, "unverified", "internal_error")
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
			e := newEnv(t)
			e.watched(testSession)
			res := e.hook(defaultPayload().build(t), "ATTEST_FAULT="+point+":goroutine_panic")

			if res.exitCode != 0 {
				t.Fatalf("exit code %d, want 0; a goroutine panic escaped its recover", res.exitCode)
			}
			assertNoTraceback(t, res)

			if got := e.declarations(testSession); len(got) != 1 {
				t.Errorf("got %d declarations, want 1: a contained goroutine panic should not cost the record", len(got))
			}
			assertCallCoverage(t, e, testSession, "verified", "")
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

// assertCallCoverage checks the most recent call-phase coverage record.
func assertCallCoverage(t *testing.T, e *env, session, wantState, wantReason string) {
	t.Helper()
	recs := e.coverage(session, "call")
	if len(recs) == 0 {
		t.Fatalf("no call-phase coverage record for session %q", session)
	}
	last := recs[len(recs)-1]

	if got := last.str("state"); got != wantState {
		t.Errorf("coverage state is %q, want %q (reason %v)", got, wantState, last.fields["reason"])
	}
	reason := last.fields["reason"]
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

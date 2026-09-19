package acceptance

import (
	"reflect"
	"runtime"
	"syscall"
	"testing"
)

func (e *env) process(payload string) (signal func(syscall.Signal), wait func() int) {
	e.t.Helper()
	cmd := e.command(payload, []string{"RASHOMON_FAULT=" + pointHookAfterDeclaration + ":hang"}, "hook")
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	waitFor(e.t, 5e9, "the declaration to land", func() bool {
		return len(e.declarations(testSession)) == 1
	})
	e.t.Cleanup(func() { _ = cmd.Process.Kill() })
	return func(sig syscall.Signal) {
			if err := cmd.Process.Signal(sig); err != nil {
				e.t.Fatal(err)
			}
		}, func() int {
			_ = cmd.Wait()
			return cmd.ProcessState.ExitCode()
		}
}

// TestH11_UncontrolledPathIsRecordedAtRunTime: SIGKILL is the definition of the
// uncontrolled path. Nothing runs, so no terminal record appears -- and the
// end-of-run probe reads that absence into the run's own coverage record,
// while the run is still the run, rather than leaving it for a report to guess.
func TestH11_UncontrolledPathIsRecordedAtRunTime(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows uses mandatory file locks (LockFileEx): the hung process holds
		// an exclusive lock that blocks even readers, so the test's declaration
		// poll never sees the store. Tracked separately.
		t.Skip("Windows mandatory locking prevents declaration reads while process is hung")
	}
	e := newEnv(t)
	e.watched(testSession)

	kill, wait := e.process(defaultPayload().build(t))
	kill(syscall.SIGKILL)
	wait()

	if got := e.declarations(testSession); len(got) != 1 {
		t.Fatalf("got %d declarations, want exactly 1", len(got))
	}
	if got := e.terminals(testSession); len(got) != 0 {
		t.Fatalf("a SIGKILLed handler left %d terminal records; it cannot have written any", len(got))
	}

	if res := e.probe("end", testSession); res.exitCode != 0 {
		t.Fatalf("probe end: exit %d", res.exitCode)
	}
	end := e.coverage(testSession, "end")
	if len(end) != 1 {
		t.Fatalf("got %d end-phase coverage records, want 1", len(end))
	}
	if end[0].str("state") != "unverified" || end[0].str("reason") != "unterminated_entry" {
		t.Errorf("end coverage is %s/%v, want unverified/unterminated_entry", end[0].str("state"), end[0].fields["reason"])
	}
	assertKeySet(t, end[0], coverageKeys)

	rep := e.report(testSession)
	if !reflect.DeepEqual(rep.Declarations.Unterminated, []string{testToolUseID}) {
		t.Errorf("report unterminated is %v, want [%s]", rep.Declarations.Unterminated, testToolUseID)
	}
	if rep.Coverage.State != "unverified" || !e.hasReason(rep, "unterminated_entry") {
		t.Errorf("report coverage is %s (%v), want unverified with unterminated_entry", rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH12_SIGTERMIsAControlledExit: SIGTERM is what a hook timeout cancellation
// delivers, and it is catchable. The handler closes its declaration, records
// that it was cancelled, exits 0 -- and the coverage record it writes on that
// path carries no count of any kind.
func TestH12_SIGTERMIsAControlledExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Same mandatory-lock issue as H11; SIGTERM semantics also differ on
		// Windows (no POSIX signal delivery). Tracked separately.
		t.Skip("Windows mandatory locking and signal semantics differ")
	}
	e := newEnv(t)
	e.watched(testSession)

	signal, wait := e.process(defaultPayload().build(t))
	signal(syscall.SIGTERM)
	if code := wait(); code != 0 {
		t.Fatalf("exit code %d after SIGTERM, want 0", code)
	}

	terms := e.terminals(testSession)
	if len(terms) != 1 {
		t.Fatalf("got %d terminal records, want 1: SIGTERM is a controlled exit", len(terms))
	}
	if terms[0].str("outcome") != "signal" || terms[0].str("reason") != "terminated_by_signal" {
		t.Errorf("terminal is %s/%v, want signal/terminated_by_signal", terms[0].str("outcome"), terms[0].fields["reason"])
	}

	cov := e.coverage(testSession, "call")
	if len(cov) != 1 {
		t.Fatalf("got %d call coverage records, want 1", len(cov))
	}
	assertKeySet(t, cov[0], coverageKeys)
	if cov[0].str("state") != "unverified" || cov[0].str("reason") != "terminated_by_signal" {
		t.Errorf("coverage is %s/%v, want unverified/terminated_by_signal", cov[0].str("state"), cov[0].fields["reason"])
	}
}

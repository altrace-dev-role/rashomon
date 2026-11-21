//go:build darwin || linux

package acceptance

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// ptyKillRounds is how many times the pin takes the kill path. A second
// cmd.Wait on that path races the goroutine already inside Wait and loses or
// wins by a few microseconds, so one round proves little and many rounds
// prove the path.
const ptyKillRounds = 100

// ptyKillDeadline is how long the pin gives the cleanup of one round to
// return. It is the pin's own bound: a hang here must fail this test, never
// run on to the go test alarm, because the mutation sweep reads a test that
// hangs as no verdict.
const ptyKillDeadline = 10 * time.Second

// TestPTYHarness_TheKillPathReapsTheChildOnce takes the harness's kill path
// ptyKillRounds times against a child that never exits on its own: the wait
// bound passes, the child is killed, and the cleanup must return with the
// child reaped and its state read. os/exec sends the result of the copies of
// stdout and stderr once, on a channel that holds one value, and every
// cmd.Wait receives from it; a second Wait in the cleanup, which the harness
// once had, leaves one of the two blocked for good, and the test that hit it
// hung until the go test alarm.
func TestPTYHarness_TheKillPathReapsTheChildOnce(t *testing.T) {
	for i := 0; i < ptyKillRounds; i++ {
		s := openOnPTY(t, exec.Command("sleep", "600"))
		s.bound = 10 * time.Millisecond
		if s.exit() {
			t.Fatalf("round %d: a child that sleeps 600 s exited within %v", i, s.bound)
		}
		reaped := make(chan bool, 1)
		go func() { reaped <- s.kill() }()
		select {
		case ok := <-reaped:
			if !ok {
				t.Fatalf("round %d: the killed child was not reaped within %v", i, ptyReapBound)
			}
		case <-time.After(ptyKillDeadline):
			t.Fatalf("round %d: the cleanup did not return within %v of the kill", i, ptyKillDeadline)
		}
		if s.cmd.ProcessState == nil {
			t.Fatalf("round %d: the reaped child has no process state", i)
		}
		if ws, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
			t.Fatalf("round %d: the child was not ended by the kill: %v", i, s.cmd.ProcessState)
		}
		s.master.Close()
	}
}

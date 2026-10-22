//go:build unix

package hook

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// TestH111_AChildOfAHookGetsTheDefaultSIGPIPE: a hook entry handles SIGPIPE
// so its own writes fail with EPIPE, and a process it starts gets the
// default disposition back. Ignoring the signal survives exec: a shell
// started under SIG_IGN cannot even trap it back, so `kill -PIPE $$` left it
// running. The memory build starts git, the learner and claude -p from hooks.
func TestH111_AChildOfAHookGetsTheDefaultSIGPIPE(t *testing.T) {
	IgnoreBrokenPipe()
	out, err := exec.Command("/bin/sh", "-c", "kill -PIPE $$; echo alive").CombinedOutput()
	if strings.Contains(string(out), "alive") {
		t.Fatal("a child started after IgnoreBrokenPipe survived SIGPIPE: it inherited SIG_IGN")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("child: %v, want death by SIGPIPE", err)
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGPIPE {
		t.Errorf("child ended %v, want killed by SIGPIPE", ee)
	}
}

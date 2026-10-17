//go:build unix

package acceptance

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// withoutTerminal starts cmd in a session of its own, so it has no
// controlling terminal and opening /dev/tty fails, as it does under CI. A
// purge test run from a developer's terminal would otherwise put its prompt
// on that terminal and wait for an answer.
func withoutTerminal(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

// TestTRU28_PurgePrintsThePlanAndRemovesNothingUnasked: with no terminal to ask on and
// no --yes, purge prints the whole plan, removes nothing and exits 1.
func TestTRU28_PurgePrintsThePlanAndRemovesNothingUnasked(t *testing.T) {
	e := newPurgeEnv(t)
	e.session(t)
	before := snapshot(t, e.fakeHome)

	res := e.wait(withoutTerminal(e.command("", nil, "purge")))
	if res.exitCode != 1 {
		t.Fatalf("purge with no terminal and no --yes: exit %d, want 1\nstdout:\n%s", res.exitCode, res.stdout)
	}
	if d := diffSnapshots(before, snapshot(t, e.fakeHome)); len(d) > 0 {
		t.Errorf("an unconfirmed purge changed HOME:\n  %s", strings.Join(d, "\n  "))
	}
	planned := plannedRemovals(t, res.stdout)
	for _, p := range []string{filepath.Join(e.home, "install.json"), filepath.Join(e.home, "install.key"), e.home} {
		if !planned[p] {
			t.Errorf("the plan does not name %s:\n%s", p, res.stdout)
		}
	}
	if !strings.Contains(res.stderr, "--yes") {
		t.Errorf("the refusal does not say how to proceed without a terminal: %q", res.stderr)
	}
}

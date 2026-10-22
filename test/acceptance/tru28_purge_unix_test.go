//go:build unix

package acceptance

import (
	"os"
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

// TestTRU28_PurgeNamesTheCommandForWhatItCannotRemoveAfterTheAnchors: the
// binary sits in a directory the user cannot write, so it cannot be removed
// once the store is gone. A rerun would refuse the root, so purge does not
// tell the user to run it again: it names the command that removes the
// binary, and exits 1.
func TestTRU28_PurgeNamesTheCommandForWhatItCannotRemoveAfterTheAnchors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes from a directory it cannot write")
	}
	e := newPurgeEnv(t)
	bin := copyBinary(t, rashomonBin, filepath.Join(t.TempDir(), "bin", "rashomon"))
	if res := e.runBin(bin, "", "watch", "--yes"); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if err := os.Chmod(filepath.Dir(bin), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Dir(bin), 0o700) }) //nolint:errcheck // TempDir cleanup reports it

	res := e.runBin(bin, "", "purge", "--yes", "--binary")
	if res.exitCode != 1 {
		t.Fatalf("purge --binary of a binary that cannot go: exit %d, want 1\nstdout:\n%s", res.exitCode, res.stdout)
	}
	if want := "rm '" + bin + "'"; !strings.Contains(res.stdout, want) {
		t.Errorf("purge does not name %q for the binary it left:\n%s", want, res.stdout)
	}
	if strings.Contains(res.stderr, "again") {
		t.Errorf("purge tells the user to run it again, which a root without install.json refuses: %q", res.stderr)
	}
	if _, err := os.Stat(e.home); err == nil {
		t.Errorf("the store is still there")
	}
}

//go:build unix

package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestH108_AHangupStillRemovesWhatTheRunLeft is the cleanup under the signal a
// person sends without meaning to. A terminal that closes, or an SSH session
// that drops, sends SIGHUP to the run's process group. dash runs no EXIT trap
// for a signal the script has not trapped, so a hangup in the middle of a run
// would leave the whole scratch behind: the raw streams and debug logs the
// checks read, Claude Code's per-session files and the sessions' transcripts.
func TestH108_AHangupStillRemovesWhatTheRunLeft(t *testing.T) {
	needTools(t, "sh", "python3", "git")
	home := t.TempDir()
	for _, d := range []string{"projects", "session-env"} {
		if err := os.MkdirAll(filepath.Join(home, ".claude", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := filesUnder(t, home)
	stub := t.TempDir()
	writeFile(t, filepath.Join(stub, "claude"), hcStub)
	if err := os.Chmod(filepath.Join(stub, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	logs := t.TempDir()
	stubWrote := filepath.Join(logs, "wrote")
	hung := filepath.Join(logs, "hung")
	tmp := t.TempDir()

	out, err := os.Create(filepath.Join(logs, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	cmd := exec.Command("sh", filepath.Join(moduleRoot, "test", "live", "hc-items.sh"))
	cmd.Dir = moduleRoot
	cmd.Env = hcEnviron(home, stub, tmp, "HC_STUB_LOG="+filepath.Join(logs, "asked"), "HC_STUB_WROTE="+stubWrote, "HC_STUB_HANG="+hung)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	kill := func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); <-done }

	for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(hung); err == nil {
			break
		}
		select {
		case <-done:
			t.Fatal("the script ended before its first session was under way")
		default:
		}
		if time.Now().After(deadline) {
			kill()
			t.Fatal("no session was under way within 60 s")
		}
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP); err != nil {
		kill()
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		kill()
		t.Fatal("the script did not end within 30 s of a hangup")
	}

	b, _ := os.ReadFile(stubWrote)
	wrote := strings.Fields(string(b))
	if len(wrote) == 0 {
		t.Fatal("the session under way when the hangup came had written nothing, so its cleanup is not tested")
	}
	for _, w := range wrote {
		if _, err := os.Lstat(w); !os.IsNotExist(err) {
			t.Errorf("after a hangup, a session's %s is still there", w)
		}
	}
	if left := filesUnder(t, tmp); len(left) != 0 {
		t.Errorf("after a hangup, the run left its scratch in TMPDIR: %v", left)
	}
	if after := filesUnder(t, home); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Errorf("after a hangup, HOME held %v; before the run it held %v", after, before)
	}
}

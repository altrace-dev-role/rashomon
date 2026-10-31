//go:build unix

package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestST_SpilledExecutionRunsNoStep: a red whose execution spilled (the
// ordered stream's lock held past the post's wait) has no seq to be judged
// at, so the step does not run: nothing printed, no steer line (card 2.3 R3).
// The control, the same calls with the lock free, prints the flag.
func TestST_SpilledExecutionRunsNoStep(t *testing.T) {
	control := newST(t)
	if b := control.red("toolu_r").Steer(stEnv(0)); !printsFlag(t, b) {
		t.Fatalf("premise: the control printed %q, want the consent flag", b)
	}

	f := newST(t)
	f.call("toolu_g", "Bash", goTest, 0)
	f.call("toolu_e", "Edit", map[string]any{"file_path": "/tmp/project/internal/a.go", "old_string": "a", "new_string": "b"}, 0)
	// The red's declaration lands first; the lock is taken before its post.
	f.setup = func(p *Post) {
		holdLocks(t, filepath.Join(f.st.RunDir(stSession), store.FileRecords))
		p.lockWaits = perf2Waits
	}
	p := f.call("toolu_r", "Bash", goTest, 1)
	if p.landedSeq != 0 {
		t.Fatalf("premise: the red landed at seq %d, want it spilled", p.landedSeq)
	}
	if b := p.Steer(stEnv(0)); b != nil {
		t.Errorf("Steer after a spilled execution = %q, want nil", b)
	}
	if lines := steerLines(t, f.st); len(lines) != 0 {
		t.Errorf("%d steer lines after a spilled execution, want 0", len(lines))
	}
}

// TestDG2f_AReadErrorStillTakesTheArmWithNoIndex (card D222 review): in a
// project with no index file the stale trigger takes its arm before the run
// is read, as the indexed path does. A read that errors (the records file
// unreadable) prints nothing, but the arm is gone: left set,
// every later relevant write would read the run and discover the spec again
// for the same answer.
func TestDG2f_AReadErrorStillTakesTheArmWithNoIndex(t *testing.T) {
	f := dgFix(t, specOneGate, true)
	f.green()
	p := f.editOf("toolu_e", "internal/a.go")
	if armed, _ := f.st.StaleArmed(stSession); !armed {
		t.Fatal("premise: the counted write left the trigger unarmed before its step")
	}
	records := filepath.Join(f.st.RunDir(stSession), store.FileRecords)
	if err := os.Chmod(records, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(records, 0o600) })
	if _, err := os.ReadFile(records); err == nil {
		t.Skip("the file is readable at mode 0 (running as root)")
	}
	if b := p.Steer(snEnv()); b != nil {
		t.Errorf("Steer with the run unreadable printed %q, want nil", b)
	}
	if got := staleLines(t, f.st); len(got) != 0 {
		t.Errorf("%d stale-evidence lines with the run unreadable, want 0", len(got))
	}
	if armed, _ := f.st.StaleArmed(stSession); armed {
		t.Error("a read error left the trigger armed: the next write would read the run again")
	}
}

//go:build unix

package hook

import (
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

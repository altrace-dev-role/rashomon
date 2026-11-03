package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
)

// TestSN1j_AnArmedWriteWithNoIndexAndNoSpecReadsNoRun (card STALE-NOINDEX1,
// #165 F1): in a project with no learned index, only a spec's required gate
// can raise the stale note (D222), and with no spec file where discovery
// looks there is none, so an armed counted write takes the arm and stops: it
// costs less than one read of the run it would have judged. Before the card
// it read the whole run, discovered no spec and dropped what it read. The
// bound is the run's own read, measured beside it, so it holds on any host.
// Break: read the run whenever the index-less arm is taken.
func TestSN1j_AnArmedWriteWithNoIndexAndNoSpecReadsNoRun(t *testing.T) {
	f := newSN(t)
	f.green()
	for i := range 60 {
		p := f.call(fmt.Sprintf("toolu_l%02d", i), "Bash", map[string]any{"command": "ls"}, 0)
		if err := p.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	e := f.editOf("toolu_e", "internal/a.go")
	env := snEnv()
	read := testing.AllocsPerRun(5, func() {
		if _, err := f.st.ReadRunConsistent(stSession, SteerReadBudget); err != nil {
			t.Fatal(err)
		}
	})
	step := testing.AllocsPerRun(5, func() {
		if err := f.st.ArmStale(stSession); err != nil {
			t.Fatal(err)
		}
		if _, view, ok := e.judge(env); ok || len(view.Findings) != 0 {
			t.Fatalf("judge = %v with %d findings, want false with none", ok, len(view.Findings))
		}
	})
	if step >= read {
		t.Errorf("an armed write with no index and no spec costs %.0f allocs, at least one read of the run (%.0f): it read the run", step, read)
	}
	if armed, _ := f.st.StaleArmed(stSession); armed {
		t.Error("the write left the trigger armed")
	}
}

// TestSN1j2_ASpecFileIsLookedForWhereDiscoveryLooks: specFileAt is true where
// a spec file is at one of discovery's candidates, the repository's or the
// managed one (whose owner discovery checks, and this Lstat does not), and
// false where none is, or with no cwd. Break: drop the managed candidate, or
// read every location as present.
func TestSN1j2_ASpecFileIsLookedForWhereDiscoveryLooks(t *testing.T) {
	s := newSpecTree(t)
	s.mkdir(".git")
	managed := s.mkdir("managed")
	key := baseline.Key(s.base)
	if specFileAt(s.base, key, "", managed) {
		t.Error("no spec file anywhere reads as present")
	}
	if specFileAt("", key, "", managed) {
		t.Error("a call with no cwd reads a spec file as present")
	}
	if err := os.MkdirAll(filepath.Join(managed, "rashomon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managed, "rashomon", "spec.json"), []byte(specOneGate), 0o644); err != nil {
		t.Fatal(err)
	}
	if !specFileAt(s.base, key, "", managed) {
		t.Error("the managed spec file is not seen")
	}
	repo := newSpecTree(t)
	repo.mkdir(".git")
	repo.put(repo.base, specOneGate)
	if !specFileAt(repo.base, baseline.Key(repo.base), "", "") {
		t.Error("the repository's spec file is not seen")
	}
}

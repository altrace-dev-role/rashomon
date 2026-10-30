package purge

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSV1k_PurgeTakesTheFiredMarkers: the hook entries' fired-<Event>
// markers are rashomon's run files, listed before their run directory and
// gone after purge; a user's file named like one but for no event is foreign
// and stays, and so does its run directory. Break: leave the markers out of
// the run-file match, and purge leaves every run directory behind non-empty.
func TestSV1k_PurgeTakesTheFiredMarkers(t *testing.T) {
	root := storeAt(t)
	var ours []string
	for _, e := range store.FiredEvents() {
		ours = append(ours, "runs/s1/"+store.FileFiredPrefix+e)
	}
	ours = append(ours, "runs/s2/"+store.FileFiredPrefix+"Stop")
	mine := "runs/s3/" + store.FileFiredPrefix + "notes.txt"
	for _, f := range append(slices.Clone(ours), mine) {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("1793000000000\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	at := func(f string) int { return slices.Index(p.Files, filepath.Join(root, filepath.FromSlash(f))) }
	for _, f := range ours {
		if at(f) < 0 {
			t.Errorf("the plan does not list %s", f)
		}
	}
	if f, d := at("runs/s2/"+store.FileFiredPrefix+"Stop"), at("runs/s2"); d < 0 || f > d {
		t.Errorf("runs/s2 is not listed after its marker (file %d, directory %d)", f, d)
	}
	abs := filepath.Join(root, filepath.FromSlash(mine))
	if !slices.Contains(p.Foreign, abs) || at(mine) >= 0 {
		t.Errorf("the plan does not name %s as foreign, or lists it as rashomon's", mine)
	}
	p.Execute()
	for _, f := range append(slices.Clone(ours), "runs/s1", "runs/s2") {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			t.Errorf("purge left %s", f)
		}
	}
	if _, err := os.Stat(abs); err != nil {
		t.Errorf("purge removed %s (%v)", mine, err)
	}
}

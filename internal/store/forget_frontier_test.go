package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const savedFrontier = "{\"schema\":1}\n"

// TestForgetWindow_RemovesTheFrontierOfEveryRunItChanges: a frontier is
// folded from its run's records, so a forget that drops lines of a run
// removes the run's frontier and any temporary file a dead save left; a run
// the forget does not change keeps its frontier.
func TestForgetWindow_RemovesTheFrontierOfEveryRunItChanges(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	declareAt(t, st, "sess-in", "toolu_a", 1000)
	declareAt(t, st, "sess-out", "toolu_b", 5000)
	for _, s := range []string{"sess-in", "sess-out"} {
		if err := st.SaveFrontier(s, []byte(savedFrontier)); err != nil {
			t.Fatal(err)
		}
	}
	tmp := filepath.Join(st.RunDir("sess-in"), "."+FileFrontier+".tmp-x")
	if err := os.WriteFile(tmp, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	from, to := time.UnixMilli(500), time.UnixMilli(2000)
	gaps, err := st.ForgetWindow(&from, &to, time.UnixMilli(9000))
	if err != nil || len(gaps) != 1 {
		t.Fatalf("ForgetWindow = %d gaps, %v; want one gap, sess-in's", len(gaps), err)
	}
	for _, p := range []string{filepath.Join(st.RunDir("sess-in"), FileFrontier), tmp} {
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s after a forget that changed its run: %v; want it gone", filepath.Base(p), err)
		}
	}
	if got, ok, err := st.LoadFrontier("sess-out"); err != nil || !ok || string(got) != savedFrontier {
		t.Errorf("the frontier of a run the forget did not change: (%q, %v, %v); want it kept", got, ok, err)
	}
}

// TestForgetHost_RemovesTheFrontierOfARunItChanges: forget --host removes
// the frontier of a run it drops lines from, as a window forget does.
func TestForgetHost_RemovesTheFrontierOfARunItChanges(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	declareAt(t, st, "sess-host", "toolu_a", 1000, "a.example")
	if err := st.SaveFrontier("sess-host", []byte(savedFrontier)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ForgetHost("a.example", time.UnixMilli(9000)); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := st.LoadFrontier("sess-host"); err != nil || ok || got != nil {
		t.Errorf("LoadFrontier after forget --host = (%q, %v, %v); want it absent", got, ok, err)
	}
}

// TestForget_AFrontierThatCannotGoFailsTheRunFirst: when the frontier
// cannot be removed for any reason but its absence, the run fails before
// its gap and before any line goes, so forget never reports a run
// forgotten with its frontier left.
func TestForget_AFrontierThatCannotGoFailsTheRunFirst(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	declareAt(t, st, "sess-stuck", "toolu_a", 1000)
	before := recordLines(t, st, "sess-stuck")
	// A directory that is not empty: os.Remove fails, and not with not-exist.
	if err := os.MkdirAll(filepath.Join(st.RunDir("sess-stuck"), FileFrontier, "held"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ForgetWindow(nil, nil, time.UnixMilli(9000)); !errors.Is(err, ErrForgetIncomplete) {
		t.Fatalf("ForgetWindow with a frontier that cannot go: %v; want ErrForgetIncomplete", err)
	}
	if after := recordLines(t, st, "sess-stuck"); !reflect.DeepEqual(after, before) {
		t.Errorf("records after the failed run: %q; want them unchanged, %q", after, before)
	}
	if gaps, _, err := st.ReadGaps(); err != nil || len(gaps) != 0 {
		t.Errorf("gaps after the failed run: %+v, %v; want none, since the run failed before its gap", gaps, err)
	}
}

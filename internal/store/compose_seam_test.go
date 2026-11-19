package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/fold"
	"github.com/altrace-dev-role/rashomon/internal/graph/session"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

const seamID = "sess"

// seamStore is a store in a new temporary root, and the root.
func seamStore(t *testing.T) (string, *store.Store) {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, st
}

// seamEdits declares seamID's edits of a.go at 1000 and b.go at 5000 under
// the project /w/p: writes the session graph puts in Touched, so the fold of
// both differs from the fold of b.go's alone.
func seamEdits(t *testing.T, st *store.Store) {
	t.Helper()
	project := "/w/p"
	for _, e := range []struct {
		id, path string
		ms       int64
	}{{"toolu_a", "/w/p/a.go", 1000}, {"toolu_b", "/w/p/b.go", 5000}} {
		d := store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion, ToolUseID: e.id,
			SessionID: seamID, RecordedAtMS: e.ms}
		d.FilePath, d.Project, d.CWD = &e.path, &project, &project
		if err := st.AppendDeclaration(d); err != nil {
			t.Fatal(err)
		}
	}
}

// forgetA forgets the window 500 to 2000, a.go's declaration alone.
func forgetA(t *testing.T, st *store.Store) {
	t.Helper()
	from, to := time.UnixMilli(500), time.UnixMilli(2000)
	if _, err := st.ForgetWindow(&from, &to, time.UnixMilli(9000)); err != nil {
		t.Fatal(err)
	}
}

// setSeam sets a compose seam for the test, nil again when it ends.
func setSeam(t *testing.T, seam *func(), fn func()) {
	t.Helper()
	*seam = fn
	t.Cleanup(func() { *seam = nil })
}

// marshalOf is f's canonical bytes.
func marshalOf(t *testing.T, f session.Frontier) string {
	t.Helper()
	b, err := session.MarshalFrontier(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// composed is one Compose of seamID, its frontier marshalled.
func composed(t *testing.T, st *store.Store) string {
	t.Helper()
	f, _, err := session.Compose(st, seamID)
	if err != nil {
		t.Fatal(err)
	}
	return marshalOf(t, f)
}

// foldOfB is the session graph's fold of b.go's declaration alone: what a
// forget of a.go keeps, folded on a store of its own.
func foldOfB(t *testing.T) string {
	t.Helper()
	_, st := seamStore(t)
	seamEdits(t, st)
	forgetA(t, st)
	return composed(t, st)
}

// noFrontier fails the test when seamID's run holds a frontier file or a
// temporary of one.
func noFrontier(t *testing.T, st *store.Store) {
	t.Helper()
	entries, err := os.ReadDir(st.RunDir(seamID))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), store.FileFrontier) {
			t.Errorf("the run holds %s", e.Name())
		}
	}
}

// TestComposeFrontier_ForgetHoldsOffTheSave: a compose that runs while a
// forget holds the records lock -- the frontier removed, the rewrite still
// to come -- saves nothing: its first take misses its budget, so no frontier
// of the lines the forget removes is written. After the forget no frontier
// is left, and the next compose is the fold of what is kept.
func TestComposeFrontier_ForgetHoldsOffTheSave(t *testing.T) {
	root, st := seamStore(t)
	seamEdits(t, st)
	before := composed(t, st)
	if _, ok, err := st.LoadFrontier(seamID); !ok || err != nil {
		t.Fatalf("the first compose saved nothing (ok %v, err %v)", ok, err)
	}
	other, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	other.LimitLockWaits(50 * time.Millisecond)
	ran, saved, inErr := false, false, error(nil)
	setSeam(t, store.AfterFrontierRemoved, func() {
		ran = true
		_, saved, inErr = session.Compose(other, seamID)
	})
	forgetA(t, st)
	*store.AfterFrontierRemoved = nil
	if !ran {
		t.Fatal("the forget never reached its frontier removal")
	}
	if saved || inErr != nil {
		t.Errorf("the compose inside the forget's hold: saved %v, err %v; want neither", saved, inErr)
	}
	noFrontier(t, st)
	if got, want := composed(t, st), foldOfB(t); got != want || got == before {
		t.Errorf("after the forget, not the fold of what is kept:\n got %s\nwant %s", got, want)
	}
}

// TestComposeFrontier_ForgetBetweenTheHoldsSavesNothing: a forget that runs
// whole between a compose's two takes moves G, so the second hold saves
// nothing, though the fold read the lines before the forget; the next
// compose is the fold of what is kept.
func TestComposeFrontier_ForgetBetweenTheHoldsSavesNothing(t *testing.T) {
	_, st := seamStore(t)
	seamEdits(t, st)
	forgot := false
	setSeam(t, store.ComposeBetween, func() {
		if !forgot {
			forgot = true
			forgetA(t, st)
		}
	})
	f, saved, err := session.Compose(st, seamID)
	*store.ComposeBetween = nil
	if !forgot {
		t.Fatal("the seam never ran")
	}
	if saved || err != nil {
		t.Errorf("a compose with a forget between its holds: saved %v, err %v; want neither", saved, err)
	}
	if got := marshalOf(t, f); !strings.Contains(got, "a.go") {
		t.Fatalf("the control: the fold did not read a.go before the forget: %s", got)
	}
	noFrontier(t, st)
	if got, want := composed(t, st), foldOfB(t); got != want {
		t.Errorf("after the forget, not the fold of what is kept:\n got %s\nwant %s", got, want)
	}
}

// TestComposeFrontier_ReadsToTheHeldBounds: the fold reads each run file to
// the size the first hold read, and those sizes are the cursors' SizeSeen
// (record line 1868 (4)): a record and a steer line naming it, appended
// after the hold, are not read, so the saved records cursor stops at the
// held size, a line end, and the steer cursor at 0; the next compose reads
// them as its tail.
func TestComposeFrontier_ReadsToTheHeldBounds(t *testing.T) {
	_, st := seamStore(t)
	seamEdits(t, st)
	records := filepath.Join(st.RunDir(seamID), store.FileRecords)
	held := int64(-1)
	setSeam(t, store.ComposeHeld, func() {
		if held >= 0 {
			return
		}
		info, err := os.Stat(records)
		if err != nil {
			t.Fatal(err)
		}
		held = info.Size()
		project, path := "/w/p", "/w/p/c.go"
		d := store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion, ToolUseID: "toolu_c",
			SessionID: seamID, RecordedAtMS: 6000}
		d.FilePath, d.Project, d.CWD = &path, &project, &project
		if err := st.AppendDeclaration(d); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(filepath.Join(st.RunDir(seamID), store.FileSteer), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close() //nolint:errcheck // the write's error is the one checked
		if _, err := f.WriteString(`{"type":"steer","v":1,"id":"0a6742a0","at_unix_ms":1793897981372,"session_id":"sess","prompt_id":"6f1c2d3e-0000-4000-8000-00000000000a","kind":"unit","key":"example.com/m/internal/a@0a1b2c3d","after_seq":3,"model_text":"some words","user_text":"more words","state":"delivered"}` + "\n"); err != nil {
			t.Fatal(err)
		}
	})
	f, saved, err := session.Compose(st, seamID)
	if err != nil || !saved || held < 0 {
		t.Fatalf("compose: saved %v, err %v, seam ran %v; want saved", saved, err, held >= 0)
	}
	b, ok, err := st.LoadFrontier(seamID)
	if err != nil || !ok {
		t.Fatalf("no frontier file (ok %v, err %v)", ok, err)
	}
	var env struct {
		Records fold.Cursor `json:"records"`
		Steer   fold.Cursor `json:"steer"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	if env.Records.Offset != held || env.Records.SizeSeen != held {
		t.Errorf("records cursor %+v; want offset and size seen %d, the held size", env.Records, held)
	}
	if env.Steer.Offset != 0 || env.Steer.SizeSeen != 0 {
		t.Errorf("steer cursor %+v; want 0, the steer file absent at the hold", env.Steer)
	}
	if got := marshalOf(t, f); strings.Contains(got, "c.go") {
		t.Errorf("the fold read the record appended after the hold: %s", got)
	}
	if got := composed(t, st); !strings.Contains(got, "c.go") {
		t.Errorf("the next compose did not read the appended record: %s", got)
	}
}

// TestComposeFrontier_HeldLockSavesNothing: a compose whose second take
// misses its budget, another handle holding the records lock, saves nothing
// and returns no error: no frontier file and no temporary of one.
func TestComposeFrontier_HeldLockSavesNothing(t *testing.T) {
	root, st := seamStore(t)
	seamEdits(t, st)
	c, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	c.LimitLockWaits(200 * time.Millisecond)
	var release func()
	setSeam(t, store.ComposeBetween, func() { release = store.HoldRecordsLock(t, st, seamID) })
	_, saved, err := session.Compose(c, seamID)
	*store.ComposeBetween = nil
	if release == nil {
		t.Fatal("the seam never ran")
	}
	release()
	if saved || err != nil {
		t.Errorf("a compose whose second take missed its budget: saved %v, err %v; want neither", saved, err)
	}
	noFrontier(t, st)
}

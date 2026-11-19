package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

const composeID = "sess-compose"

// stalePath is a path no test run writes: a frontier that shows it was
// continued from a poisoned file (poison).
const stalePath = "file:stale-e1.go"

// composeStore is a store in a new temporary root.
func composeStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// declareEdit appends composeID's declaration of an edit of rel under the
// project /w/p, recorded at ms: a write the fold puts in Touched, so a
// frontier folded with the line differs from one folded without it.
func declareEdit(t *testing.T, st *store.Store, id, rel string, ms int64) {
	t.Helper()
	path, project := "/w/p/"+rel, "/w/p"
	d := store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion, ToolUseID: id,
		SessionID: composeID, RecordedAtMS: ms}
	d.FilePath, d.Project, d.CWD = &path, &project, &project
	if err := st.AppendDeclaration(d); err != nil {
		t.Fatal(err)
	}
}

// appendBytes writes b to the end of path as it is, the way a torn or
// foreign writer would.
func appendBytes(t *testing.T, path string, b []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // the write's error is the one checked
	if _, err := f.Write(b); err != nil {
		t.Fatal(err)
	}
}

// wholeFold is Fold over every whole line of composeID's two run files as
// they are now: read with os.ReadFile and split at line ends, not through
// RunLinesFrom, and decoded as Compose decodes them.
func wholeFold(t *testing.T, st *store.Store) string {
	t.Helper()
	var evs []Event
	for _, file := range []struct {
		name   string
		decode func([]byte) (Event, bool, error)
	}{{store.FileRecords, RecordEvent}, {store.FileSteer, SteerEvent}} {
		b, err := os.ReadFile(filepath.Join(st.RunDir(composeID), file.name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range bytes.Split(b[:bytes.LastIndexByte(b, '\n')+1], []byte("\n")) {
			if ev, ok, err := file.decode(line); err == nil && ok {
				evs = append(evs, ev)
			}
		}
	}
	return mustMarshal(t, Fold(composeID, evs))
}

// mustCompose is one compose of composeID, its frontier marshalled.
func mustCompose(t *testing.T, st *store.Store) (string, composeStats, bool) {
	t.Helper()
	f, stats, saved, err := compose(st, composeID)
	if err != nil {
		t.Fatal(err)
	}
	return mustMarshal(t, f), stats, saved
}

// savedEnvelope is composeID's frontier file, decoded.
func savedEnvelope(t *testing.T, st *store.Store) (envelope, Frontier) {
	t.Helper()
	b, ok, err := st.LoadFrontier(composeID)
	if err != nil || !ok {
		t.Fatalf("no frontier file (ok %v, err %v)", ok, err)
	}
	env, f, err := parseEnvelope(b)
	if err != nil {
		t.Fatal(err)
	}
	return env, f
}

// poison saves composeID's frontier again with stalePath in Touched, its
// cursors and G kept, then edit's change to the envelope.
func poison(t *testing.T, st *store.Store, edit func(*envelope)) {
	t.Helper()
	env, f := savedEnvelope(t, st)
	f.Touched = append(f.Touched, stalePath)
	var err error
	if env.Frontier, err = MarshalFrontier(f); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(&env)
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveFrontier(composeID, b); err != nil {
		t.Fatal(err)
	}
}

// TestCompose_EqualsTheWholeFold: a compose is Fold over the run's lines
// whether it folds them whole or continues the frontier it saved. The first
// folds whole and saves; a second with no new line continues the frontier by
// no event, its read starting at the saved cursors; a third after one more
// line continues it by that line alone.
func TestCompose_EqualsTheWholeFold(t *testing.T) {
	st := composeStore(t)
	declareEdit(t, st, "toolu_a", "a.go", 1000)
	declareEdit(t, st, "toolu_b", "b.go", 5000)
	for i, want := range []composeStats{{tail: 2, full: true}, {tail: 0}, {tail: 1}} {
		if i == 2 {
			declareEdit(t, st, "toolu_c", "c.go", 6000)
		}
		got, stats, saved := mustCompose(t, st)
		if stats != want || !saved {
			t.Errorf("compose %d: %+v, saved %v; want %+v, saved", i+1, stats, saved, want)
		}
		if whole := wholeFold(t, st); got != whole {
			t.Errorf("compose %d is not the whole fold:\n got %s\nwant %s", i+1, got, whole)
		}
	}
}

// TestCompose_LateSteerLineFoldsWhole: a steer line that lands after the
// frontier passed its after_seq is a Catch miss, which the compose answers
// with the whole fold, never with the frontier continued past a record the
// line sorts before.
func TestCompose_LateSteerLineFoldsWhole(t *testing.T) {
	st := composeStore(t)
	declareEdit(t, st, "toolu_a", "a.go", 1000)
	declareEdit(t, st, "toolu_b", "b.go", 5000)
	mustCompose(t, st)
	_, f := savedEnvelope(t, st)
	late := f.Through.Seq - 1
	if late < 1 || f.Through.Steer {
		t.Fatalf("the frontier is through %+v; want a record past seq 1", f.Through)
	}
	appendBytes(t, filepath.Join(st.RunDir(composeID), store.FileSteer), fmt.Appendf(nil,
		`{"type":"steer","v":1,"id":"0a6742a0","at_unix_ms":1793897981372,"session_id":%q,"prompt_id":"6f1c2d3e-0000-4000-8000-00000000000a","kind":"unit","key":"example.com/m/internal/a@0a1b2c3d","after_seq":%d,"model_text":"some words","user_text":"more words","state":"delivered"}`+"\n",
		composeID, late))
	got, stats, _ := mustCompose(t, st)
	if !stats.full {
		t.Errorf("a steer line at after_seq %d continued a frontier through seq %d: %+v", late, f.Through.Seq, stats)
	}
	if whole := wholeFold(t, st); got != whole {
		t.Errorf("not the whole fold:\n got %s\nwant %s", got, whole)
	}
}

// TestCompose_ForgetRemovesTheFrontier: a forget removes the frontier it
// folded, so none is left, and the next compose is the whole fold of what is
// kept, the forgotten write gone from it.
func TestCompose_ForgetRemovesTheFrontier(t *testing.T) {
	st := composeStore(t)
	declareEdit(t, st, "toolu_a", "a.go", 1000)
	declareEdit(t, st, "toolu_b", "b.go", 5000)
	if before, _, saved := mustCompose(t, st); !saved || !strings.Contains(before, "a.go") {
		t.Fatalf("the first compose: saved %v, frontier %s; want saved, with a.go", saved, before)
	}
	from, to := time.UnixMilli(500), time.UnixMilli(2000)
	if _, err := st.ForgetWindow(&from, &to, time.UnixMilli(9000)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.LoadFrontier(composeID); ok || err != nil {
		t.Errorf("after the forget a frontier is left (ok %v, err %v)", ok, err)
	}
	got, _, _ := mustCompose(t, st)
	if whole := wholeFold(t, st); got != whole || strings.Contains(got, "a.go") {
		t.Errorf("after the forget, not the whole fold of what is kept:\n got %s\nwant %s", got, whole)
	}
}

// TestCompose_MovedGenerationFoldsWhole: a frontier saved under another G is
// never continued. The saved file is poisoned with a path the run never
// wrote, so a continuation shows it: under its own G it is continued, poison
// and all (the control); once a gap of another session moves G the compose
// folds whole. A steer cursor with a generation is refused the same way.
func TestCompose_MovedGenerationFoldsWhole(t *testing.T) {
	st := composeStore(t)
	declareEdit(t, st, "toolu_a", "a.go", 1000)
	declareEdit(t, st, "toolu_b", "b.go", 5000)
	mustCompose(t, st)
	poison(t, st, nil)
	if got, stats, _ := mustCompose(t, st); stats.full || !strings.Contains(got, stalePath) {
		t.Fatalf("the control: under its own G the poisoned frontier was not continued (%+v)", stats)
	}
	if err := st.AppendGap(store.NewGap(store.GapForget, "sess-other", time.Now())); err != nil {
		t.Fatal(err)
	}
	if got, stats, _ := mustCompose(t, st); !stats.full || got != wholeFold(t, st) {
		t.Errorf("G moved, and the compose continued the saved frontier (%+v):\n%s", stats, got)
	}
	poison(t, st, func(e *envelope) { e.Steer.Generation = 1 })
	if got, stats, _ := mustCompose(t, st); !stats.full || got != wholeFold(t, st) {
		t.Errorf("a steer cursor of generation 1 was continued (%+v):\n%s", stats, got)
	}
}

// TestCompose_ReplacedRecordsFoldWhole: a records file that is not the one
// the frontier was read from is not continued, in the two cases its cursors
// see. Removed: the run's files are gone and the frontier is left, so the
// first take finds no records file, and the compose folds what is there and
// saves nothing. Rewritten shorter than the size the save saw: here the
// bytes the frontier read with one path changed, where the save also saw a
// torn line past them, so the records cursor is refused and the compose
// folds the new bytes whole. A file regrown to at least the size seen, with
// a line end at the cursor, is one these cursors do not tell from the old.
func TestCompose_ReplacedRecordsFoldWhole(t *testing.T) {
	t.Run("removed", func(t *testing.T) {
		st := composeStore(t)
		declareEdit(t, st, "toolu_a", "a.go", 1000)
		declareEdit(t, st, "toolu_b", "b.go", 5000)
		mustCompose(t, st)
		for _, name := range []string{store.FileRecords, store.FileSteer} {
			if err := os.Remove(filepath.Join(st.RunDir(composeID), name)); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
		got, stats, saved := mustCompose(t, st)
		if !stats.full || saved || got != wholeFold(t, st) {
			t.Errorf("with no records file: %+v, saved %v, frontier %s; want the whole fold of nothing, not saved", stats, saved, got)
		}
	})
	t.Run("rewritten shorter", func(t *testing.T) {
		st := composeStore(t)
		declareEdit(t, st, "toolu_a", "a.go", 1000)
		declareEdit(t, st, "toolu_b", "b.go", 5000)
		path := filepath.Join(st.RunDir(composeID), store.FileRecords)
		read, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		appendBytes(t, path, []byte(`{"type":"declaration"`))
		mustCompose(t, st)
		if env, _ := savedEnvelope(t, st); env.Records.Offset != int64(len(read)) || env.Records.SizeSeen <= env.Records.Offset {
			t.Fatalf("the save's records cursor is %+v; want offset %d below the size seen", env.Records, len(read))
		}
		rewritten := bytes.Replace(read, []byte("/w/p/a.go"), []byte("/w/p/z.go"), 1)
		if err := os.WriteFile(path, rewritten, 0o600); err != nil {
			t.Fatal(err)
		}
		got, stats, _ := mustCompose(t, st)
		if whole := wholeFold(t, st); !stats.full || got != whole || !strings.Contains(whole, "z.go") {
			t.Errorf("a records file rewritten shorter was continued (%+v):\n got %s\nwant %s", stats, got, whole)
		}
	})
}

// TestParseEnvelope_RefusesAnyOtherFile: only the envelope marshalEnvelope
// writes is a frontier file; a bare frontier, an unknown field, data after
// the object, another schema and an envelope with no frontier are refused.
func TestParseEnvelope_RefusesAnyOtherFile(t *testing.T) {
	good, err := marshalEnvelope(Fold("s", nil), runRead{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseEnvelope(good); err != nil {
		t.Fatalf("the envelope marshalEnvelope wrote: %v", err)
	}
	bare, err := MarshalFrontier(Fold("s", nil))
	if err != nil {
		t.Fatal(err)
	}
	other := strings.Replace(string(good), `{"schema":1,`, `{"schema":2,`, 1)
	if other == string(good) {
		t.Fatalf("the envelope does not open with its schema: %s", good)
	}
	for name, b := range map[string]string{
		"a bare frontier":       string(bare),
		"an unknown field":      `{"extra":1,` + string(good[1:]),
		"data after the object": string(good) + "{}",
		"another schema":        other,
		"no frontier":           `{"schema":1}`,
	} {
		if _, _, err := parseEnvelope([]byte(b)); err == nil {
			t.Errorf("%s was taken: %s", name, b)
		}
	}
}

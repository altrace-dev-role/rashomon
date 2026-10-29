package hook

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// withSteerDisclosure sets ConsentSteerDisclosure for the rest of the test.
func withSteerDisclosure(t *testing.T, f func(string) (int, error)) {
	t.Helper()
	was := ConsentSteerDisclosure
	ConsentSteerDisclosure = f
	t.Cleanup(func() { ConsentSteerDisclosure = was })
}

// readSteerMarker is the steer marker's content, and whether it is there.
func readSteerMarker(t *testing.T, root string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, store.FileSteerDisclosure))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// TestSC4_SessionStartSyncsTheSteerMarker: syncSteerDisclosure sets the
// steer marker to consent.json's answer, writes only on a difference (an
// equal marker is the same file after, not a rewrite of it), removes the
// marker and reports false when consent.json cannot be read, and with no
// reader handed in does nothing.
func TestSC4_SessionStartSyncsTheSteerMarker(t *testing.T) {
	two := func(string) (int, error) { return 2, nil }
	t.Run("no marker", func(t *testing.T) {
		root := t.TempDir()
		withSteerDisclosure(t, two)
		if !syncSteerDisclosure(root) {
			t.Error("sync reported false")
		}
		if got, ok := readSteerMarker(t, root); !ok || got != "2\n" {
			t.Errorf("marker = %q (present %v), want \"2\\n\"", got, ok)
		}
	})
	t.Run("a stale marker", func(t *testing.T) {
		root := t.TempDir()
		if err := store.WriteSteerDisclosureMarker(root, 1); err != nil {
			t.Fatal(err)
		}
		withSteerDisclosure(t, two)
		if !syncSteerDisclosure(root) {
			t.Error("sync reported false")
		}
		if got, _ := readSteerMarker(t, root); got != "2\n" {
			t.Errorf("marker = %q, want \"2\\n\"", got)
		}
	})
	t.Run("an equal marker is left alone", func(t *testing.T) {
		root := t.TempDir()
		if err := store.WriteSteerDisclosureMarker(root, 2); err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(filepath.Join(root, store.FileSteerDisclosure))
		if err != nil {
			t.Fatal(err)
		}
		withSteerDisclosure(t, two)
		if !syncSteerDisclosure(root) {
			t.Error("sync reported false")
		}
		after, err := os.Stat(filepath.Join(root, store.FileSteerDisclosure))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) {
			t.Errorf("an equal marker was written again: the file was replaced")
		}
	})
	t.Run("consent.json unreadable", func(t *testing.T) {
		root := t.TempDir()
		if err := store.WriteSteerDisclosureMarker(root, 2); err != nil {
			t.Fatal(err)
		}
		withSteerDisclosure(t, func(string) (int, error) { return 0, errors.New("consent.json unreadable") })
		if syncSteerDisclosure(root) {
			t.Error("sync reported true with consent.json unreadable")
		}
		if got, ok := readSteerMarker(t, root); ok {
			t.Errorf("the marker stayed at %q, want it removed (0, no consent)", got)
		}
	})
	t.Run("no consent on file", func(t *testing.T) {
		root := t.TempDir()
		withSteerDisclosure(t, func(string) (int, error) { return 0, nil })
		if !syncSteerDisclosure(root) {
			t.Error("sync reported false")
		}
		if got, ok := readSteerMarker(t, root); ok {
			t.Errorf("a store with no consent gained a marker %q; absent already reads 0", got)
		}
	})
	t.Run("no reader handed in", func(t *testing.T) {
		root := t.TempDir()
		withSteerDisclosure(t, nil)
		if !syncSteerDisclosure(root) {
			t.Error("sync reported false with no reader")
		}
		if got, ok := readSteerMarker(t, root); ok {
			t.Errorf("no reader, yet a marker %q", got)
		}
	})
}

// TestSC4b_ASteerSyncFailureSetsNoReason: the SessionStart probe syncs the
// steer marker beside the record-scope one, and a steer sync that fails
// adds no reason to the start coverage record: the record carries what it
// carries with no steer reader at all, and record_scope_unresolved stays
// the record scope's alone. Each sync runs whatever the other did.
func TestSC4b_ASteerSyncFailureSetsNoReason(t *testing.T) {
	now := func() time.Time { return time.UnixMilli(1_760_000_000_000) }
	reasonOf := func(r *string) string {
		if r == nil {
			return "<none>"
		}
		return *r
	}
	probe := func(t *testing.T, scope func(string) (string, error), steer func(string) (int, error)) (string, string, bool) {
		t.Helper()
		st := mustOpen(t)
		was := ConsentRecordScope
		ConsentRecordScope = scope
		t.Cleanup(func() { ConsentRecordScope = was })
		withSteerDisclosure(t, steer)
		const session = "s-sc4b"
		in := strings.NewReader(`{"session_id":"` + session + `","cwd":"/tmp","hook_event_name":"SessionStart","source":"startup"}`)
		RunProbe(&Signals{ch: make(chan os.Signal, 1)}, store.PhaseStart, in, st, now)
		run, err := st.ReadRun(session)
		if err != nil || len(run.Coverage) != 1 {
			t.Fatalf("probe: %d coverage records, %v", len(run.Coverage), err)
		}
		m, ok := readSteerMarker(t, st.Root())
		return reasonOf(run.Coverage[0].Reason), m, ok
	}
	scopeOK := func(string) (string, error) { return store.RecordScopeV5, nil }
	scopeBad := func(string) (string, error) { return "", errors.New("consent.json unreadable") }
	steerOK := func(string) (int, error) { return 2, nil }
	steerBad := func(string) (int, error) { return 0, errors.New("consent.json unreadable") }

	baseline, _, _ := probe(t, scopeOK, nil)
	if baseline == store.ReasonRecordScopeUnresolved {
		t.Fatalf("premise: the baseline start already reads %q", baseline)
	}
	if r, m, ok := probe(t, scopeOK, steerOK); r != baseline || !ok || m != "2\n" {
		t.Errorf("a working steer sync: reason %q, marker %q (present %v); want reason %q and \"2\\n\"", r, m, ok, baseline)
	}
	if r, _, ok := probe(t, scopeOK, steerBad); r != baseline || ok {
		t.Errorf("a failed steer sync: reason %q, marker present %v; want reason %q and no marker", r, ok, baseline)
	}
	if r, m, ok := probe(t, scopeBad, steerOK); r != store.ReasonRecordScopeUnresolved || !ok || m != "2\n" {
		t.Errorf("a failed record-scope sync: reason %q, steer marker %q (present %v); want %q and \"2\\n\"",
			r, m, ok, store.ReasonRecordScopeUnresolved)
	}
}

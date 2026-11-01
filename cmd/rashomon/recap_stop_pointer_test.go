package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// alpha1Index is the project index as v1.2.0-alpha.1 (949d6aca) decodes it,
// key for key (internal/verify/evaluate.go and index.go there), with
// DisallowUnknownFields.
type alpha1Index struct {
	ProjectKey string `json:"project_key"`
	Entries    []struct {
		Family      string `json:"family"`
		Kind        string `json:"kind"`
		Sessions    int    `json:"sessions"`
		LastSession string `json:"last_session"`
		FirstSeen   int64  `json:"first_seen_unix_ms"`
		LastSeen    int64  `json:"last_seen_unix_ms"`
	} `json:"entries"`
}

// stopOnce runs one Stop view over a payload.WriteSession run of 100 calls in
// a fresh store, as BenchmarkRecapStop's op does, and returns the store.
func stopOnce(t *testing.T, now time.Time) *store.Store {
	t.Helper()
	root := t.TempDir()
	if _, err := payload.WriteSession(root, 100, payload.Prompts); err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := digest.ReadRun(st, payload.Session)
	if err != nil {
		t.Fatal(err)
	}
	d, err := digest.BuildFrom(run, st, payload.Session, "", payload.FinalMessage, now)
	if err != nil {
		t.Fatal(err)
	}
	p := recapPayload{SessionID: payload.Session, LastAssistantMessage: payload.FinalMessage,
		Harness: harnessSignal{HookEventName: install.EventStop}, OK: true, CWD: payload.CWD}
	if _, err := stopView(st, run, d, p, false, now); err != nil {
		t.Fatal(err)
	}
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}
	return st
}

// TestIPF1_AnAlpha1BinaryReadsTheIndexAStopWrites (INDEX-POINTER-FILE1, R29):
// alpha.1 reads the project index with DisallowUnknownFields, and its Stop
// writes a fresh index over one it cannot read, so a key this build writes
// there costs a person who goes back to alpha.1 the project's learned gates.
// A Stop that cuts a checkpoint writes an index alpha.1's decoder reads, its
// families intact. Break: note the checkpoint in the index again.
func TestIPF1_AnAlpha1BinaryReadsTheIndexAStopWrites(t *testing.T) {
	st := stopOnce(t, time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC))
	key := baseline.Key(payload.CWD)
	raw, err := os.ReadFile(verify.IndexPath(st.Root(), key))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var old alpha1Index
	if err := dec.Decode(&old); err != nil {
		t.Fatalf("alpha.1's reader refuses the index a Stop writes (%v): %s", err, raw)
	}
	if old.ProjectKey != key || len(old.Entries) == 0 {
		t.Errorf("alpha.1 reads %+v; want the run's families under %q", old, key)
	}
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The prompt path's read of the store's record scope (promptScope in
// preflight.go, W-M2.11 step 1d), through the real prompt entry: at most one
// read a prompt, none on the preflight's early exit, a failed read taken as
// v4, and that one value deciding both the evidence file (writeGraphEvidence)
// and the steer line's record form (preflightNote's digest).

// scopePrompt yields a brief over the graph specimen from services/auth
// (TestGraphEvidence_OnlyAtRecordScopeV5) and names no path.
const scopePrompt = "purging the sessions"

// scopeRun is what one prompt left: each read of the record scope, as the
// value it returned; whether the evidence file was written; the strings in
// the graph line's slots; and the store's key.
type scopeRun struct {
	reads    []string
	evidence bool
	values   map[string]bool
	key      []byte
	lines    []string
}

// promptScopeRun runs text through the real prompt entry in a fresh
// consented store at record scope v5 whose marker mark then sets, from
// services/auth under the indexed graph specimen, or from an empty directory
// when indexed is false, counting readPromptScope's calls.
func promptScopeRun(t *testing.T, indexed bool, text string, mark func(t *testing.T, root string)) scopeRun {
	t.Helper()
	s := newPlanSession(t, store.RecordScopeV5)
	mark(t, s.root)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.project = dir
	if indexed {
		graphProject(t, dir)
		s.project = filepath.Join(dir, "services", "auth")
	}
	var r scopeRun
	was := readPromptScope
	readPromptScope = func(st *store.Store) string {
		v := was(st)
		r.reads = append(r.reads, v)
		return v
	}
	defer func() { readPromptScope = was }()
	s.prompt(text)
	_, err = os.Stat(filepath.Join(s.root, graph.StoreDir, graph.EvidenceFileName(benchSessionID, "prompt-1", time.Time{})))
	r.evidence = err == nil
	st, err := store.OpenExisting(s.root)
	if err != nil {
		t.Fatal(err)
	}
	r.key = st.Key()
	r.values = map[string]bool{}
	for _, l := range s.steerLog() {
		r.lines = append(r.lines, l.Pattern+" "+l.Key)
		if l.Key != keyGraph {
			continue
		}
		for name, raw := range l.Slots {
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatalf("slot %s: %v", name, err)
			}
			slotStrings(r.values, v)
		}
	}
	return r
}

// steerLog is every line of the session's steer log, of any pattern
// (steerLines keeps the plan check's alone).
func (s *planSession) steerLog() []steer.Line {
	s.t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		s.t.Fatal(err)
	}
	raw, _, _ := st.ReadSteer(benchSessionID)
	var out []steer.Line
	for _, b := range raw {
		var l steer.Line
		if err := json.Unmarshal(b, &l); err != nil {
			s.t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

// slotStrings adds every string in a decoded slot value to into.
func slotStrings(into map[string]bool, v any) {
	switch x := v.(type) {
	case string:
		into[x] = true
	case []any:
		for _, e := range x {
			slotStrings(into, e)
		}
	case map[string]any:
		for _, e := range x {
			slotStrings(into, e)
		}
	}
}

// digestsOf counts the strings of v5 whose keyed digest under r's store key
// is among r's: the steer line's record form holds each path as that digest.
func digestsOf(r scopeRun, v5 map[string]bool) int {
	n := 0
	for p := range v5 {
		if r.values[intent.PathDigest(r.key, p)] {
			n++
		}
	}
	return n
}

// TestPromptScope_OneReadDecidesTheFileAndTheLine: an indexed prompt reads
// the record scope once, and that one value decides both the evidence file
// and the steer line's form. At v5 the file is written and the graph line
// holds the message itself; at v4, with no marker, and with a marker that
// cannot be read (RecordScopeMarker's error, taken as v4), there is one read
// of v4, no file, and the graph line holds this store's keyed digests of the
// v5 line's strings, its record form. The v5 run, holding none, is the
// witness that the digest count tells the two forms apart. Break: read the
// scope again for the note's record form, or take the marker's error as v5.
func TestPromptScope_OneReadDecidesTheFileAndTheLine(t *testing.T) {
	v5 := promptScopeRun(t, true, scopePrompt, func(*testing.T, string) {})
	if len(v5.reads) != 1 || v5.reads[0] != store.RecordScopeV5 || !v5.evidence || len(v5.values) == 0 {
		t.Fatalf("v5: reads %q, evidence file %v, %d slot strings (steer lines %q); want one read of v5, the file and a graph line",
			v5.reads, v5.evidence, len(v5.values), v5.lines)
	}
	if n := digestsOf(v5, v5.values); n != 0 {
		t.Errorf("v5: the graph line holds %d keyed digests of its own strings, want the message itself", n)
	}
	for _, c := range []struct {
		name string
		mark func(t *testing.T, root string)
	}{
		{"v4", func(t *testing.T, root string) {
			if err := store.WriteRecordScopeMarker(root, store.RecordScopeV4); err != nil {
				t.Fatal(err)
			}
		}},
		{"no marker", func(t *testing.T, root string) {
			if err := store.RemoveRecordScopeMarker(root); err != nil {
				t.Fatal(err)
			}
		}},
		{"unreadable marker", func(t *testing.T, root string) {
			path := filepath.Join(root, store.FileRecordScope)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := store.RecordScopeMarker(root); err == nil {
				t.Fatal("premise: the marker reads")
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := promptScopeRun(t, true, scopePrompt, c.mark)
			if len(r.reads) != 1 || r.reads[0] != store.RecordScopeV4 {
				t.Errorf("reads %q, want one read of v4", r.reads)
			}
			if r.evidence {
				t.Error("the evidence file was written below v5")
			}
			if n := digestsOf(r, v5.values); n == 0 {
				t.Errorf("the graph line (%d strings) holds no keyed digest of the v5 line's strings: not its record form", len(r.values))
			}
		})
	}
}

// TestPromptScope_NoReadOnTheEarlyExit: from a directory with no index, a
// prompt that names nothing takes the preflight's early exit and reads no
// scope, as the BenchmarkRecapPending/preflight rows do; one that names a
// path goes on to its note and reads it once, as before the read was lazy,
// the witness that the count is on the path. Break: read the scope above
// the early exit.
func TestPromptScope_NoReadOnTheEarlyExit(t *testing.T) {
	for _, c := range []struct {
		name, text string
		want       int
	}{
		{"names nothing", "what does this do", 0},
		{"names a path", "fix internal/graph/g.go", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if r := promptScopeRun(t, false, c.text, func(*testing.T, string) {}); len(r.reads) != c.want {
				t.Errorf("%d reads %q, want %d", len(r.reads), r.reads, c.want)
			}
		})
	}
}

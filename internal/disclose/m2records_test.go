package disclose

import (
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// m2Kinds are the record kinds M2 writes to a run's records.ndjson
// (plan_check, W-M2.6; unitbrief_miss, W-M2.3; conflict_check, W-M2.4), each
// with the words the records row's Holds owes for it besides its name: what
// it keeps in the clear and what only as a keyed digest.
var m2Kinds = []struct {
	kind  string
	words []string
}{
	{store.TypePlanCheck, []string{"the id of each rule that covered a path the plan names, in the clear",
		"keyed digests of those paths, never a path or a word of the plan"}},
	{store.TypeUnitBriefMiss, []string{"a keyed digest of the unit's id",
		"(" + store.UnitBriefAbsent + ", " + store.UnitBriefStale + " or " + store.UnitBriefRefused + ")"}},
	{store.TypeConflictCheck, []string{"its kind and its rule's id in the clear",
		"or keyed digests of the two and no path at v4"}},
}

// m2StoredWords are what the STORED paragraph owes for a rule's id kept in
// the clear, and m2KeptWords what KEPT owes for the record's rows about a
// kept Go edit.
var (
	m2StoredWords = []string{"the rule's id in the clear", "never a word of the plan"}
	m2KeptWords   = []string{"why (absent, stale or refused) and a keyed digest of the unit",
		"each conflict the edit leaves, with its kind, its rule's id"}
)

// m2RecordFaults is each word the records row's Holds, the STORED paragraph
// or the KEPT paragraph lacks.
func m2RecordFaults(holds, stored, kept string) []string {
	var out []string
	for _, k := range m2Kinds {
		for _, w := range append([]string{"a " + k.kind + " with"}, k.words...) {
			if !strings.Contains(holds, w) {
				out = append(out, k.kind+": the records row's Holds does not say "+strconv.Quote(w))
			}
		}
	}
	for _, w := range m2StoredWords {
		if !strings.Contains(stored, w) {
			out = append(out, "the STORED paragraph does not say "+strconv.Quote(w))
		}
	}
	for _, w := range m2KeptWords {
		if !strings.Contains(kept, w) {
			out = append(out, "the KEPT paragraph does not say "+strconv.Quote(w))
		}
	}
	return out
}

// TestM2Records_TheScreenNamesEachKindM2Writes: each record kind M2 writes
// to a run's records.ndjson is named in that file's inventory row with what
// it keeps, the STORED paragraph says a rule's id is kept in the clear, and
// KEPT says what the record keeps about each kept Go edit. A kind added to
// the record without its words fails here. The empty-text witness shows the
// check fails on every word.
func TestM2Records_TheScreenNamesEachKindM2Writes(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var holds, stored, kept string
	rows := 0
	for _, r := range inv.Files {
		if r.Base == store.DirRuns+"/<session>" && r.Pattern == store.FileRecords {
			holds = r.Holds
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("%d inventory rows for a run's records file, want 1", rows)
	}
	for _, p := range inv.Paragraphs {
		switch {
		case strings.HasPrefix(p.Text, StoredHeading+" for each tool call"):
			stored = p.Text
		case strings.HasPrefix(p.Text, "KEPT FROM YOUR SESSIONS"):
			kept = p.Text
		}
	}
	for _, f := range m2RecordFaults(holds, stored, kept) {
		t.Error(f)
	}
	want := len(m2StoredWords) + len(m2KeptWords)
	for _, k := range m2Kinds {
		want += 1 + len(k.words)
	}
	if got := len(m2RecordFaults("", "", "")); got != want {
		t.Errorf("the empty-text witness has %d faults, want %d: a word the check cannot fail on", got, want)
	}
}

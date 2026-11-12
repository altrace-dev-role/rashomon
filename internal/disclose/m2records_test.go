package disclose

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// m2Kinds are the record kinds M2 writes to a run's records.ndjson
// (plan_check, W-M2.6; unitbrief_miss, W-M2.3; conflict_check and its rows,
// W-M2.4). Each maps every json field its struct records to the words of the
// records row's Holds that say what the field keeps, in the clear or only as
// a keyed digest; head is the words that name the kind ("" for a row type).
var m2Kinds = []struct {
	name, head string
	typ        reflect.Type
	fields     map[string]string
}{
	{store.TypePlanCheck, "a " + store.TypePlanCheck + " with", reflect.TypeFor[store.PlanCheck](), map[string]string{
		"landed_seq":   "a plan_check with the seq the plan's call landed",
		"rule_ids":     "the id of each rule that covered a path the plan names, in the clear",
		"named":        "the distinct paths a rule covered",
		"planned":      "how many of them the project did not hold yet",
		"matched":      "the path and rule pairs",
		"path_digests": "keyed digests of those paths, never a path or a word of the plan",
	}},
	{store.TypeUnitBriefMiss, "a " + store.TypeUnitBriefMiss + " with", reflect.TypeFor[store.UnitBriefMiss](), map[string]string{
		"landed_seq": "a unitbrief_miss with the seq the edit landed",
		"unit":       "a keyed digest of the unit's id",
		"reason":     "one word for why (" + store.UnitBriefAbsent + ", " + store.UnitBriefStale + " or " + store.UnitBriefRefused + ")",
	}},
	{store.TypeConflictCheck, "a " + store.TypeConflictCheck + " with", reflect.TypeFor[store.ConflictCheck](), map[string]string{
		"landed_seq": "a conflict_check with the seq the edit landed",
		"commit":     "the graph's commit",
		"file":       "a keyed digest of the edited file",
		"rows":       "for each conflict the edit leaves",
	}},
	{store.TypeConflictCheck + " row", "", reflect.TypeFor[store.ConflictCheckRow](), m2RowFields},
}

// m2RowFields are a conflict_check row's fields and their Holds words; the
// unit_path words carry the v4 form, which keeps no path.
var m2RowFields = map[string]string{
	"kind":      "its kind and its rule's id in the clear",
	"rule_id":   "its kind and its rule's id in the clear",
	"key":       "a keyed digest of its identity",
	"unit":      "its unit and its subject (a symbol or file) as global ids",
	"subject":   "its unit and its subject (a symbol or file) as global ids",
	"unit_path": "with the unit's path at record scope v5, or keyed digests of the two and no path at v4",
}

// m2StoredWords are what the STORED paragraph owes for a rule's id kept in
// the clear, and m2KeptWords what KEPT owes for the record's rows about a
// kept Go edit, the v4 conflict row included.
var (
	m2StoredWords = []string{"the rule's id in the clear", "never a word of the plan"}
	m2KeptWords   = []string{"why (absent, stale or refused) and a keyed digest of the unit",
		"each conflict the edit leaves, with its kind, its rule's id",
		"or keyed digests of the two and no path where the record scope is v4"}
)

// m2SpecNever is what every never-stored sentence says of a spec file (the
// screen's, README's and the plugin description's), and m2RecordNever what
// the records and spill rows' NeverHolds say. A rule's id is a word of the
// spec kept in the clear; where Go edits are kept the conflict base also
// holds the paths, patterns and symbols its rules name
// (cmd/rashomon/conflictbase.go).
const (
	m2SpecNever = "a spec file's text (of a spec, only its digest and its rules' ids, and, where Go edits are kept, " +
		"the paths, patterns and symbols named by the rules each edit is checked against)"
	m2RecordNever = "or a spec file's text, paths or patterns (of a spec, only its digest and its rules' ids)"
)

// m2Fields is each json field name of typ, embedded structs walked, with
// store.EventHeader left out: it is every record's header, not the kind's.
func m2Fields(typ reflect.Type) []string {
	var out []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Anonymous {
			if f.Type != reflect.TypeFor[store.EventHeader]() {
				out = append(out, m2Fields(f.Type)...)
			}
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		out = append(out, name)
	}
	return out
}

// m2FieldFaults is each field of typ with no Holds word mapped, and each
// mapped word holds lacks.
func m2FieldFaults(name string, typ reflect.Type, fields map[string]string, holds string) []string {
	var out []string
	for _, f := range m2Fields(typ) {
		w, ok := fields[f]
		switch {
		case !ok:
			out = append(out, name+": field "+f+" has no Holds word; name it in the records row and map it here")
		case !strings.Contains(holds, w):
			out = append(out, name+": the records row's Holds does not say "+strconv.Quote(w)+" for "+f)
		}
	}
	return out
}

// m2RecordFaults is each word the records row's Holds, the STORED paragraph
// or the KEPT paragraph lacks.
func m2RecordFaults(holds, stored, kept string) []string {
	var out []string
	for _, k := range m2Kinds {
		if k.head != "" && !strings.Contains(holds, k.head) {
			out = append(out, k.name+": the records row's Holds does not name it")
		}
		out = append(out, m2FieldFaults(k.name, k.typ, k.fields, holds)...)
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

// m2NeverFaults is each never-stored text that does not say what of a spec
// is kept.
func m2NeverFaults(never, readme, plugin string, neverHolds []string) []string {
	var out []string
	for where, s := range map[string]string{"the screen's never-stored paragraph": never, "the README": readme,
		"the plugin description": plugin} {
		if !strings.Contains(s, m2SpecNever) {
			out = append(out, where+" does not say "+strconv.Quote(m2SpecNever))
		}
	}
	for i, s := range neverHolds {
		if !strings.Contains(s, m2RecordNever) {
			out = append(out, "NeverHolds "+strconv.Itoa(i)+" does not say "+strconv.Quote(m2RecordNever))
		}
	}
	return out
}

// m2WitnessRow is a conflict row with one field the records row's Holds has
// no word for: the field check must report it, once.
type m2WitnessRow struct {
	store.ConflictCheckRow
	Extra string `json:"extra"`
}

// TestM2Records_TheScreenNamesEachKindM2Writes: every field of each record
// kind M2 writes to a run's records.ndjson has words in that file's inventory
// row, the STORED paragraph says a rule's id is kept in the clear, KEPT says
// what the record keeps about each kept Go edit at v5 and at v4, and each
// never-stored text says what of a spec is kept. A field added to a row with
// no Holds word fails here; the witnesses show each check can fail.
func TestM2Records_TheScreenNamesEachKindM2Writes(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var holds, stored, kept, never string
	var neverHolds []string
	for _, r := range inv.Files {
		if r.Base != store.DirRuns+"/<session>" {
			continue
		}
		switch r.Pattern {
		case store.FileRecords:
			holds = r.Holds
			neverHolds = append(neverHolds, r.NeverHolds)
		case store.FileSpill:
			neverHolds = append(neverHolds, r.NeverHolds)
		}
	}
	if holds == "" || len(neverHolds) != 2 {
		t.Fatalf("the records row's Holds is %q and %d records or spill rows were found, want a Holds and 2", holds, len(neverHolds))
	}
	for _, p := range inv.Paragraphs {
		switch {
		case strings.HasPrefix(p.Text, StoredHeading+" for each tool call"):
			stored = p.Text
		case strings.HasPrefix(p.Text, "KEPT FROM YOUR SESSIONS"):
			kept = p.Text
		case strings.HasPrefix(p.Text, "Never stored:"):
			never = p.Text
		}
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := strings.Join(strings.Fields(string(b)), " ")
	for _, f := range m2RecordFaults(holds, stored, kept) {
		t.Error(f)
	}
	for _, f := range m2NeverFaults(never, readme, PluginDescription, neverHolds) {
		t.Error(f)
	}

	want := len(m2StoredWords) + len(m2KeptWords)
	for _, k := range m2Kinds {
		want += len(m2Fields(k.typ))
		if k.head != "" {
			want++
		}
	}
	if got := len(m2RecordFaults("", "", "")); got != want {
		t.Errorf("the empty-text witness has %d faults, want %d: a word the check cannot fail on", got, want)
	}
	if got := m2FieldFaults("witness", reflect.TypeFor[m2WitnessRow](), m2RowFields, holds); len(got) != 1 || !strings.Contains(got[0], "field extra") {
		t.Errorf("a row with one unmapped field gives %q, want one fault naming it", got)
	}
	if got := len(m2NeverFaults("", "", "", []string{"", ""})); got != 5 {
		t.Errorf("the empty never-stored witness has %d faults, want 5", got)
	}
}

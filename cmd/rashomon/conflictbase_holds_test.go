package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// What the conflict base keeps (W-M2.11 step 1d): the unit-brief row's
// Holds names each field the base file stores, to the leaf, and the fields
// the base keeps empty are empty in a base the real writer stores.

// cbLeafTypes are the struct types the base keeps whole, one leaf each.
var cbLeafTypes = []reflect.Type{reflect.TypeFor[graph.Rev](), reflect.TypeFor[graph.Prov]()}

// cbLeaves is each leaf of typ as its JSON spells it, under prefix: an
// embedded struct with no name adds none, a pointer or a slice is its
// element, a struct is its fields unless cbLeafTypes keeps it whole, and
// anything else (a string, a number, a map) is one leaf.
func cbLeaves(typ reflect.Type, prefix string) []string {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || slices.Contains(cbLeafTypes, typ) {
		return []string{prefix}
	}
	var out []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() && !f.Anonymous {
			continue
		}
		tag := f.Tag
		name, _, _ := strings.Cut(tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			out = append(out, cbLeaves(f.Type, prefix)...)
			continue
		}
		if name == "" {
			name = f.Name
		}
		if prefix != "" {
			name = prefix + "." + name
		}
		out = append(out, cbLeaves(f.Type, name)...)
	}
	return out
}

// cbHolds is the Holds words for each leaf of the base file, "" for a leaf
// the base keeps empty (cbEmpty).
var cbHolds = map[string]string{
	"v":                    "its format's version",
	"commit":               "the index's commit and repository",
	"repo":                 "the index's commit and repository",
	"rules_digest":         "a digest of the rules",
	"rules.ID":             "each rule's id and kind",
	"rules.Kind":           "each rule's id and kind",
	"rules.From":           "the unit prefixes a boundary runs from and into",
	"rules.To":             "the unit prefixes a boundary runs from and into",
	"rules.Predicate":      "for a recorded decision, which of its two checks it makes",
	"rules.Paths":          "its path patterns",
	"rules.AppliesTo":      "the ids of the files, units and symbols it applies to",
	"rules.Source":         "",
	"records.Node.ID":      "with the symbols and files they run through, each by its ids",
	"records.Node.Kind":    "its kind or predicate",
	"records.Node.Attrs":   "",
	"records.Node.Tier":    "its tier, confidence and status",
	"records.Node.Conf":    "its tier, confidence and status",
	"records.Node.Status":  "its tier, confidence and status",
	"records.Node.Rev":     "the commit it belongs to",
	"records.Node.Prov":    "",
	"records.Edge.Src":     "with the symbols and files they run through, each by its ids",
	"records.Edge.Pred":    "its kind or predicate",
	"records.Edge.Dst":     "with the symbols and files they run through, each by its ids",
	"records.Edge.Attrs":   "",
	"records.Edge.Tier":    "its tier, confidence and status",
	"records.Edge.Conf":    "its tier, confidence and status",
	"records.Edge.Status":  "its tier, confidence and status",
	"records.Edge.Rev":     "the commit it belongs to",
	"records.Edge.Prov":    "",
	"records.Edge.Certain": "",
	"digests":              "the content digests of the files a rule guards",
}

// cbEmpty is the leaves the base keeps empty, sorted: what an index keeps
// of a record beyond its identity and standing, and a rule's source.
var cbEmpty = []string{
	"records.Edge.Attrs", "records.Edge.Certain", "records.Edge.Prov",
	"records.Node.Attrs", "records.Node.Prov", "rules.Source",
}

// cbHoldsText is the unit-brief row's Holds from "the conflict base holds"
// on, as disclose.Default gives it.
func cbHoldsText(t *testing.T) string {
	t.Helper()
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, f := range inv.Files {
		if f.Base == "graph/unitbrief/<session>" && f.Pattern == "x-*.json" {
			rows = append(rows, f.Holds)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("%d unit-brief rows, want 1", len(rows))
	}
	_, after, ok := strings.Cut(rows[0], "the conflict base holds")
	if !ok {
		t.Fatalf("the unit-brief row's Holds names no conflict base: %q", rows[0])
	}
	return after
}

// cbHoldsFaults is each leaf of typ that cbHolds does not name, and each
// whose words holds lacks.
func cbHoldsFaults(typ reflect.Type, holds string) []string {
	var out []string
	for _, leaf := range cbLeaves(typ, "") {
		words, ok := cbHolds[leaf]
		switch {
		case !ok:
			out = append(out, "field "+leaf+" is stored and named by no Holds words")
		case words != "" && !strings.Contains(holds, words):
			out = append(out, "field "+leaf+": the Holds lacks "+strconv.Quote(words))
		}
	}
	return out
}

// cbWitnessBase is the base file with one field more.
type cbWitnessBase struct {
	conflictBase
	Extra string `json:"extra"`
}

// TestConflictBase_HoldsNamesEachStoredField: every leaf of the base file
// (conflictBaseHead, conflictBaseBody, engine.Rule, graph.Record's Node and
// Edge, with Rev, Prov and each map taken whole) has its words in the
// unit-brief row's Holds, or is one the base keeps empty. Witnesses: with
// no Holds text every named leaf faults, and one field more gives exactly
// one fault, naming it.
func TestConflictBase_HoldsNamesEachStoredField(t *testing.T) {
	holds := cbHoldsText(t)
	typ := reflect.TypeFor[conflictBase]()
	if f := cbHoldsFaults(typ, holds); len(f) != 0 {
		t.Errorf("the Holds does not name what the base stores:\n%s", strings.Join(f, "\n"))
	}
	leaves := cbLeaves(typ, "")
	if len(leaves) != len(cbHolds) {
		t.Errorf("%d leaves, %d named: %v", len(leaves), len(cbHolds), leaves)
	}
	var empty []string
	named := 0
	for leaf, words := range cbHolds {
		if !slices.Contains(leaves, leaf) {
			t.Errorf("cbHolds names %s, which the base does not store", leaf)
		}
		if words == "" {
			empty = append(empty, leaf)
		} else {
			named++
		}
	}
	slices.Sort(empty)
	if !slices.Equal(empty, cbEmpty) {
		t.Errorf("leaves kept empty %v, want %v", empty, cbEmpty)
	}
	if n := len(cbHoldsFaults(typ, "")); n != named {
		t.Errorf("witness: %d faults against no Holds text, want %d", n, named)
	}
	if f := cbHoldsFaults(reflect.TypeFor[cbWitnessBase](), holds); len(f) != 1 || !strings.HasPrefix(f[0], "field extra ") {
		t.Errorf("witness: one field more gives faults %q, want exactly one naming extra", f)
	}
}

// TestConflictBase_StoredRulesKeepADecisionsCheck: storedRules drops every
// rule's Source and every Predicate but a decision rule's closed one, and
// leaves the rules it is given as they were.
func TestConflictBase_StoredRulesKeepADecisionsCheck(t *testing.T) {
	mk := func() []engine.Rule {
		return []engine.Rule{
			{ID: "d1", Kind: string(spec.Decision), Predicate: spec.PredicatePathsUntouched, Paths: []string{"keep/*.go"}, Source: "kn-d1"},
			{ID: "d2", Kind: string(spec.Decision), Predicate: "free words of a decision", Source: "kn-d2"},
			{ID: "b1", Kind: string(spec.Boundary), Predicate: spec.PredicateSymbolPresent,
				From: []string{"unit:go:a"}, To: []string{"unit:go:b"}, Source: "words of a spec"},
		}
	}
	in := mk()
	got := storedRules(in)
	want := mk()
	for i := range want {
		want[i].Source = ""
	}
	want[1].Predicate, want[2].Predicate = "", ""
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stored rules %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(in, mk()) {
		t.Errorf("storedRules changed its input: %+v", in)
	}
}

// cbwProv is where an extractor read the written-base fixture's records.
var cbwProv = graph.Prov{Extractor: "pylite", Version: "v1", Evidence: []string{"cbw-evidence-word"}}

// cbJSONPaths adds each key path of v, a decoded JSON value, under prefix
// to out, stopping at a path cbHolds names (a map, Rev or Prov whole).
func cbJSONPaths(v any, prefix string, out map[string]bool) {
	if _, ok := cbHolds[prefix]; ok {
		out[prefix] = true
		return
	}
	switch x := v.(type) {
	case nil:
	case map[string]any:
		for k, e := range x {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			cbJSONPaths(e, p, out)
		}
	case []any:
		for _, e := range x {
			cbJSONPaths(e, prefix, out)
		}
	default:
		out[prefix] = true
	}
}

// TestConflictBase_WrittenBaseKeepsNoTextOfARule: the real writer, over an
// index whose crossing call carries a signature, a keyword's value, its
// provenance and Certain, under a trusted spec rule whose source and
// predicate are its author's words, stores none of them; every key path of
// the file is one cbHolds names. Witness: the index and the rule hold each.
func TestConflictBase_WrittenBaseKeepsNoTextOfARule(t *testing.T) {
	root := t.TempDir()
	if err := store.WriteSessionContentMarker(root); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	rev := graph.RevCommit("c1")
	s := graph.NewStore()
	ua, ub := graph.UnitID("py", "svc/a"), graph.UnitID("py", "svc/b")
	fa, fb := graph.FileID("svc/a/x.py"), graph.FileID("svc/b/y.py")
	sa, sb := graph.SymID("py", "svc/a", "func", "x.call_b"), graph.SymID("py", "svc/b", "func", "y.target")
	for _, n := range []graph.Node{
		{ID: ua, Kind: "unit", Rev: rev}, {ID: ub, Kind: "unit", Rev: rev},
		{ID: fa, Kind: "file", Rev: rev}, {ID: fb, Kind: "file", Rev: rev},
		{ID: sa, Kind: "sym", Attrs: map[string]string{"symkind": "func", "sig": "(cbw_sig_word)"}, Prov: cbwProv, Rev: rev},
		{ID: sb, Kind: "sym", Attrs: map[string]string{"symkind": "func"}, Rev: rev},
	} {
		s.AssertNode(n)
	}
	for _, e := range []graph.Edge{
		{Src: ua, Pred: graph.Contains, Dst: fa, Rev: rev}, {Src: ub, Pred: graph.Contains, Dst: fb, Rev: rev},
		{Src: fa, Pred: graph.Declares, Dst: sa, Rev: rev}, {Src: fb, Pred: graph.Declares, Dst: sb, Rev: rev},
		{Src: sa, Pred: graph.Calls, Dst: sb, Tier: graph.Algorithmic, Conf: 1, Status: graph.Asserted,
			Attrs: map[string]string{"argc": "1", "kw.token": "cbw-kw-word"}, Prov: cbwProv, Rev: rev},
	} {
		s.AssertEdge(e)
	}
	rule := spec.Rule{ID: "b1", Kind: string(spec.Boundary), Why: "w", HardControl: "h",
		From: []string{string(ua)}, To: []string{string(ub)}, Predicate: "cbw predicate words", Source: "cbw source words"}
	sp := spec1(spec.Trusted, rule)
	writeConflictBase(root, "s1", project, planRID, s, rev, func() *spec.Compiled { return sp })

	path := conflictBasePath(root, "s1", project)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the writer stored no base: %v", err)
	}
	for _, w := range []string{"cbw_sig_word", "cbw-kw-word", "cbw-evidence-word", "cbw predicate words", "cbw source words", "pylite"} {
		if bytes.Contains(data, []byte(w)) {
			t.Errorf("the base holds %q", w)
		}
	}
	cb, ok := readConflictBase(path)
	if !ok || len(cb.Rules) != 1 || len(cb.Records) == 0 {
		t.Fatalf("base: ok %v, %d rules, %d records; want the rule and the crossing call", ok, len(cb.Rules), len(cb.Records))
	}
	if r := cb.Rules[0]; r.Source != "" || r.Predicate != "" {
		t.Errorf("the base keeps rule %+v, want no source or predicate", r)
	}
	calls := 0
	for _, rec := range cb.Records {
		if n := rec.Node; n != nil && (n.Attrs != nil || !reflect.DeepEqual(n.Prov, graph.Prov{})) {
			t.Errorf("the base keeps node %s with attrs %v and prov %+v", n.ID, n.Attrs, n.Prov)
		}
		if e := rec.Edge; e != nil {
			if e.Attrs != nil || !reflect.DeepEqual(e.Prov, graph.Prov{}) || e.Certain {
				t.Errorf("the base keeps edge %s %s %s with attrs %v, prov %+v, certain %v", e.Src, e.Pred, e.Dst, e.Attrs, e.Prov, e.Certain)
			}
			if e.Pred == graph.Calls {
				calls++
			}
		}
	}
	if calls != 1 {
		t.Errorf("%d calls in the base, want the crossing one", calls)
	}
	paths := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'}) {
		var v any
		if err := json.Unmarshal(line, &v); err != nil {
			t.Fatalf("a base line does not read back: %v", err)
		}
		cbJSONPaths(v, "", paths)
	}
	for p := range paths {
		if _, ok := cbHolds[p]; !ok {
			t.Errorf("the written base stores %s, which cbHolds does not name", p)
		}
	}
	if len(paths) < len(cbHolds)-len(cbEmpty) {
		t.Errorf("witness: the written base has %d key paths, want at least the %d named ones", len(paths), len(cbHolds)-len(cbEmpty))
	}

	idx := s.Out(sa, rev, graph.Calls)
	if len(idx) != 1 || !idx[0].Certain || idx[0].Attrs["kw.token"] == "" || len(idx[0].Prov.Evidence) == 0 {
		t.Errorf("witness: the index's call %+v carries no Certain, keyword value or provenance", idx)
	}
	if n, ok := s.Node(sa, rev); !ok || n.Attrs["sig"] == "" {
		t.Errorf("witness: the index's caller %+v has no signature", n)
	}
	if r := sp.Rules[0].Rule; r.Source == "" || r.Predicate == "" {
		t.Errorf("witness: the spec rule %+v has no source or predicate", r)
	}
}

// TestConflictBase_AVersion1BaseIsRefusedAndRebuilt: a base the prompt
// wrote, its version set back to 1, is refused by both readers, so the post
// checks the crossing edit against nothing and records no check; the next
// prompt writes the base again at conflictBaseVersion, and the same edit
// then raises the boundary note (the witness).
func TestConflictBase_AVersion1BaseIsRefusedAndRebuilt(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	if cb, ok := readConflictBase(s.basePath()); !ok || cb.V != conflictBaseVersion {
		t.Fatalf("precondition: base ok %v at version %d, want %d", ok, cb.V, conflictBaseVersion)
	}
	data, err := os.ReadFile(s.basePath())
	if err != nil {
		t.Fatal(err)
	}
	cur := []byte(`{"v":` + strconv.Itoa(conflictBaseVersion) + `,`)
	if n := bytes.Count(data, cur); n != 1 {
		t.Fatalf("the base holds %q %d times, want once", cur, n)
	}
	if err := os.WriteFile(s.basePath(), bytes.Replace(data, cur, []byte(`{"v":1,`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readConflictBaseHead(s.basePath()); ok {
		t.Error("readConflictBaseHead reads a version 1 base")
	}
	if _, ok := readConflictBase(s.basePath()); ok {
		t.Error("readConflictBase reads a version 1 base")
	}
	if out := s.land("toolu_cfv1", cfGraph, cfCross); strings.Contains(out, cfNote) {
		t.Errorf("against a version 1 base the post says %q", out)
	}
	if cs := s.run().ConflictChecks; len(cs) != 0 {
		t.Errorf("against a version 1 base: checks %+v, want none", cs)
	}

	s.prompt("fix " + cfGraph)
	if head, ok := readConflictBaseHead(s.basePath()); !ok || head.V != conflictBaseVersion {
		t.Errorf("after the next prompt: head %+v ok %v, want version %d", head, ok, conflictBaseVersion)
	}
	if out := s.land("toolu_cfv2", cfGraph, cfCross+"\nvar _ = 0\n"); !strings.Contains(out, cfNote) {
		t.Errorf("witness: against the rebuilt base the post says %q, want %q", out, cfNote)
	}
}

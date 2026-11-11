package engine

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// onlyKind is the conflicts of kind k, in their order.
func onlyKind(cs []Conflict, k ConflictKind) []Conflict {
	var out []Conflict
	for _, c := range cs {
		if c.Kind == k {
			out = append(out, c)
		}
	}
	return out
}

// cstr is a conflict list as a failure prints it.
func cstr(cs []Conflict) string {
	var b strings.Builder
	b.WriteString("[")
	for _, c := range cs {
		b.WriteString(string(c.Kind) + " rule=" + c.RuleID + " unit=" + string(c.Unit) + " subject=" + string(c.Subject))
		if c.Edge.Src != "" {
			b.WriteString(" edge=" + string(c.Edge.Src) + " " + string(c.Edge.Pred) + " " + string(c.Edge.Dst))
		}
		b.WriteString("; ")
	}
	return b.String() + "]"
}

// bfx is the boundary fixture at C1: internal/graph and its subunit
// internal/graph/engine, the near miss internal/graphx, and
// internal/hookio, one file each. G0 calls E0 inside the prefix and X0
// calls H1 from outside it, so the tree as committed crosses nothing.
type bfx struct {
	f                          *fx
	fileG, fileE, fileX, fileH graph.ID
	g0, e0, e1, x0, h1         graph.ID
}

func newBoundaryFx() bfx {
	f := newFx()
	b := bfx{f: f}
	b.fileG = f.file("internal/graph", "internal/graph/g.lx", fxC1)
	b.fileE = f.file("internal/graph/engine", "internal/graph/engine/e.lx", fxC1)
	b.fileX = f.file("internal/graphx", "internal/graphx/x.lx", fxC1)
	b.fileH = f.file("internal/hookio", "internal/hookio/h.lx", fxC1)
	b.g0 = f.sym(b.fileG, "internal/graph", "func", "G0", fxC1, nil)
	b.e0 = f.sym(b.fileE, "internal/graph/engine", "func", "E0", fxC1, nil)
	b.e1 = f.sym(b.fileE, "internal/graph/engine", "func", "E1", fxC1, nil)
	b.x0 = f.sym(b.fileX, "internal/graphx", "func", "X0", fxC1, nil)
	b.h1 = f.sym(b.fileH, "internal/hookio", "func", "H1", fxC1, nil)
	f.edge(b.g0, graph.Calls, b.e0, fxC1, nil)
	f.edge(b.x0, graph.Calls, b.h1, fxC1, nil)
	return b
}

func (b bfx) files() []graph.ID { return []graph.ID{b.fileG, b.fileE, b.fileX, b.fileH} }

func boundaryRule(id, from, to string) Rule {
	return Rule{ID: id, Kind: "boundary", From: []string{from}, To: []string{to}}
}

var (
	graphU = string(fxUnit("internal/graph"))
	hookU  = string(fxUnit("internal/hookio"))
)

// TestConflicts_BoundaryCrossOnNewEdgeOnly: AC-M2.1's three reads on a
// fixture (the live twin is TestConflicts_NoneOnThisRepositoryAtHead):
// nothing on the empty diff, nothing with every file's records against an
// empty base, one on an overlay that adds a call from a unit under
// internal/graph into internal/hookio. Then: a crossing that exists at the
// base and only changed is not new, a new one beside it is; References,
// MayCall and Imports cross, ResolvesTo does not; a file's own edge crosses
// from the unit that contains the file.
func TestConflicts_BoundaryCrossOnNewEdgeOnly(t *testing.T) {
	rules := []Rule{boundaryRule("b1", graphU, hookU)}

	b := newBoundaryFx()
	if got := Conflicts(b.f.s, Diff{Rev: fxC1}, rules); len(got) != 0 {
		t.Errorf("empty diff: %s, want none", cstr(got))
	}
	all := DiffOf(b.f.s, graph.RevCommit("none"), fxC1, b.files())
	if len(all.Decls) == 0 || len(all.Removed) != 0 {
		t.Fatalf("every file against an empty base: Decls %d Removed %d", len(all.Decls), len(all.Removed))
	}
	if got := onlyKind(Conflicts(b.f.s, all, rules), ConflictBoundaryCross); len(got) != 0 {
		t.Errorf("the tree as committed: %s, want no boundary_cross", cstr(got))
	}

	ov := fxOv(1)
	b.f.edge(b.e0, graph.Calls, b.h1, ov, map[string]string{"argc": "1"})
	got := Conflicts(b.f.s, DiffOf(b.f.s, fxC1, ov, []graph.ID{b.fileE}), rules)
	if len(got) != 1 {
		t.Fatalf("one new call into hookio: %s, want one", cstr(got))
	}
	c := got[0]
	if c.Edge.Src != b.e0 || c.Edge.Pred != graph.Calls || c.Edge.Dst != b.h1 {
		t.Errorf("Edge = %+v, want E0 calls H1", c.Edge)
	}
	c.Edge = graph.Edge{}
	want := Conflict{Kind: ConflictBoundaryCross, Rev: ov, Unit: fxUnit("internal/graph/engine"), RuleID: "b1",
		Subject: b.e0, Evidence: []graph.ID{b.e0, b.h1}}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("conflict = %+v\nwant %+v", c, want)
	}

	// A crossing at the base whose call changed in place (its argc) is in
	// neither list, identity deciding (P2 ruling 3): changed, not new. The
	// new one beside it fires alone.
	b = newBoundaryFx()
	b.f.edge(b.e0, graph.Calls, b.h1, fxC1, map[string]string{"argc": "1"})
	b.f.edge(b.e0, graph.Calls, b.h1, ov, map[string]string{"argc": "2"})
	b.f.edge(b.e1, graph.Calls, b.h1, ov, nil)
	d := DiffOf(b.f.s, fxC1, ov, []graph.ID{b.fileE})
	if l := "e " + string(b.e0) + " calls " + string(b.h1); slices.Contains(labels(d.Decls), l) || slices.Contains(labels(d.Removed), l) {
		t.Fatalf("precondition: the changed call is in the diff: Decls %q Removed %q", labels(d.Decls), labels(d.Removed))
	}
	if got := Conflicts(b.f.s, d, rules); len(got) != 1 || got[0].Subject != b.e1 {
		t.Errorf("a changed crossing and a new one: %s, want E1 alone", cstr(got))
	}

	for _, tc := range []struct {
		p    graph.Pred
		want int
	}{{graph.References, 1}, {graph.MayCall, 1}, {graph.Imports, 1}, {graph.ResolvesTo, 0}} {
		b = newBoundaryFx()
		b.f.edge(b.e1, tc.p, b.h1, ov, nil)
		got := Conflicts(b.f.s, DiffOf(b.f.s, fxC1, ov, []graph.ID{b.fileE}), rules)
		if len(got) != tc.want {
			t.Errorf("a new %s edge into hookio: %s, want %d", tc.p, cstr(got), tc.want)
		}
	}

	b = newBoundaryFx()
	b.f.edge(b.fileE, graph.References, b.h1, ov, nil)
	got = Conflicts(b.f.s, DiffOf(b.f.s, fxC1, ov, []graph.ID{b.fileE}), rules)
	if len(got) != 1 || got[0].Subject != b.fileE || got[0].Unit != fxUnit("internal/graph/engine") {
		t.Errorf("a file's new reference into hookio: %s, want the file, in its unit", cstr(got))
	}
}

// TestConflicts_BoundaryPrefixNeverRegexp: a From or To is a unit id and
// matches that unit and the units under it, by segment: never a regexp
// (RES-2), never a bare string prefix.
func TestConflicts_BoundaryPrefixNeverRegexp(t *testing.T) {
	b := newBoundaryFx()
	ov := fxOv(1)
	b.f.edge(b.g0, graph.Calls, b.h1, ov, nil)
	b.f.edge(b.e0, graph.Calls, b.h1, ov, nil)
	b.f.edge(b.x0, graph.MayCall, b.h1, ov, nil)
	d := DiffOf(b.f.s, fxC1, ov, []graph.ID{b.fileG, b.fileE, b.fileX})

	for _, tc := range []struct {
		name, from, to string
		want           []graph.ID
	}{
		{"the unit and its subunits, never graphx", graphU, hookU, []graph.ID{b.g0, b.e0}},
		{"a dot is not a wildcard", "unit:lx:internal/gr.ph", hookU, nil},
		{"a star is not a wildcard", "unit:lx:internal/graph.*", hookU, nil},
		{"a prefix stops at a segment", "unit:lx:internal/gr", hookU, nil},
		{"hook does not reach hookio", graphU, "unit:lx:internal/hook", nil},
		{"graphx is its own unit", string(fxUnit("internal/graphx")), hookU, []graph.ID{b.x0}},
	} {
		var subjects []graph.ID
		for _, c := range Conflicts(b.f.s, d, []Rule{boundaryRule("p", tc.from, tc.to)}) {
			subjects = append(subjects, c.Subject)
		}
		if !slices.Equal(subjects, tc.want) {
			t.Errorf("%s: subjects %q, want %q", tc.name, subjects, tc.want)
		}
	}
}

// TestConflicts_DecisionPathsUntouched: a paths_untouched decision fires
// once per AppliesTo id the diff touches, a file in Files or a unit in
// Touched, and not for a diff elsewhere; an unknown predicate fires
// nothing.
func TestConflicts_DecisionPathsUntouched(t *testing.T) {
	f := newFx()
	fa := f.file("a", "a/one.lx", fxC1)
	fb := f.file("b", "b/two.lx", fxC1)
	fc := f.file("c", "c/three.lx", fxC1)
	ov := fxOv(1)
	rule := Rule{ID: "d1", Kind: "decision", Predicate: "paths_untouched", AppliesTo: []graph.ID{fa, fxUnit("b")}}

	got := onlyKind(Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fb, fa}), []Rule{rule}), ConflictDecisionContradicted)
	want := []Conflict{
		{Kind: ConflictDecisionContradicted, Rev: ov, Unit: fxUnit("a"), RuleID: "d1", Subject: fa, Evidence: []graph.ID{fa}},
		{Kind: ConflictDecisionContradicted, Rev: ov, Unit: fxUnit("b"), RuleID: "d1", Subject: fxUnit("b"), Evidence: []graph.ID{fxUnit("b")}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a file and a unit the rule names: %s\nwant %s", cstr(got), cstr(want))
	}
	if got := onlyKind(Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fc}), []Rule{rule}), ConflictDecisionContradicted); len(got) != 0 {
		t.Errorf("a diff in another unit: %s, want none", cstr(got))
	}
	rule.Predicate = "no_such_predicate"
	if got := Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fa}), []Rule{rule}); len(got) != 0 {
		t.Errorf("an unknown predicate: %s, want none", cstr(got))
	}
}

// symbolPresentFx is the symbol_present fixture, C1 against cur: unit a's
// file with S0 and lib's file with L at both (S0's doc changed at cur);
// at cur alone S1 with no edge to L, S2 calling L, a method T.S3 that may
// call it, S4 referencing it and a constant K. At a commit cur every
// record is restated there, as an index writes it; at an overlay cur the
// added ones are overlay records.
func symbolPresentFx(cur graph.Rev) (*graph.Store, Diff, graph.ID, graph.ID) {
	f := newFx()
	tree := func(r graph.Rev, doc string) (graph.ID, graph.ID) {
		fa := f.file("a", "a/one.lx", r)
		f.sym(fa, "a", "func", "S0", r, map[string]string{"doc": doc})
		return fa, f.sym(f.file("lib", "lib/l.lx", r), "lib", "func", "L", r, nil)
	}
	fa, l := tree(fxC1, "old")
	if cur.Kind == graph.RevKindCommit {
		tree(cur, "new")
	} else {
		f.sym(fa, "a", "func", "S0", cur, map[string]string{"doc": "new"})
	}
	s1 := f.sym(fa, "a", "func", "S1", cur, nil)
	s2 := f.sym(fa, "a", "func", "S2", cur, nil)
	s3 := f.sym(fa, "a", "method", "T.S3", cur, nil)
	s4 := f.sym(fa, "a", "func", "S4", cur, nil)
	f.edge(s2, graph.Calls, l, cur, nil)
	f.edge(s3, graph.MayCall, l, cur, nil)
	f.edge(s4, graph.References, l, cur, nil)
	f.sym(fa, "a", "const", "K", cur, nil)
	return f.s, DiffOf(f.s, fxC1, cur, []graph.ID{fa}), s1, l
}

func symbolPresentRule(applies ...graph.ID) []Rule {
	return []Rule{{ID: "d2", Kind: "decision", Predicate: "symbol_present", AppliesTo: applies}}
}

// TestConflicts_DecisionSymbolPresent: a symbol_present decision fires for
// a new callable in a touched unit with no Calls, MayCall or References
// edge to the symbol it names (the P2 edge set), at a commit revision. A
// callable that reaches it by any of the three, a callable changed in
// place, a new constant, and a callable outside the units the rule names
// are not conflicts.
func TestConflicts_DecisionSymbolPresent(t *testing.T) {
	c2 := graph.RevCommit("C2")
	s, d, s1, l := symbolPresentFx(c2)
	got := onlyKind(Conflicts(s, d, symbolPresentRule(l)), ConflictDecisionContradicted)
	want := []Conflict{{Kind: ConflictDecisionContradicted, Rev: c2, Unit: fxUnit("a"), RuleID: "d2",
		Subject: s1, Evidence: []graph.ID{s1, l}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("symbol_present L: %s\nwant %s", cstr(got), cstr(want))
	}
	if got := onlyKind(Conflicts(s, d, symbolPresentRule(l, fxUnit("a"))), ConflictDecisionContradicted); len(got) != 1 || got[0].Subject != s1 {
		t.Errorf("scoped to unit a: %s, want S1", cstr(got))
	}
	if got := onlyKind(Conflicts(s, d, symbolPresentRule(l, fxUnit("other"))), ConflictDecisionContradicted); len(got) != 0 {
		t.Errorf("scoped to another unit: %s, want none", cstr(got))
	}
	if got := onlyKind(Conflicts(s, d, symbolPresentRule(fxUnit("a"))), ConflictDecisionContradicted); len(got) != 0 {
		t.Errorf("a rule naming no symbol: %s, want none", cstr(got))
	}
}

// TestConflicts_DecisionSymbolPresent_SilentAtOverlayRev: P2 ruling 1. The
// same edit at an overlay revision raises nothing: an added callable whose
// node at d.Rev is an overlay record has out-edges that are unknown, not
// absent (the Go overlay carries Declares alone), so in M2 symbol_present
// fires at commit revisions only. The control is the commit twin.
func TestConflicts_DecisionSymbolPresent_SilentAtOverlayRev(t *testing.T) {
	s, d, s1, l := symbolPresentFx(graph.RevCommit("C2"))
	if got := onlyKind(Conflicts(s, d, symbolPresentRule(l)), ConflictDecisionContradicted); len(got) != 1 || got[0].Subject != s1 {
		t.Fatalf("control, at a commit: %s, want S1", cstr(got))
	}
	s, d, s1, l = symbolPresentFx(fxOv(1))
	if !slices.Contains(labels(d.Decls), "n "+string(s1)) {
		t.Fatalf("precondition: the overlay does not add S1: %q", labels(d.Decls))
	}
	if got := onlyKind(Conflicts(s, d, symbolPresentRule(l)), ConflictDecisionContradicted); len(got) != 0 {
		t.Errorf("at an overlay: %s, want none", cstr(got))
	}
}

// TestConflicts_ProtectedPath: a protect_path rule fires once per file in
// the diff's Files that its AppliesTo names; a unit id there is not a path.
func TestConflicts_ProtectedPath(t *testing.T) {
	f := newFx()
	fa := f.file("a", "a/one.lx", fxC1)
	fb := f.file("b", "b/two.lx", fxC1)
	fc := f.file("c", "c/three.lx", fxC1)
	ov := fxOv(1)
	rules := []Rule{{ID: "p1", Kind: "protect_path", AppliesTo: []graph.ID{fc, fa}}}

	got := onlyKind(Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fa, fb}), rules), ConflictProtectedPath)
	want := []Conflict{{Kind: ConflictProtectedPath, Rev: ov, Unit: fxUnit("a"), RuleID: "p1", Subject: fa, Evidence: []graph.ID{fa}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a protected file in the diff: %s\nwant %s", cstr(got), cstr(want))
	}
	if got := onlyKind(Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fb}), rules), ConflictProtectedPath); len(got) != 0 {
		t.Errorf("no protected file in the diff: %s, want none", cstr(got))
	}
	units := []Rule{{ID: "p2", Kind: "protect_path", AppliesTo: []graph.ID{fxUnit("a")}}}
	if got := onlyKind(Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fa}), units), ConflictProtectedPath); len(got) != 0 {
		t.Errorf("a unit id as a path: %s, want none", cstr(got))
	}
}

// TestConflicts_SignatureMismatchWrapsArity: the specimen's signature edit
// at act 12. Each arity conflict on the edited callee at the overlay is a
// signature_mismatch, its Edge, Callee, Want and Got the arity conflict's,
// in the callee's unit. The callee is changed in place, so it is in
// neither Decls nor Removed: the detector reads its sig at d.Base and at
// d.Rev. Controls: no edit, no conflict; a base version with no sig fires
// nothing though the arity conflict stands (a sig appearing is a new
// spelling, not a change: the Go index writes none), the same edit with a
// base sig fires, and a Diff built with no Base compares nothing.
func TestConflicts_SignatureMismatchWrapsArity(t *testing.T) {
	sp := graphtest.New(t)
	c1 := graph.RevCommit(graphtest.Commit)
	if _, err := sp.Overlay(sessionsPy, oldDef, newDef, 12); err != nil {
		t.Fatal(err)
	}
	ov12 := graph.RevOverlay(graphtest.Commit, 12)
	files := []graph.ID{graph.FileID(sessionsPy)}

	got := onlyKind(Conflicts(sp.Store, DiffOf(sp.Store, c1, ov12, files), nil), ConflictSignatureMismatch)
	arity := ArityConflicts(sp.Store, createSession, ov12)
	if len(got) != 2 || len(arity) != 2 {
		t.Fatalf("signature_mismatch %s, arity %s; want two of each", cstr(got), describe(arity))
	}
	if got[0].Edge.Src != handler || got[1].Edge.Src != testCreate {
		t.Errorf("callers %s, want api.py then test_sessions.py", cstr(got))
	}
	unit := graph.UnitID(pylite.Lang, "services/auth")
	for i, c := range got {
		a := arity[i]
		if c.Edge.Key() != a.Edge.Key() || c.Callee.ID != createSession || c.Want != "(user, org, ttl)" || c.Got != 2 ||
			c.Want != a.Want || c.Got != a.Got || c.Rev != ov12 || c.Subject != createSession || c.Unit != unit || c.RuleID != "" {
			t.Errorf("conflict %d = %+v\narity %+v", i, c, a)
		}
	}
	if got := onlyKind(Conflicts(sp.Store, DiffOf(sp.Store, c1, c1, files), nil), ConflictSignatureMismatch); len(got) != 0 {
		t.Errorf("no edit: %s, want none", cstr(got))
	}

	for _, tc := range []struct {
		name    string
		baseSig string
		want    int
	}{{"a base with no sig", "", 0}, {"a base with a sig", "(a)", 1}} {
		f := newFx()
		fl := f.file("a", "a/one.lx", fxC1)
		base := map[string]string{"arity_min": "1", "arity_max": "1"}
		if tc.baseSig != "" {
			base["sig"] = tc.baseSig
		}
		s := f.sym(fl, "a", "func", "S", fxC1, base)
		caller := f.sym(fl, "a", "func", "T", fxC1, nil)
		f.edge(caller, graph.Calls, s, fxC1, map[string]string{"argc": "1"})
		ov := fxOv(1)
		f.s.AssertNode(graph.Node{ID: s, Kind: "sym", Rev: ov,
			Attrs: map[string]string{"name": "S", "symkind": "func", "sig": "(a, b)", "arity_min": "2", "arity_max": "2"}})
		if a := ArityConflicts(f.s, s, ov); len(a) != 1 {
			t.Fatalf("%s: precondition, arity %s, want one", tc.name, describe(a))
		}
		if got := onlyKind(Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fl}), nil), ConflictSignatureMismatch); len(got) != tc.want {
			t.Errorf("%s: %s, want %d", tc.name, cstr(got), tc.want)
		}
		if got := onlyKind(Conflicts(f.s, Diff{Rev: ov, Files: []graph.ID{fl}}, nil), ConflictSignatureMismatch); len(got) != 0 {
			t.Errorf("%s, a Diff with no Base: %s, want none", tc.name, cstr(got))
		}
	}
}

// TestConflicts_Deterministic: the order is the kind's place in
// ConflictKinds, then Unit, RuleID, Subject and the edge key, whatever the
// order of the rules or of the diff's records; an unknown rule kind yields
// nothing; every key is distinct.
func TestConflicts_Deterministic(t *testing.T) {
	b := newBoundaryFx()
	ov := fxOv(1)
	b.f.edge(b.e1, graph.Calls, b.h1, ov, nil)
	b.f.edge(b.e0, graph.Calls, b.h1, ov, nil)
	d := DiffOf(b.f.s, fxC1, ov, []graph.ID{b.fileE})
	rules := []Rule{
		{ID: "p1", Kind: "protect_path", AppliesTo: []graph.ID{b.fileE}},
		boundaryRule("b1", graphU, hookU),
		{ID: "d1", Kind: "decision", Predicate: "paths_untouched", AppliesTo: []graph.ID{b.fileE}},
		{ID: "x1", Kind: "no_such_kind", AppliesTo: []graph.ID{b.fileE}},
		{ID: "p0", Kind: "protect_path", AppliesTo: []graph.ID{b.fileE}},
	}

	got := Conflicts(b.f.s, d, rules)
	var seq []string
	for _, c := range got {
		seq = append(seq, string(c.Kind)+" "+c.RuleID+" "+string(c.Subject))
	}
	want := []string{
		"boundary_cross b1 " + string(b.e0),
		"boundary_cross b1 " + string(b.e1),
		"decision_contradicted d1 " + string(b.fileE),
		"protected_path p0 " + string(b.fileE),
		"protected_path p1 " + string(b.fileE),
	}
	if !slices.Equal(seq, want) {
		t.Fatalf("order:\n%q\nwant\n%q", seq, want)
	}
	rev := slices.Clone(rules)
	slices.Reverse(rev)
	if again := Conflicts(b.f.s, d, rev); !reflect.DeepEqual(got, again) {
		t.Errorf("reversed rules:\n%s\n%s", cstr(got), cstr(again))
	}
	d2 := d
	d2.Decls = slices.Clone(d.Decls)
	slices.Reverse(d2.Decls)
	if again := Conflicts(b.f.s, d2, rules); !reflect.DeepEqual(got, again) {
		t.Errorf("reversed Decls:\n%s\n%s", cstr(got), cstr(again))
	}
	keys := map[string]bool{}
	for _, c := range got {
		keys[c.Key()] = true
	}
	if len(keys) != len(got) {
		t.Errorf("%d conflicts, %d keys", len(got), len(keys))
	}
}

// TestConflict_KeySeparatesEdgelessKinds: D4. An arity conflict's key is
// its edge's key byte for byte (the session's map is keyed by it). Every
// other kind's key is its kind, rule, unit and subject, and its edge when
// it has one, so two conflicts that differ in any of these differ in key;
// Evidence is not identity.
func TestConflict_KeySeparatesEdgelessKinds(t *testing.T) {
	e := graph.Edge{Src: "sym:lx:a:func:A", Pred: graph.Calls, Dst: "sym:lx:b:func:B", Attrs: map[string]string{"argc": "1"}}
	ar := Conflict{Kind: ConflictArity, Edge: e, Got: 1, Want: "()"}
	if ar.Key() != e.Key() {
		t.Fatalf("arity key %q, want the edge key %q", ar.Key(), e.Key())
	}

	keys := map[string]string{}
	add := func(name string, c Conflict) {
		t.Helper()
		k := c.Key()
		if prev, ok := keys[k]; ok {
			t.Errorf("%s and %s share the key %q", prev, name, k)
		}
		keys[k] = name
	}
	add("arity", ar)
	base := Conflict{RuleID: "r1", Unit: "unit:lx:a", Subject: "file:a/one.lx"}
	for _, k := range ConflictKinds[1:] {
		c := base
		c.Kind = k
		add(string(k), c)
	}
	dec := base
	dec.Kind = ConflictDecisionContradicted
	for name, mut := range map[string]func(*Conflict){
		"another rule":    func(c *Conflict) { c.RuleID = "r2" },
		"another unit":    func(c *Conflict) { c.Unit = "unit:lx:b" },
		"another subject": func(c *Conflict) { c.Subject = "file:a/two.lx" },
	} {
		c := dec
		mut(&c)
		add("decision, "+name, c)
	}
	bc := base
	bc.Kind = ConflictBoundaryCross
	bc.Edge = e
	add("boundary with an edge", bc)
	bc.Edge.Dst = "sym:lx:c:func:C"
	add("boundary with another edge", bc)
	sig := Conflict{Kind: ConflictSignatureMismatch, Unit: "unit:lx:b", Subject: e.Dst, Edge: e}
	add("signature", sig)
	sig.Edge.Src = "sym:lx:a:func:A2"
	add("signature, another caller", sig)

	ev, ev2 := dec, dec
	ev.Evidence = []graph.ID{"x"}
	ev2.Evidence = []graph.ID{"y", "z"}
	if ev.Key() != ev2.Key() {
		t.Errorf("Evidence changed the key: %q vs %q", ev.Key(), ev2.Key())
	}
}

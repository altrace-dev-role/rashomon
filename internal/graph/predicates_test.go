package graph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// predConsts is every Pred constant this package declares, by value, read
// from the source so the list cannot drift from the declarations.
func predConsts(t *testing.T) map[Pred]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[Pred]string{}
	fset := token.NewFileSet()
	for _, f := range files {
		if filepath.Ext(f) != ".go" || len(f) > 8 && f[len(f)-8:] == "_test.go" {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range af.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				id, ok := vs.Type.(*ast.Ident)
				if !ok || id.Name != "Pred" || len(vs.Values) != 1 {
					continue
				}
				lit, ok := vs.Values[0].(*ast.BasicLit)
				if !ok {
					continue
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				if prev, dup := out[Pred(v)]; dup {
					t.Errorf("%s and %s both spell %q", prev, vs.Names[0].Name, v)
				}
				out[Pred(v)] = vs.Names[0].Name
			}
		}
	}
	return out
}

// TestPredicates_Closed (W-M1.7, AD15, AC-X.5): Predicates is the closed
// vocabulary. Every Pred constant the package declares is listed once, the
// list holds nothing the package does not declare, every word is lower-case
// letters and underscores, the process and lifecycle predicates of Part B4
// and B6 are among them, and IsPredicate answers by the list.
func TestPredicates_Closed(t *testing.T) {
	consts := predConsts(t)
	listed := map[Pred]bool{}
	for _, p := range Predicates() {
		if listed[p] {
			t.Errorf("%q listed twice", p)
		}
		listed[p] = true
		if _, ok := consts[p]; !ok {
			t.Errorf("%q listed but not declared", p)
		}
		for i := 0; i < len(p); i++ {
			if c := p[i]; (c < 'a' || c > 'z') && c != '_' {
				t.Errorf("%q is not lower-case letters and underscores", p)
				break
			}
		}
		if !IsPredicate(p) {
			t.Errorf("IsPredicate(%q) = false", p)
		}
	}
	for p, name := range consts {
		if !listed[p] {
			t.Errorf("%s = %q declared but not listed", name, p)
		}
	}
	for _, p := range []Pred{Publishes, Subscribes, Schedules, Migrates, SameAs, Supersedes, Contradicts, Advances, Step, Exposes, Consumes, Reads, Writes} {
		if !listed[p] {
			t.Errorf("%q (Part B4/B6) not listed", p)
		}
	}
	for _, p := range []Pred{Asks, Acts, ReadsFile, Edits, Runs, Claims, EvidencedBy, Delivered, FollowedBy} {
		if !listed[p] {
			t.Errorf("%q (session predicate, W-M2.11) not listed", p)
		}
	}
	if Reads == ReadsFile || ReadsFile != "reads_file" {
		t.Errorf("the session read edge is %q; it must stay apart from Reads (engine walks read Reads)", ReadsFile)
	}
	if len(Predicates()) != 34 || IsPredicate("may_calls") || IsPredicate("") {
		t.Errorf("%d predicates, IsPredicate(may_calls)=%v; want 34 and false", len(Predicates()), IsPredicate("may_calls"))
	}
	first := Predicates()
	first[0] = "x"
	if Predicates()[0] == "x" {
		t.Errorf("Predicates returns the one slice; a caller's edit changed it")
	}
}

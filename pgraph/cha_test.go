package pgraph

import (
	"go/ast"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dm = "example.test/dispatch"

var dispatchRoot = filepath.Join("testdata", "dispatch")

func dispatchLabels(t *testing.T) Labels {
	t.Helper()
	l, err := ParseLabels(mustRead(t, filepath.Join(dispatchRoot, "labels.tsv")))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func dispatchOf(t *testing.T, p *Program) []Triple {
	t.Helper()
	ts, err := Dispatch(p)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// PL3 done-when: precision and recall against a hand-labelled set, published
// with the corpus named and sized: precision 0.368 (14 of 38 CHA edges are
// true) and recall 1.000 (14 of 14 labelled edges found) on dispatch-v1, whose
// 14 labelled edges were read from the source.
// Expected CHA size, from the algorithm applied by hand to testdata/dispatch:
// the five Area sites (Total, Closure, UseHolder, the package initialiser and
// Vol, which reaches Shape.Area through Holder's embedded interface) each
// reach the six concrete Area methods (Circle, Square, Tri, Ghost, Cube,
// Rect) = 30; Vol's Volume reaches Cube and Holder = 2; Label's Name reaches
// Circle, Base and Dog = 3; Seal reaches Circle and Cube (Rect's seal is
// another package's method) = 2; Fail's Error reaches Bad = 1. Total 38,
// of which the 14 labelled edges are true.
func TestPL3_CorpusPrecisionRecall(t *testing.T) {
	p := loadDepsT(t, dispatchRoot)
	lab := dispatchLabels(t)
	got := dispatchOf(t, p)
	acc := Score(got, lab.Truth)
	t.Logf("PL3 %s", acc.Describe(lab.Corpus))
	if lab.Corpus != "dispatch-v1" {
		t.Errorf("corpus named %q", lab.Corpus)
	}
	const published = "precision 0.368 (14 of 38 CHA edges are true), recall 1.000 (14 of 14 labelled edges found), corpus dispatch-v1: 14 labelled edges"
	if got := acc.Describe(lab.Corpus); got != published {
		t.Errorf("published figure = %q, want %q", got, published)
	}
	if acc != (Accuracy{Got: 38, Want: 14, Hit: 14}) {
		t.Errorf("accuracy = %+v, want {Got:38 Want:14 Hit:14}", acc)
	}
	if acc.Recall() != 1 {
		t.Errorf("recall = %v: CHA must find every true edge", acc.Recall())
	}
	if want := 14.0 / 38.0; math.Abs(acc.Precision()-want) > 1e-12 {
		t.Errorf("precision = %v, want %v", acc.Precision(), want)
	}

	// Spurious edges are allowed and are the ones the algorithm must produce:
	// Ghost and Rect are never constructed yet every Area site reaches them.
	g := set(got)
	areaSites := []string{"func:" + dm + "/app.Total", "func:" + dm + "/app.Closure", "func:" + dm + "/app.UseHolder", "func:" + dm + "/app.Vol", "pkg:" + dm + "/app"}
	for _, never := range []string{"method:" + dm + "/shape.Ghost.Area", "method:" + dm + "/extra.Rect.Area"} {
		for _, s := range areaSites {
			if !g[Triple{s, MayCall, never}] {
				t.Errorf("CHA lacks the spurious edge %s -> %s", s, never)
			}
		}
	}
	// And edges that no sound analysis includes are absent: another method
	// name, another package's unexported method of the same spelling, an
	// abstract method, and anything outside the scan root.
	for _, absent := range []Triple{
		{"func:" + dm + "/app.Total", MayCall, "method:" + dm + "/extra.Dog.Name"},
		{"func:" + dm + "/app.Total", MayCall, "method:" + dm + "/shape.Cube.Volume"},
		{"func:" + dm + "/shape.Seal", MayCall, "method:" + dm + "/extra.Rect.seal"},
		{"func:" + dm + "/app.Vol", MayCall, "method:" + dm + "/shape.Shape.Area"},
		{"func:" + dm + "/app.Vol", MayCall, "method:" + dm + "/shape.Solid.Area"},
	} {
		if g[absent] {
			t.Errorf("unexpected edge %v", absent)
		}
	}
	for _, x := range got {
		if !strings.HasPrefix(x.Object, "method:"+dm+"/") {
			t.Errorf("target outside the scan root: %v", x)
		}
	}
}

// REQUIREMENT (Karthik, 2026-10-24): an interface call made inside a
// dependency is dispatch like any other. sort.Sort calls Len, Less and Swap
// through sort.Interface from inside the standard library, so a repo type's
// Less is reached through sort, and the graph must say so: a controller that
// asks what a change to byLen.Less owes has to find sort.Sort among its
// callers. The caller of such an edge is the dependency function, named in
// the vocabulary of every other id (func:sort.Sort); the target is still a
// method in the scan root, and the repo function that merely calls sort.Sort
// makes no interface call of its own, so it gets no dispatch edge (its static
// call to sort.Sort is a Calls fact). The control shows the fixture is not
// blind: an interface call in the repo's own code makes its edge.
func TestPL3_DependencyInternalDispatchIsVisible(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/gap\n\ngo 1.25.0\n")
	write("p.go", `package p

import "sort"

// byLen is sorted by sort.Sort, which calls Len, Less and Swap through
// sort.Interface from inside the standard library.
type byLen []string

func (b byLen) Len() int           { return len(b) }
func (b byLen) Less(i, j int) bool { return len(b[i]) < len(b[j]) }
func (b byLen) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func Sorted(s []string) { sort.Sort(byLen(s)) }

// Sizer and Box are the control: an interface call in this module's own code.
type Sizer interface{ Size() int }

type Box struct{}

func (Box) Size() int { return 1 }

func Measure(s Sizer) int { return s.Size() }
`)
	got := dispatchOf(t, loadDepsT(t, dir))
	const gp = "example.test/gap"
	g := set(got)
	control := Triple{"func:" + gp + ".Measure", MayCall, "method:" + gp + ".Box.Size"}
	if !g[control] {
		t.Fatalf("the control edge %v is missing, so the rest proves nothing: %v", control, got)
	}
	for _, m := range []string{"Len", "Less", "Swap"} {
		want := Triple{"func:sort.Sort", MayCall, "method:" + gp + ".byLen." + m}
		if !g[want] {
			t.Errorf("missing %v: sort.Sort reaches byLen.%s through sort.Interface", want, m)
		}
	}
	for _, x := range got {
		if strings.HasPrefix(x.Subject, "func:sort.") && !ast.IsExported(strings.TrimPrefix(x.Subject, "func:sort.")) {
			t.Errorf("a dependency's internal helper is a caller: %v", x)
		}
		if x.Subject == "func:"+gp+".Sorted" {
			t.Errorf("an edge from Sorted, which makes no interface call of its own: %v", x)
		}
		if !strings.HasPrefix(x.Object, "method:"+gp+".") {
			t.Errorf("target outside the scan root: %v", x)
		}
	}
}

// A Program without dependency bodies must not answer: every call made inside
// a dependency would be silently absent, which reads as a graph that says
// nothing calls through them.
func TestPL3_DispatchRefusesAProgramWithoutDependencyBodies(t *testing.T) {
	if _, err := Dispatch(loadT(t, dispatchRoot)); err == nil {
		t.Fatal("Dispatch answered on a scan-root-only Program")
	}
	var l Log
	if _, err := SyncCHA(&l, loadT(t, dispatchRoot), "c"); err == nil || l.Len() != 0 {
		t.Fatalf("SyncCHA on a scan-root-only Program: %v, %d facts", err, l.Len())
	}
}

func TestPL3_AgreesWithRederivation(t *testing.T) {
	for name, root := range map[string]string{"dispatch": dispatchRoot, "testdata": testModule, "idrepo": filepath.Join("testdata", "idrepo", "commit1"), "repo": ""} {
		t.Run(name, func(t *testing.T) {
			if name == "repo" {
				root = repoRoot(t)
			}
			p := loadDepsT(t, root)
			all := dispatchOf(t, p)
			// The oracle reads the syntax of the scan root, so it speaks for
			// the edges whose caller is in the scan root; the edges whose
			// caller is a dependency function are checked by
			// TestPL3_DependencyEdgesNameEntryPoints and the sort fixture.
			got := onlyRootCallers(p, all)
			want := rederiveDispatch(t, p)
			if name == "dispatch" && len(all) != 38 {
				t.Errorf("%d edges on the corpus, want 38", len(got))
			}
			if len(got) == 0 {
				t.Fatal("no edges: two empty answers would agree")
			}
			diffTriples(t, got, want)
			t.Logf("%s: %d may-call edges from the scan root agree (%d more have a dependency function as caller)", name, len(got), len(all)-len(got))
		})
	}
}

// inRoot reports whether a symbol id (kind:PKG.Name, or pkg:PKG) names
// something declared in one of p's scanned packages.
func inRoot(p *Program, id string) bool {
	_, rest, _ := strings.Cut(id, ":")
	for _, pkg := range p.pkgs {
		if rest == pkg.PkgPath || strings.HasPrefix(rest, pkg.PkgPath+".") {
			return true
		}
	}
	return false
}

func onlyRootCallers(p *Program, ts []Triple) []Triple {
	var out []Triple
	for _, x := range ts {
		if inRoot(p, x.Subject) {
			out = append(out, x)
		}
	}
	return out
}

// A dependency function is a caller only as the entry point scanned code can
// reach it by: an exported function, a method with an exported name on an
// exported type, or the package. Its internal helpers (sort.insertionSort,
// which is where Less is really called) never appear as callers.
func TestPL3_DependencyEdgesNameEntryPoints(t *testing.T) {
	p := loadDepsT(t, repoRoot(t))
	n := 0
	for _, x := range dispatchOf(t, p) {
		if inRoot(p, x.Subject) {
			continue
		}
		n++
		kind, rest, _ := strings.Cut(x.Subject, ":")
		if kind == "pkg" {
			continue
		}
		parts := strings.Split(rest[strings.LastIndex(rest, "/")+1:], ".")
		if kind == "func" && len(parts) == 2 && ast.IsExported(parts[1]) {
			continue
		}
		if kind == "method" && len(parts) == 3 && ast.IsExported(parts[1]) && ast.IsExported(parts[2]) {
			continue
		}
		t.Errorf("a dependency caller that is not an entry point: %v", x)
	}
	if n < 100 {
		t.Errorf("only %d edges from dependency callers on this repository", n)
	}
}

// A partial program: CHA over a subset of the packages is a subset of CHA over
// the whole, so adding code never withdraws an edge. Without package extra the
// difference is exactly the six edges whose targets are extra's (Rect.Area
// from five sites, Dog.Name from Label); without package app the only site
// left is Seal's.
func TestPL3_SoundOnPartialPrograms(t *testing.T) {
	full := set(dispatchOf(t, loadDepsT(t, dispatchRoot)))
	for _, c := range []struct {
		drop    string
		missing int
		keep    int
	}{{"extra", 6, 32}, {"app", 36, 2}} {
		dir := copyTree(t, dispatchRoot)
		if err := os.RemoveAll(filepath.Join(dir, c.drop)); err != nil {
			t.Fatal(err)
		}
		part := dispatchOf(t, loadDepsT(t, dir))
		if len(part) != c.keep {
			t.Errorf("without %s: %d edges, want %d", c.drop, len(part), c.keep)
		}
		for _, x := range part {
			if !full[x] {
				t.Errorf("without %s: edge %v is not in the whole program's", c.drop, x)
			}
		}
		if len(full)-len(part) != c.missing {
			t.Errorf("without %s: %d edges lost, want %d", c.drop, len(full)-len(part), c.missing)
		}
		if c.drop == "extra" {
			for x := range full {
				if !set(part)[x] && !strings.Contains(x.Object, "/extra.") {
					t.Errorf("without extra: lost %v, whose target is not extra's", x)
				}
			}
		}
	}
}

func TestPL3_DispatchDeterministicSortedUnique(t *testing.T) {
	p := loadDepsT(t, dispatchRoot)
	a, b := dispatchOf(t, p), dispatchOf(t, p)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("lens %d %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("runs differ at %d", i)
		}
		if a[i].Predicate != MayCall {
			t.Fatalf("predicate %v", a[i].Predicate)
		}
		if i > 0 && !tripleLess(a[i-1], a[i]) {
			t.Fatalf("not strictly sorted at %d", i)
		}
	}
}

// Tier is the producer's: CHA writes ALGORITHMIC facts, extraction writes
// PROVEN ones, and neither can write the other's.
func TestPL3_FactsAreAlgorithmicNeverProven(t *testing.T) {
	var l Log
	prog := loadDepsT(t, dispatchRoot)
	if _, err := Sync(&l, prog, "c1"); err != nil {
		t.Fatal(err)
	}
	for _, f := range l.Live() {
		if f.Triple().Predicate == MayCall {
			t.Fatalf("the PROVEN extractor wrote a may-call fact: %v", f.Triple())
		}
	}
	d, err := SyncCHA(&l, prog, "c1")
	if err != nil || d.Asserted != 38 || d.Retracted != 0 {
		t.Fatalf("SyncCHA: %+v %v", d, err)
	}
	proven, algo := 0, 0
	for _, f := range l.Live() {
		switch f.Tier() {
		case Proven:
			proven++
			if f.Extractor() != "gotypes@v1" {
				t.Errorf("PROVEN fact from %s", f.Extractor())
			}
		case Algorithmic:
			algo++
			if f.Triple().Predicate != MayCall || f.Extractor() != "cha@v1" || f.Commit() != "c1" || f.Config() != prog.Config().String() {
				t.Errorf("ALGORITHMIC fact %v from %s", f.Triple(), f.Extractor())
			}
		default:
			t.Errorf("tier %v", f.Tier())
		}
	}
	if proven == 0 || algo != 38 {
		t.Fatalf("%d PROVEN, %d ALGORITHMIC", proven, algo)
	}
	// The two tiers do not retract each other: re-syncing one leaves the other.
	before := string(l.View())
	if d, _ := Sync(&l, prog, "c2"); d != (Delta{}) {
		t.Errorf("no-op PROVEN sync: %+v", d)
	}
	if d, _ := SyncCHA(&l, prog, "c2"); d != (Delta{}) || string(l.View()) != before {
		t.Errorf("no-op CHA sync: %+v", d)
	}
	if !strings.Contains(before, "\tmay-call\t") || !strings.Contains(before, "\tALGORITHMIC\t") {
		t.Error("view does not show the tier")
	}
}

// The rule that keeps ALGORITHMIC from being read as PROVEN: a may-call fact
// at the PROVEN tier is refused when written and when read back.
func TestPL3_MayCallRefusedAtProven(t *testing.T) {
	bad := Triple{"func:a.F", MayCall, "method:a.T.M"}
	var l Log
	if _, err := l.Reconcile(gotypesProducer(), cfgA, "c1", []Triple{bad}); err == nil {
		t.Error("PROVEN producer wrote a may-call fact")
	}
	if l.Len() != 0 {
		t.Error("a refused reconcile appended records")
	}
	if _, err := l.Reconcile(chaProducer(), cfgA, "c1", []Triple{bad}); err != nil {
		t.Errorf("ALGORITHMIC producer refused: %v", err)
	}
	if _, err := l.Reconcile(mustProposed(t, "m"), cfgA, "c1", []Triple{bad}); err != nil {
		t.Errorf("model-proposed producer refused: %v", err)
	}
	rec := "+\tfunc:a.F\tmay-call\tmethod:a.T.M\t%s\t" + cfgA + "\tc1\t%s\n"
	for tier, ok := range map[string]bool{"PROVEN": false, "ALGORITHMIC": true, "model-proposed": true} {
		in := LogHeader + "\n" + strings.Replace(strings.Replace(rec, "%s", tier, 1), "%s", "x@1", 1)
		if _, err := DecodeLog([]byte(in)); (err == nil) != ok {
			t.Errorf("decode of a %s may-call record: err=%v, want accepted=%v", tier, err, ok)
		}
	}
	// Negative control: the same record at PROVEN with another predicate is fine.
	in := LogHeader + "\n+\tfunc:a.F\tcalls\tfunc:a.G\tPROVEN\t" + cfgA + "\tc1\tx@1\n"
	if _, err := DecodeLog([]byte(in)); err != nil {
		t.Errorf("PROVEN calls record refused: %v", err)
	}
}

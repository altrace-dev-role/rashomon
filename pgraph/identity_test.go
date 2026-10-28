package pgraph

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const idm = "example.test/id"

// git runs git in dir with no user or system configuration, so the fixture
// commits do not depend on the machine's settings.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// idRepo makes a throwaway git repository whose two commits are the trees
// testdata/idrepo/commit1 and commit2, and returns it with the two commit ids.
// The tree is replaced wholesale for the second commit, so a renamed or moved
// file is a deletion and an addition, as git records it.
func idRepo(t *testing.T) (dir string, commits [2]string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	dir = t.TempDir()
	git(t, dir, "init", "-q")
	for i, snap := range []string{"commit1", "commit2"} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.Name() != ".git" {
				os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
		src := copyTree(t, filepath.Join("testdata", "idrepo", snap))
		ents, _ := os.ReadDir(src)
		for _, e := range ents {
			if err := os.Rename(filepath.Join(src, e.Name()), filepath.Join(dir, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
		git(t, dir, "add", "-A")
		git(t, dir, "commit", "-q", "-m", snap)
		commits[i] = git(t, dir, "rev-parse", "HEAD")
	}
	return dir, commits
}

func loadAt(t *testing.T, dir, commit string) *Program {
	t.Helper()
	git(t, dir, "checkout", "-q", commit)
	return loadT(t, dir)
}

// declared returns the objects the package at path declares under name, in
// source order (file name, then offset) (several for init, the blank identifier and shadowed locals).
func declared(p *Program, path, name string) []types.Object {
	var out []types.Object
	for _, pkg := range p.pkgs {
		if pkg.PkgPath != path {
			continue
		}
		for id, obj := range pkg.TypesInfo.Defs {
			if obj != nil && id.Name == name {
				if _, isVar := obj.(*types.Var); isVar && obj.Parent() != pkg.Types.Scope() {
					continue
				}
				out = append(out, obj)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := p.fset.Position(out[i].Pos()), p.fset.Position(out[j].Pos())
		return a.Filename < b.Filename || (a.Filename == b.Filename && a.Offset < b.Offset)
	})
	return out
}

func methodOf(t *testing.T, p *Program, path, typ, name string) *types.Func {
	t.Helper()
	for _, o := range declared(p, path, typ) {
		if n, ok := o.Type().(*types.Named); ok {
			for i := 0; i < n.NumMethods(); i++ {
				if n.Method(i).Name() == name {
					return n.Method(i)
				}
			}
			if it, ok := n.Underlying().(*types.Interface); ok {
				for i := 0; i < it.NumExplicitMethods(); i++ {
					if it.ExplicitMethod(i).Name() == name {
						return it.ExplicitMethod(i)
					}
				}
			}
		}
	}
	t.Fatalf("no method %s.%s in %s", typ, name, path)
	return nil
}

// Ids are written from the scheme: kind, a colon, the package path, a dot,
// the name; a method spells its receiver type (no pointer, no type
// parameters); names that may repeat in a package carry "#" and a 12-digit hex
// fingerprint of what they are ({fp} below), never an ordinal or a position.
// The second column of each row is a spelling the replaced schemes produced and
// this one must not.
func TestPL10_Spelling(t *testing.T) {
	p := loadT(t, filepath.Join("testdata", "idrepo", "commit1"))
	nm := newNamer(p)
	core := idm + "/core"
	one := func(name string, i int) types.Object {
		t.Helper()
		objs := declared(p, core, name)
		if len(objs) <= i {
			t.Fatalf("%s: %d objects", name, len(objs))
		}
		return objs[i]
	}
	for _, c := range []struct {
		what      string
		obj       types.Object
		want, old string
	}{
		{"function", one("Compute", 0), "func:" + core + ".Compute", ""},
		{"method, pointer receiver", methodOf(t, p, core, "Stack", "Reset"), "method:" + core + ".Stack.Reset", "method:(*" + core + ".Stack).Reset"},
		{"same method name, other receiver", methodOf(t, p, core, "Queue", "Reset"), "method:" + core + ".Queue.Reset", "method:(*" + core + ".Queue).Reset"},
		{"type", one("Stack", 0), "type:" + core + ".Stack", ""},
		{"variable", one("registry", 0), "var:" + core + ".registry", ""},
		{"constant", one("MaxSize", 0), "const:" + core + ".MaxSize", ""},
		{"first init", one("init", 0), "func:" + core + ".init{fp}", "func:" + core + ".init#a_init.go@"},
		{"second init", one("init", 1), "func:" + core + ".init{fp}", "func:" + core + ".init#b_init.go@"},
		{"blank function", one("_", 0), "func:" + core + "._{fp}", ""},
		{"local type", one("shower", 0), "type:" + core + ".Pairs$shower{fp}", ""},
		{"same local name, other function", one("shower", 1), "type:" + core + ".Triples$shower{fp}", ""},
		{"shadowing local, outer", one("pair", 0), "type:" + core + ".Both$pair{fp}", ""},
		{"shadowing local, inner", one("pair", 1), "type:" + core + ".Both$pair{fp}", ""},
		{"method of a local interface", methodOf(t, p, core, "shower", "Show"), "method:" + core + ".Pairs$shower{fp}.Show", ""},
	} {
		got := nm.id(c.obj)
		if strings.Contains(c.want, "{fp}") {
			re := regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(c.want), regexp.QuoteMeta("{fp}"), "#[0-9a-f]{12}") + "$")
			if !re.MatchString(got) {
				t.Errorf("%s: id = %q, want the form %q", c.what, got, c.want)
			}
		} else if got != c.want {
			t.Errorf("%s: id = %q, want %q", c.what, got, c.want)
		}
		if c.old != "" && strings.HasPrefix(got, c.old) {
			t.Errorf("%s: id %q still has the replaced spelling", c.what, got)
		}
	}

	// A method of an anonymous interface is spelled by its package and the
	// interface's own text.
	var closeM *types.Func
	for _, pkg := range p.pkgs {
		for _, obj := range pkg.TypesInfo.Uses {
			if f, ok := obj.(*types.Func); ok && f.Name() == "Close" {
				closeM = f
			}
		}
	}
	if closeM == nil {
		t.Fatal("no Close call in the fixture")
	}
	if got, want := nm.id(closeM), "method:"+core+".interface{Close() error}.Close"; got != want {
		t.Errorf("anonymous interface method: %q, want %q", got, want)
	}

	// Universe, standard library and generics, on the first fixture module.
	mod := loadT(t, testModule)
	mm := newNamer(mod)
	errIface := types.Universe.Lookup("error").Type().(*types.Named).Underlying().(*types.Interface)
	if got := mm.id(errIface.Method(0)); got != "method:error.Error" {
		t.Errorf("universe method: %q", got)
	}
	for _, pkg := range mod.pkgs {
		for _, imp := range pkg.Types.Imports() {
			if imp.Path() == "fmt" {
				if got := mm.id(imp.Scope().Lookup("Sprint")); got != "func:fmt.Sprint" {
					t.Errorf("stdlib function: %q", got)
				}
				str := imp.Scope().Lookup("Stringer").Type().(*types.Named).Underlying().(*types.Interface)
				if got := mm.id(str.ExplicitMethod(0)); got != "method:fmt.Stringer.String" {
					t.Errorf("stdlib interface method: %q", got)
				}
			}
		}
	}
	// A call on an instantiated generic receiver names the generic method.
	var inst types.Object
	for _, pkg := range mod.pkgs {
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				if se, ok := n.(*ast.SelectorExpr); ok && se.Sel.Name == "Get" {
					inst = pkg.TypesInfo.Selections[se].Obj()
				}
				return true
			})
		}
	}
	if inst == nil {
		t.Fatal("no Box.Get call in the fixture")
	}
	if got, want := mm.id(inst), "method:"+m+"/a.Box.Get"; got != want {
		t.Errorf("instantiated method: %q, want %q", got, want)
	}
}

// symbolsOf lists every symbol the package set declares: package-level
// objects, methods, interface methods, init and blank functions, and local
// types.
func symbolsOf(p *Program) []types.Object {
	var out []types.Object
	for _, pkg := range p.pkgs {
		for _, obj := range pkg.TypesInfo.Defs {
			switch o := obj.(type) {
			case *types.Func:
				out = append(out, o)
			case *types.TypeName:
				if _, tp := o.Type().(*types.TypeParam); !tp {
					out = append(out, o)
				}
			case *types.Var:
				if o.Parent() == pkg.Types.Scope() {
					out = append(out, o)
				}
			case *types.Const:
				if o.Parent() == pkg.Types.Scope() {
					out = append(out, o)
				}
			}
		}
	}
	return out
}

// sameAnonymousMethod: methods of identical anonymous interface types are one
// symbol by design, since the types are identical.
func sameAnonymousMethod(a, b types.Object) bool {
	fa, ok1 := a.(*types.Func)
	fb, ok2 := b.(*types.Func)
	if !ok1 || !ok2 || fa.Name() != fb.Name() || fa.Pkg() != fb.Pkg() {
		return false
	}
	ra, rb := fa.Type().(*types.Signature).Recv(), fb.Type().(*types.Signature).Recv()
	if ra == nil || rb == nil {
		return false
	}
	_, named := ra.Type().(*types.Named)
	return !named && types.Identical(ra.Type(), rb.Type())
}

// collisions is PL10's first half as a check: the ids, under nm, that name two
// symbols within one build, and how many symbols it looked at. A symbol is a
// declaration in the scanned packages; dependencies are read from export data
// one package instance at a time and checked the same way.
func collisions(p *Program, nm *namer) (found []string, checked int) {
	seen := map[string]types.Object{}
	syms := symbolsOf(p)
	for _, o := range syms {
		id := nm.id(o)
		if prev, dup := seen[id]; dup && prev != o && !sameAnonymousMethod(prev, o) {
			found = append(found, fmt.Sprintf("id %q names %v at %v and %v at %v", id, prev, p.fset.Position(prev.Pos()), o, p.fset.Position(o.Pos())))
		}
		seen[id] = o
	}
	visited := map[*types.Package]bool{}
	var visit func(*types.Package)
	visit = func(tp *types.Package) {
		if visited[tp] {
			return
		}
		visited[tp] = true
		ids := map[string]types.Object{}
		note := func(o types.Object) {
			if id := nm.id(o); ids[id] != nil && ids[id] != o {
				found = append(found, fmt.Sprintf("dependency %s: id %q names %v and %v", tp.Path(), id, ids[id], o))
			} else {
				ids[id] = o
			}
		}
		for _, n := range tp.Scope().Names() {
			o := tp.Scope().Lookup(n)
			note(o)
			if tn, ok := o.(*types.TypeName); ok {
				if nt, ok := tn.Type().(*types.Named); ok {
					for i := 0; i < nt.NumMethods(); i++ {
						note(nt.Method(i))
					}
					if it, ok := nt.Underlying().(*types.Interface); ok {
						for i := 0; i < it.NumExplicitMethods(); i++ {
							note(it.ExplicitMethod(i))
						}
					}
				}
			}
		}
		for _, imp := range tp.Imports() {
			visit(imp)
		}
	}
	for _, pkg := range p.pkgs {
		visit(pkg.Types)
	}
	return found, len(syms) + len(visited)
}

func injective(t *testing.T, name string, p *Program) int {
	t.Helper()
	found, n := collisions(p, newNamer(p))
	for _, f := range found {
		t.Errorf("%s: %s", name, f)
	}
	return n
}

func TestPL10_InjectiveWithinBuild(t *testing.T) {
	dir, commits := idRepo(t)
	for name, p := range map[string]*Program{
		"commit1":  loadAt(t, dir, commits[0]),
		"commit2":  loadAt(t, dir, commits[1]),
		"mod":      loadT(t, testModule),
		"dispatch": loadT(t, filepath.Join("testdata", "dispatch")),
	} {
		if n := injective(t, name, p); n < 20 {
			t.Errorf("%s: only %d symbols checked", name, n)
		}
	}
	t.Run("repo", func(t *testing.T) {
		p := loadT(t, repoRoot(t))
		if n := injective(t, "repo", p); n < 1000 {
			t.Errorf("repo: only %d symbols checked", n)
		}
	})
}

// The checker must be able to fail: with the fingerprints stripped from the
// namer, the fixture's repeated names collide and are reported: two init
// functions and two nested types spelled pair. (The two local interfaces
// spelled shower are told apart by their enclosing function, with or without a
// fingerprint.)
func TestPL10_InjectivityCheckerCatchesCollisions(t *testing.T) {
	p := loadT(t, filepath.Join("testdata", "idrepo", "commit1"))
	nm := newNamer(p)
	if found, _ := collisions(p, nm); len(found) != 0 {
		t.Fatalf("collisions with the real namer: %v", found)
	}
	nm.suffix = map[types.Object]string{}
	found, _ := collisions(p, nm)
	if len(found) != 2 {
		t.Errorf("%d collisions reported without fingerprints, want 2 (init, Both$pair): %v", len(found), found)
	}
}

// Stability is PL10's second half, tested on two real commits: what a commit
// does not rename keeps its id, wherever the commit moved its text. Offsets
// shift, a file is renamed, a body is edited, a third init and a second blank
// function appear, and a type is renamed.
func TestPL10_StableAcrossCommits(t *testing.T) {
	dir, commits := idRepo(t)
	p1, p2 := loadAt(t, dir, commits[0]), loadAt(t, dir, commits[1])
	n1, n2 := newNamer(p1), newNamer(p2)
	core := idm + "/core"

	kept := []struct{ name string }{
		{"Compute"}, {"New"}, {"Pairs"}, {"Triples"}, {"Both"}, {"CloseIt"}, {"Measure"},
		{"Queue"}, {"Sizer"}, {"Small"}, {"registry"}, {"MaxSize"}, {"shower"}, {"pair"},
	}
	for _, k := range kept {
		a, b := declared(p1, core, k.name), declared(p2, core, k.name)
		if len(a) == 0 || len(a) != len(b) {
			t.Fatalf("%s: %d symbols at commit 1, %d at commit 2", k.name, len(a), len(b))
		}
		for i := range a {
			if n1.id(a[i]) != n2.id(b[i]) {
				t.Errorf("%s #%d: id %q became %q", k.name, i, n1.id(a[i]), n2.id(b[i]))
			}
		}
	}
	// The test proves something only if the text really moved.
	c1, c2 := declared(p1, core, "Compute")[0], declared(p2, core, "Compute")[0]
	if p1.fset.Position(c1.Pos()).Offset == p2.fset.Position(c2.Pos()).Offset {
		t.Fatal("Compute did not move between the commits")
	}
	if q1, q2 := methodOf(t, p1, core, "Queue", "Reset"), methodOf(t, p2, core, "Queue", "Reset"); n1.id(q1) != n2.id(q2) {
		t.Errorf("Queue.Reset: %q became %q", n1.id(q1), n2.id(q2))
	}

	// What a commit renames changes id: the scheme names, it does not guess.
	for _, r := range []struct{ from, to string }{{"Normalize", "Canonicalize"}, {"Stack", "Pile"}} {
		if len(declared(p1, core, r.from)) != 1 || len(declared(p2, core, r.to)) != 1 || len(declared(p2, core, r.from)) != 0 {
			t.Fatalf("fixture: %s -> %s", r.from, r.to)
		}
		if a, b := n1.id(declared(p1, core, r.from)[0]), n2.id(declared(p2, core, r.to)[0]); a == b {
			t.Errorf("%s and %s share id %q", r.from, r.to, a)
		}
	}

	// The facts about untouched symbols are the same bytes at both commits.
	// The local interfaces' ids carry a fingerprint; the same type in another
	// function is another symbol, and the id is the same at both commits.
	show := func(p *Program, nm *namer, i int) string {
		return nm.id(declared(p, core, "shower")[i].Type().(*types.Named).Underlying().(*types.Interface).ExplicitMethod(0))
	}
	for i := 0; i < 2; i++ {
		if a, b := show(p1, n1, i), show(p2, n2, i); a != b {
			t.Errorf("local interface %d: %q became %q", i, a, b)
		}
	}
	if show(p1, n1, 0) == show(p1, n1, 1) {
		t.Error("the shower of Pairs and the shower of Triples share an id")
	}
	f1, f2 := set(Extract(p1)), set(Extract(p2))
	for _, w := range []Triple{
		{"pkg:" + core, Declares, "func:" + core + ".Compute"},
		{"pkg:" + core, Declares, "type:" + core + ".Queue"},
		{"type:" + core + ".Queue", Declares, "method:" + core + ".Queue.Reset"},
		{"func:" + core + ".Measure", Calls, "method:" + core + ".Sizer.Size"},
		{"func:" + core + ".Pairs", Calls, show(p1, n1, 0)},
		{"func:" + core + ".Triples", Calls, show(p1, n1, 1)},
		{"func:" + core + ".CloseIt", Calls, "method:" + core + ".interface{Close() error}.Close"},
	} {
		if !f1[w] {
			t.Errorf("commit 1 lacks %v", w)
		}
		if !f2[w] {
			t.Errorf("commit 2 lacks %v", w)
		}
	}

	// The inits are told apart by what they are: the two of commit 1 keep
	// their ids at commit 2, where a third has been added and one of them has
	// moved ahead of the other, and so does the blank function.
	fp1, fp2 := fingerprintsByID(p1), fingerprintsByID(p2)
	ids := func(fps map[string]Fingerprint, declared string) []string {
		var out []string
		for id, f := range fps {
			if f.Declared == declared {
				out = append(out, id)
			}
		}
		sort.Strings(out)
		return out
	}
	i1, i2 := ids(fp1, "func:"+core+".init"), ids(fp2, "func:"+core+".init")
	if len(i1) != 2 || len(i2) != 3 {
		t.Fatalf("inits: %d at commit 1, %d at commit 2", len(i1), len(i2))
	}
	for _, id := range i1 {
		if _, ok := fp2[id]; !ok {
			t.Errorf("init %s of commit 1 has no such id at commit 2: %v", id, i2)
		}
	}
	b1, b2 := ids(fp1, "func:"+core+"._"), ids(fp2, "func:"+core+"._")
	if len(b1) != 1 || len(b2) != 2 || b1[0] != b2[0] && b1[0] != b2[1] {
		t.Errorf("blank functions: %v then %v", b1, b2)
	}
}

func fingerprintsByID(p *Program) map[string]Fingerprint {
	m := map[string]Fingerprint{}
	for _, f := range Fingerprints(p) {
		m[f.ID] = f
	}
	return m
}

// writeModule writes a module example.test/merge with the given files.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module example.test/merge\n\ngo 1.25.0\n"
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The bodies of the merge test. initA and initB are two inits that are what
// they are; initC is the declaration inserted above them; initA2 is initA with
// one literal edited; initX is a different init written where initB was, with
// nothing of initB's body left.
const (
	mergeBase = `package p

var registry = map[string]int{}

func compute(xs []int) int { return len(xs) }
`
	initA = `func init() {
	registry["a"] = compute([]int{1, 2, 3})
	registry["a2"] = compute([]int{4, 5, 6})
	registry["a3"] = compute(nil)
}
`
	initA2 = `func init() {
	registry["a"] = compute([]int{1, 2, 3})
	registry["a2"] = compute([]int{4, 5, 7})
	registry["a3"] = compute(nil)
}
`
	initB = `func init() {
	for i := 0; i < 3; i++ {
		registry[string(rune('a'+i))] += i * i
	}
}
`
	initC = `func init() {
	registry["c"] = compute([]int{9, 8, 7})
	registry["c2"] = compute([]int{6, 5, 4})
	registry["c3"] = compute([]int{3, 2, 1})
}
`
	initX = `func init() {
	if len(registry) > 0 {
		delete(registry, "a")
	} else {
		registry["z"] = -1
	}
}
`
)

// initLinks keeps the links whose source is an init function.
func initLinks(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if strings.HasPrefix(k, "func:example.test/merge.init") {
			out[k] = v
		}
	}
	return out
}

func fingerprintsOfModule(t *testing.T, files map[string]string) []Fingerprint {
	t.Helper()
	return Fingerprints(loadT(t, writeModule(t, files)))
}

// idOf returns the id, among fps, of the init function whose body is the
// text of src (found by fingerprinting src alone in a reference module).
func idOf(t *testing.T, fps []Fingerprint, src string) string {
	t.Helper()
	want := fingerprintsOfModule(t, map[string]string{"p.go": mergeBase + src})
	var body string
	for _, f := range want {
		if strings.HasPrefix(f.ID, "func:"+"example.test/merge.init") {
			body = f.Body
		}
	}
	if body == "" {
		t.Fatal("no init in the reference module")
	}
	for _, f := range fps {
		if f.Body == body && strings.HasPrefix(f.ID, "func:example.test/merge.init") {
			return f.ID
		}
	}
	t.Fatalf("no init with the body of %q among the fingerprints", src)
	return ""
}

// THE MERGE TEST (PL10, ruling 2026-10-24 item 2), stated before the scheme
// that has to pass it.
//
// Must stay the same symbol: two inits, A and B, in two files; a third init, C,
// is inserted above them (once in A's own file, ahead of it, once in a file
// that sorts before both). A and B are the same symbols after the insertion,
// so each keeps its id: identity carries no position. Under the ordinal
// scheme A was init#1 and became init#2, and every fact naming it was
// retracted and asserted again.
//
// Must not: B is replaced by X, an init with nothing of B's body left, and A
// is edited by one literal. Link carries A across the edit (the same declared
// name, a body 96 percent the same) and must not carry B to X, though X is
// B's only possible partner by name, and though no other init competes for
// it.
func TestPL10_MergeTest(t *testing.T) {
	v1 := fingerprintsOfModule(t, map[string]string{"base.go": mergeBase, "a.go": "package p\n\n" + initA, "b.go": "package p\n\n" + initB})
	idA, idB := idOf(t, v1, initA), idOf(t, v1, initB)
	if idA == idB {
		t.Fatalf("two different bodies share the id %q", idA)
	}

	for name, files := range map[string]map[string]string{
		"C inserted above A in A's file":    {"base.go": mergeBase, "a.go": "package p\n\n" + initC + "\n" + initA, "b.go": "package p\n\n" + initB},
		"C inserted in a file sorted first": {"base.go": mergeBase, "0.go": "package p\n\n" + initC, "a.go": "package p\n\n" + initA, "b.go": "package p\n\n" + initB},
		"C inserted and A, B swap files":    {"base.go": mergeBase, "a.go": "package p\n\n" + initB, "b.go": "package p\n\n" + initC + "\n" + initA},
	} {
		t.Run("same/"+name, func(t *testing.T) {
			v2 := fingerprintsOfModule(t, files)
			if got := idOf(t, v2, initA); got != idA {
				t.Errorf("A: id %q became %q", idA, got)
			}
			if got := idOf(t, v2, initB); got != idB {
				t.Errorf("B: id %q became %q", idB, got)
			}
			idC := idOf(t, v2, initC)
			if idC == idA || idC == idB {
				t.Errorf("C shares an id with A or B: %q", idC)
			}
			got := initLinks(linkMap(t, v1, v2))
			if want := map[string]string{idA: idA, idB: idB}; !reflect.DeepEqual(got, want) {
				t.Errorf("links %v, want %v", got, want)
			}
		})
	}

	t.Run("not/edited beyond recognition", func(t *testing.T) {
		v3 := fingerprintsOfModule(t, map[string]string{"base.go": mergeBase, "a.go": "package p\n\n" + initA2, "b.go": "package p\n\n" + initX})
		idA2, idX := idOf(t, v3, initA2), idOf(t, v3, initX)
		if idA2 == idA || idX == idB {
			t.Errorf("an edited body kept its id: A %q -> %q, B %q -> %q", idA, idA2, idB, idX)
		}
		got := initLinks(linkMap(t, v1, v3))
		if want := map[string]string{idA: idA2}; !reflect.DeepEqual(got, want) {
			t.Errorf("links %v, want only the edit of A: %v", got, want)
		}
	})
}

// Exact duplicates: the same declared identity and the same body. The ids are
// the fingerprint and a rank among the copies, the bare "#fp" for the first
// and "#fp~2" for the next, so the set of ids depends on the text and the
// count of copies only: inserting another declaration above them, or putting
// them in other files in another order, leaves the set as it was, and adding a
// copy adds exactly one id.
func TestPL10_ExactDuplicates(t *testing.T) {
	twin := "func init() { registry[\"t\"] = compute([]int{1, 1, 1}) }\n"
	idsOfInit := func(files map[string]string) []string {
		var out []string
		for _, f := range fingerprintsOfModule(t, files) {
			if f.Declared == "func:example.test/merge.init" {
				out = append(out, f.ID)
			}
		}
		sort.Strings(out)
		return out
	}
	two := idsOfInit(map[string]string{"base.go": mergeBase, "a.go": "package p\n\n" + twin, "b.go": "package p\n\n" + twin})
	if len(two) != 2 || two[0] == two[1] {
		t.Fatalf("two copies of a body: ids %v, want two distinct", two)
	}
	bare, ranked := two[0], two[1]
	if !strings.HasSuffix(ranked, "~2") || strings.Contains(bare, "~") || !strings.HasPrefix(ranked, bare) {
		t.Errorf("ids %v: want the bare fingerprint and the same with ~2", two)
	}
	// Other files, another order: the same set. Another init above them in
	// the same file: the same two ids, and one more for the other init.
	if got := idsOfInit(map[string]string{"base.go": mergeBase, "z.go": "package p\n\n" + twin, "0.go": "package p\n\n" + twin}); !reflect.DeepEqual(got, two) {
		t.Errorf("files renamed and swapped: ids %v, want %v", got, two)
	}
	above := idsOfInit(map[string]string{"base.go": mergeBase, "a.go": "package p\n\n" + initC + twin + twin})
	has := func(ids []string, id string) bool {
		for _, x := range ids {
			if x == id {
				return true
			}
		}
		return false
	}
	if len(above) != 3 || !has(above, bare) || !has(above, ranked) {
		t.Errorf("another init above the copies: ids %v do not hold %v and one more", above, two)
	}
	// A third copy adds one id and renames neither.
	three := idsOfInit(map[string]string{"base.go": mergeBase, "a.go": "package p\n\n" + twin + twin + twin})
	if len(three) != 3 || three[0] != bare || three[1] != ranked || !strings.HasSuffix(three[2], "~3") {
		t.Errorf("three copies: %v, want %s, %s and a ~3", three, bare, ranked)
	}
	// Deterministic: the same text, loaded twice, names the same.
	files := map[string]string{"base.go": mergeBase, "a.go": "package p\n\n" + twin + twin}
	if a, b := idsOfInit(files), idsOfInit(files); !reflect.DeepEqual(a, b) {
		t.Errorf("two loads: %v and %v", a, b)
	}
}

// A local type is told apart from the same type in another function by its
// enclosing declaration, and an enclosing init by its own fingerprint: the
// same local type in two different inits is two symbols, and an edit to
// the type's own definition is a new id.
func TestPL10_LocalTypesCarryTheirOwnerAndDefinition(t *testing.T) {
	src := func(a, b, tdef string) string {
		return mergeBase + "\nfunc init() {\n\ttype T " + tdef + "\n\tregistry[\"" + a + "\"] = len([]T{})\n}\n" +
			"\nfunc init() {\n\ttype T " + tdef + "\n\tregistry[\"" + b + "\"] = len([]T{})\n}\n"
	}
	idsOfT := func(source string) []string {
		p := loadT(t, writeModule(t, map[string]string{"p.go": source}))
		nm := newNamer(p)
		var out []string
		for _, o := range declared(p, "example.test/merge", "T") {
			out = append(out, nm.id(o))
		}
		sort.Strings(out)
		return out
	}
	one := idsOfT(src("x", "y", "struct{ a int }"))
	if len(one) != 2 || one[0] == one[1] {
		t.Fatalf("the same local type in two different inits: ids %v, want two", one)
	}
	for _, id := range one {
		if !strings.Contains(id, "merge.init#") || !strings.Contains(id, "$T#") {
			t.Errorf("id %q does not spell its enclosing init and its own fingerprint", id)
		}
	}
	// Swapping the two inits' places changes nothing.
	swapped := mergeBase + "\nfunc init() {\n\ttype T struct{ a int }\n\tregistry[\"y\"] = len([]T{})\n}\n" +
		"\nfunc init() {\n\ttype T struct{ a int }\n\tregistry[\"x\"] = len([]T{})\n}\n"
	if got := idsOfT(swapped); !reflect.DeepEqual(got, one) {
		t.Errorf("swapped inits: %v, want %v", got, one)
	}
	// Editing the type's definition is a new id.
	if got := idsOfT(src("x", "y", "struct{ a, b int }")); reflect.DeepEqual(got, one) || got[0] == one[0] || got[1] == one[1] {
		t.Errorf("an edited local type kept an id: %v then %v", one, got)
	}
}

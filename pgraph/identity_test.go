package pgraph

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
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
// parameters); names that may repeat in a package carry an ordinal in source
// order. The second column of each row is a spelling the replaced scheme
// produced and this one must not.
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
		{"first init, by file order", one("init", 0), "func:" + core + ".init#1", "func:" + core + ".init#a_init.go@"},
		{"second init", one("init", 1), "func:" + core + ".init#2", "func:" + core + ".init#b_init.go@"},
		{"blank function", one("_", 0), "func:" + core + "._#1", ""},
		{"local type", one("shower", 0), "type:" + core + ".Pairs$shower#1", ""},
		{"same local name, other function", one("shower", 1), "type:" + core + ".Triples$shower#1", ""},
		{"shadowing local, outer", one("pair", 0), "type:" + core + ".Both$pair#1", ""},
		{"shadowing local, inner", one("pair", 1), "type:" + core + ".Both$pair#2", ""},
		{"method of a local interface", methodOf(t, p, core, "shower", "Show"), "method:" + core + ".Pairs$shower#1.Show", ""},
	} {
		got := nm.id(c.obj)
		if got != c.want {
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

// The checker must be able to fail: with the ordinals stripped from the namer,
// the fixture's repeated names collide and are reported: two init functions
// and two nested types spelled pair. (The two local interfaces spelled shower
// are told apart by their enclosing function, with or without an ordinal.)
func TestPL10_InjectivityCheckerCatchesCollisions(t *testing.T) {
	p := loadT(t, filepath.Join("testdata", "idrepo", "commit1"))
	nm := newNamer(p)
	if found, _ := collisions(p, nm); len(found) != 0 {
		t.Fatalf("collisions with the real namer: %v", found)
	}
	nm.ordinal = map[types.Object]int{}
	found, _ := collisions(p, nm)
	if len(found) != 2 {
		t.Errorf("%d collisions reported without ordinals, want 2 (init, Both$pair): %v", len(found), found)
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
	f1, f2 := set(Extract(p1)), set(Extract(p2))
	for _, w := range []Triple{
		{"pkg:" + core, Declares, "func:" + core + ".Compute"},
		{"pkg:" + core, Declares, "type:" + core + ".Queue"},
		{"type:" + core + ".Queue", Declares, "method:" + core + ".Queue.Reset"},
		{"func:" + core + ".Measure", Calls, "method:" + core + ".Sizer.Size"},
		{"func:" + core + ".Pairs", Calls, "method:" + core + ".Pairs$shower#1.Show"},
		{"func:" + core + ".Triples", Calls, "method:" + core + ".Triples$shower#1.Show"},
		{"func:" + core + ".CloseIt", Calls, "method:" + core + ".interface{Close() error}.Close"},
	} {
		if !f1[w] {
			t.Errorf("commit 1 lacks %v", w)
		}
		if !f2[w] {
			t.Errorf("commit 2 lacks %v", w)
		}
	}

	// ACCEPTED GAP: an ordinal says where a symbol is, not which one it is.
	// init#1 is the init of a_init.go at commit 1 and of 0_init.go at commit
	// 2; the id alone cannot tell, and Link (below) is what carries the
	// identity across.
	fp1, fp2 := fingerprintsByID(p1), fingerprintsByID(p2)
	if a, b := fp1["func:"+core+".init#1"], fp2["func:"+core+".init#1"]; a.Body == b.Body {
		t.Error("init#1 kept its body; the fixture does not show the ordinal moving")
	}
}

func fingerprintsByID(p *Program) map[string]Fingerprint {
	m := map[string]Fingerprint{}
	for _, f := range Fingerprints(p) {
		m[f.ID] = f
	}
	return m
}

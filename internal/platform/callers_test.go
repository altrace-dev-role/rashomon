package platform

// ARCH-INLINE1: the blast-radius read. After a Go edit, the other packages
// whose code the log proves calls into the edited file's package, from a
// pair the walk file vouches for, each caller as the sync read it.

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// ai1Log is a log over packages a, b, c, d, e, f, g and yaml.v3 of module
// example.com/m, each the caller of a in one way: b by a PROVEN call, c by
// an ALGORITHMIC one only, d by a may-call only, e by a call retracted, f
// by a package variable's initialiser ("pkg:" subject), g by a PROVEN call
// from a file changed since the sync; a calls itself, and yaml.v3, whose
// path holds a dot, is called by b and calls nothing in a.
func ai1Log() string {
	const m = "example.com/m/"
	rec := func(op, s, p, o, tier string) string {
		return op + "\t" + s + "\t" + p + "\t" + o + "\t" + tier + "\t" + cfg + "\tc1\tgotypes@v2\n"
	}
	return LogHeader + "\n" +
		rec("+", "pkg:"+m+"a", "declares", "func:"+m+"a.F", tierProven) +
		rec("+", "pkg:"+m+"a", "declares", "type:"+m+"a.T", tierProven) +
		rec("+", "type:"+m+"a.T", "declares", "method:"+m+"a.T.M", tierProven) +
		rec("+", "method:"+m+"a.T.M", "calls", "func:"+m+"a.F", tierProven) +
		rec("+", "func:"+m+"b.G", "calls", "func:"+m+"a.F", tierProven) +
		rec("+", "func:"+m+"b.G", "calls", "func:"+m+"yaml.v3.Marshal", tierProven) +
		rec("+", "func:"+m+"c.K", "calls", "func:"+m+"a.F", tierAlgorithmic) +
		rec("+", "func:"+m+"d.H", "may-call", "method:"+m+"a.T.M", tierProven) +
		rec("+", "func:"+m+"e.Old", "calls", "func:"+m+"a.F", tierProven) +
		rec("-", "func:"+m+"e.Old", "calls", "func:"+m+"a.F", tierProven) +
		rec("+", "pkg:"+m+"f", "calls", "func:"+m+"a.F", tierProven) +
		rec("+", "func:"+m+"g.X", "calls", "method:"+m+"a.T.M", tierProven)
}

// ai1Sources is each package's one file as the sync read it.
var ai1Sources = map[string]string{
	"a/a.go": "package a\n", "b/b.go": "package b\n", "c/c.go": "package c\n", "d/d.go": "package d\n",
	"e/e.go": "package e\n", "f/f.go": "package f\n", "g/g.go": "package g\n", "yaml.v3/y.go": "package yaml\n",
}

// ai1Digests is the digests file a sync of ai1Sources writes.
func ai1Digests() string {
	out := DigestsHeader + "\n"
	for _, rel := range []string{"a/a.go", "b/b.go", "c/c.go", "d/d.go", "e/e.go", "f/f.go", "g/g.go", "yaml.v3/y.go"} {
		out += "example.com/m/" + filepath.Dir(rel) + "\t" + rel + "\t" + sumOf(ai1Sources[rel]) + "\n"
	}
	return out
}

// ai1Synced is the module as a sync leaves it, with the walk file vouching
// for the pair, a test file in b's directory, and g's file changed since.
func ai1Synced(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.25\n"))
	for rel, src := range ai1Sources {
		put(t, filepath.Join(root, filepath.FromSlash(rel)), []byte(src))
	}
	put(t, filepath.Join(root, "b", "b_test.go"), []byte("package b\n"))
	put(t, LogPath(root), []byte(ai1Log()))
	put(t, digestsPath(root), []byte(ai1Digests()))
	putWalk(t, root, sumOf(ai1Log()), sumOf(ai1Digests()))
	put(t, filepath.Join(root, "g", "g.go"), []byte("package g\n\nvar changed = 1\n"))
	return root
}

// TestAI1_CallersAreProvenCallsFromFreshPackages: an edit of a/a.go finds b
// and f, the one by a function's PROVEN call and the other by a package
// initialiser's, b with a test file. Not a (the edited package), not c
// (ALGORITHMIC), d (may-call), e (retracted), g (changed since the sync) nor
// yaml.v3 (called, not calling). Break: drop the tier, predicate, self or
// freshness check, and its package is named.
func TestAI1_CallersAreProvenCallsFromFreshPackages(t *testing.T) {
	root := ai1Synced(t)
	site, ok := Locate(root, filepath.Join(root, "a", "a.go"))
	if !ok || site.Package != "example.com/m/a" || site.Log != sumOf(ai1Log()) || site.Root != root {
		t.Fatalf("Locate = %+v, %v; want package a under the vouched log", site, ok)
	}
	got, err := Callers(site, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := []Caller{{Package: "example.com/m/b", Dir: "b", Tests: true}, {Package: "example.com/m/f", Dir: "f"}}
	if !slices.Equal(got, want) {
		t.Errorf("Callers = %+v\nwant %+v", got, want)
	}
}

// TestAI1_LocateNamesNoSiteWhereNoSyncVouches: a test file, a file in no
// package, a file outside the root, and a root with no walk file, or with
// one that vouches for no pair, have no site, so no log is read. Break:
// read a log without the walk file, and an unvouched pair is read.
func TestAI1_LocateNamesNoSiteWhereNoSyncVouches(t *testing.T) {
	root := ai1Synced(t)
	for _, file := range []string{"a/a_test.go", "a/testdata/x.go", "../x/x.go", "a/a.txt"} {
		if s, ok := Locate(root, filepath.Join(root, filepath.FromSlash(file))); ok {
			t.Errorf("Locate(%s) = %+v, want none", file, s)
		}
	}
	bare := t.TempDir()
	put(t, filepath.Join(bare, "go.mod"), []byte("module example.com/m\n\ngo 1.25\n"))
	put(t, filepath.Join(bare, "a", "a.go"), []byte("package a\n"))
	put(t, LogPath(bare), []byte(ai1Log()))
	if s, ok := Locate(bare, filepath.Join(bare, "a", "a.go")); ok {
		t.Errorf("Locate with no walk file = %+v, want none", s)
	}
	put(t, filepath.Join(bare, stateDir, walkFile), []byte(walkHeader+"\nwalk\t"+sumOf("walk")+"\nrefused\n"))
	if s, ok := Locate(bare, filepath.Join(bare, "a", "a.go")); ok {
		t.Errorf("Locate with a refused walk file = %+v, want none", s)
	}
}

// TestAI1_ASyncBetweenLocateAndReadIsNotVouched: the walk file names another
// log by the read (a sync landed between the two), or the digests are not
// the pair's: the read is errNotVouched, never callers from a torn pair.
// Break: skip either check, and the read answers.
func TestAI1_ASyncBetweenLocateAndReadIsNotVouched(t *testing.T) {
	root := ai1Synced(t)
	site, ok := Locate(root, filepath.Join(root, "a", "a.go"))
	if !ok {
		t.Fatal("premise: no site")
	}
	putWalk(t, root, sumOf("another log"), sumOf(ai1Digests()))
	if got, err := Callers(site, time.Second); !errors.Is(err, errNotVouched) {
		t.Errorf("a moved log: Callers = %+v, %v; want errNotVouched", got, err)
	}
	putWalk(t, root, sumOf(ai1Log()), sumOf("other digests"))
	if got, err := Callers(site, time.Second); !errors.Is(err, errNotVouched) {
		t.Errorf("unvouched digests: Callers = %+v, %v; want errNotVouched", got, err)
	}
}

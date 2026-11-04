package platform

// PW1b-ON-230 step 3 (WED-SLICE row 1(c)): the prompt's read of the
// platform graph. A name the request named reaches the model only as what
// the log proves of it: the package that declares it and its direct
// callers, from a pair the walk file vouches for, with every package the
// answer rests on still as the sync read it.

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"testing"
)

// symLog is a log over packages a, b, c and yaml.v3 (a path whose last
// element holds a dot) of module example.com/m. F is called by a method of
// a, by b and by c (once under each of two configs), and also by an
// ALGORITHMIC record and a retracted one; T.M has only a may-call, PROVEN
// so that the predicate, not the tier, is what keeps it out; Read is
// declared in three packages and Pair in two; b's HasF ends in F.
func symLog() string {
	const a, b, c, y = "example.com/m/a", "example.com/m/b", "example.com/m/c", "example.com/m/yaml.v3"
	rec := func(op, s, p, o, tier, config string) string {
		return op + "\t" + s + "\t" + p + "\t" + o + "\t" + tier + "\t" + config + "\tc1\tgotypes@v2\n"
	}
	const darwin = "darwin/arm64/cgo=1/tags="
	return LogHeader + "\n" +
		rec("+", "pkg:"+a, "declares", "func:"+a+".F", tierProven, cfg) +
		rec("+", "pkg:"+a, "declares", "type:"+a+".T", tierProven, cfg) +
		rec("+", "type:"+a+".T", "declares", "method:"+a+".T.M", tierProven, cfg) +
		rec("+", "pkg:"+a, "declares", "func:"+a+".Read", tierProven, cfg) +
		rec("+", "pkg:"+b, "declares", "func:"+b+".G", tierProven, cfg) +
		rec("+", "pkg:"+b, "declares", "func:"+b+".Read", tierProven, cfg) +
		rec("+", "pkg:"+b, "declares", "func:"+b+".Pair", tierProven, cfg) +
		rec("+", "pkg:"+b, "declares", "func:"+b+".HasF", tierProven, cfg) +
		rec("+", "pkg:"+c, "declares", "func:"+c+".H", tierProven, cfg) +
		rec("+", "pkg:"+c, "declares", "func:"+c+".K", tierProven, cfg) +
		rec("+", "pkg:"+c, "declares", "func:"+c+".Read", tierProven, cfg) +
		rec("+", "pkg:"+c, "declares", "func:"+c+".Pair", tierProven, cfg) +
		rec("+", "pkg:"+y, "declares", "func:"+y+".Marshal", tierProven, cfg) +
		rec("+", "method:"+a+".T.M", "calls", "func:"+a+".F", tierProven, cfg) +
		rec("+", "func:"+b+".G", "calls", "func:"+a+".F", tierProven, cfg) +
		rec("+", "func:"+c+".H", "calls", "func:"+a+".F", tierProven, cfg) +
		rec("+", "func:"+c+".H", "calls", "func:"+a+".F", tierProven, darwin) +
		rec("+", "func:"+c+".K", "calls", "func:"+a+".F", tierAlgorithmic, cfg) +
		rec("+", "func:"+b+".Old", "calls", "func:"+a+".F", tierProven, cfg) +
		rec("-", "func:"+b+".Old", "calls", "func:"+a+".F", tierProven, cfg) +
		rec("+", "func:"+b+".G", "may-call", "method:"+a+".T.M", tierProven, cfg) +
		rec("+", "func:"+b+".G", "calls", "func:"+y+".Marshal", tierProven, cfg)
}

// symSources is each package's one file as the sync read it.
var symSources = map[string]string{
	"a/a.go": "package a\n", "b/b.go": "package b\n", "c/c.go": "package c\n", "yaml.v3/y.go": "package yaml\n",
}

// symSynced is the module as a sync leaves it: the sources, symLog, their
// digests and, when walk is set, the walk file that vouches for the pair.
func symSynced(t *testing.T, walk bool) string {
	t.Helper()
	root := t.TempDir()
	put(t, filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.25\n"))
	for rel, src := range symSources {
		put(t, filepath.Join(root, filepath.FromSlash(rel)), []byte(src))
	}
	put(t, LogPath(root), []byte(symLog()))
	put(t, digestsPath(root), []byte(symDigests()))
	if walk {
		putWalk(t, root, sumOf(symLog()), sumOf(symDigests()))
	}
	return root
}

// symDigests is the digests file a sync of symSources writes.
func symDigests() string {
	digests := DigestsHeader + "\n"
	for _, rel := range []string{"a/a.go", "b/b.go", "c/c.go", "yaml.v3/y.go"} {
		digests += "example.com/m/" + path.Dir(rel) + "\t" + rel + "\t" + sumOf(symSources[rel]) + "\n"
	}
	return digests
}

// putWalk writes root's walk file naming log and digests as the pair.
func putWalk(t *testing.T, root, log, digests string) {
	t.Helper()
	put(t, filepath.Join(root, stateDir, walkFile), []byte(walkHeader+"\nwalk\t"+sumOf("walk")+"\nlog\t"+log+"\ndigests\t"+digests+"\n"))
}

func symbolsText(syms []Symbol) []string {
	out := []string{}
	for _, s := range syms {
		out = append(out, fmt.Sprintf("%s in %s: %d callers in %d packages", s.Name, s.Dir, s.Callers, s.CallerPackages))
	}
	return out
}

// TestPW1b3_SymbolsNamesTheDeclaringPackageAndItsDirectCallers: each name
// is matched after its package, bare or with its package's last element,
// and a method as T.M; callers are distinct PROVEN calls subjects whatever
// their config, never an ALGORITHMIC record, a may-call or a retracted
// call; a package path with a dot in its last element is still found; a
// name declared in two packages is both, in three is left out (which one
// the request meant is a guess); HasF is not F. Break: count every tier,
// or every predicate, or a call once per config, or match F inside HasF,
// and this fails.
func TestPW1b3_SymbolsNamesTheDeclaringPackageAndItsDirectCallers(t *testing.T) {
	root := symSynced(t, true)
	for _, tc := range []struct {
		names []string
		want  []string
	}{
		{[]string{"F"}, []string{"F in a: 3 callers in 3 packages"}},
		{[]string{"a.F"}, []string{"F in a: 3 callers in 3 packages"}},
		{[]string{"T.M"}, []string{"T.M in a: 0 callers in 0 packages"}},
		{[]string{"Marshal"}, []string{"Marshal in yaml.v3: 1 callers in 1 packages"}},
		{[]string{"Pair"}, []string{"Pair in b: 0 callers in 0 packages", "Pair in c: 0 callers in 0 packages"}},
		{[]string{"Read", "Nope", "T"}, []string{"T in a: 0 callers in 0 packages"}},
	} {
		if got := symbolsText(Symbols(context.Background(), root, tc.names)); !slices.Equal(got, tc.want) {
			t.Errorf("Symbols(%q) = %q, want %q", tc.names, got, tc.want)
		}
	}
}

// TestPW1b3_AStaleCallerPackageWithholdsTheSymbol: a file of a package the
// answer rests on, the declaring one or a caller's, that no longer reads as
// the sync read it withholds that symbol, since a caller count from it is
// a guess; a symbol that rests only on fresh packages is still given.
// Break: check the declaring package alone, and F, called from the edited
// c, is still given.
func TestPW1b3_AStaleCallerPackageWithholdsTheSymbol(t *testing.T) {
	root := symSynced(t, true)
	put(t, filepath.Join(root, "c", "c.go"), []byte("package c\n\nfunc H() {}\n"))
	got := symbolsText(Symbols(context.Background(), root, []string{"F", "T.M", "Marshal"}))
	want := []string{"T.M in a: 0 callers in 0 packages", "Marshal in yaml.v3: 1 callers in 1 packages"}
	if !slices.Equal(got, want) {
		t.Errorf("after an edit to c: %q, want %q", got, want)
	}
}

// TestPW1b3_OnlyAVouchedPairProvesASymbol: with no walk file, or one that
// names another log or other digests, nothing is given. The Stop's read
// takes a pair with no walk file (a pgraph from before it); a line to the
// model does not. Control: the same files with the walk file give F.
// Break: skip the vouch, or take a missing walk file as vouching, and the
// three refused rows give F.
func TestPW1b3_OnlyAVouchedPairProvesASymbol(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) string
		want  int
	}{
		{"vouched", func(t *testing.T) string { return symSynced(t, true) }, 1},
		{"no walk file", func(t *testing.T) string { return symSynced(t, false) }, 0},
		{"another log", func(t *testing.T) string {
			root := symSynced(t, false)
			putWalk(t, root, sumOf("another log"), sumOf(symDigests()))
			return root
		}, 0},
		{"other digests", func(t *testing.T) string {
			root := symSynced(t, false)
			putWalk(t, root, sumOf(symLog()), sumOf("other digests"))
			return root
		}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Symbols(context.Background(), tc.setup(t), []string{"F"}); len(got) != tc.want {
				t.Errorf("%d symbols for F, want %d: %q", len(got), tc.want, symbolsText(got))
			}
		})
	}
}

// TestPW1b3_TheReadLooksUpFourNamesAtMost: a request's fifth name and on
// cost nothing. Control: F as the first name is given. Break: drop the cap.
func TestPW1b3_TheReadLooksUpFourNamesAtMost(t *testing.T) {
	root := symSynced(t, true)
	if got := Symbols(context.Background(), root, []string{"F", "N1", "N2", "N3"}); len(got) != 1 {
		t.Errorf("F first: %q, want F", symbolsText(got))
	}
	if got := Symbols(context.Background(), root, []string{"N1", "N2", "N3", "N4", "F"}); len(got) != 0 {
		t.Errorf("F fifth: %q, want nothing", symbolsText(got))
	}
}

// TestPW1b3_ADroppedRecordCostsTheNameMatchNothing: the prompt's decode,
// matching names on every record's object, allocates as much over 1,024
// records none of which it keeps as over 2, so the prompt's memory follows
// the named symbols' records, not the log. Break: convert the object to a
// string to match it, and the larger log allocates once a record.
func TestPW1b3_ADroppedRecordCostsTheNameMatchNothing(t *testing.T) {
	keep := keepNamed([]string{"Nope", "a.Nope", "T.Nope", "F9"}) // synthLog spells none
	allocs := func(n int) float64 {
		b := synthLog(t, n)
		return allocsNoGC(5, func() {
			if _, err := decode(context.Background(), bytes.NewReader(b), nil, keep); err != nil {
				t.Fatal(err)
			}
		})
	}
	if small, big := allocs(2), allocs(4*checkEvery); big != small {
		t.Errorf("a decode that keeps no named symbol: %v allocations for %d records, %v for 2", big, 4*checkEvery, small)
	}
}

// TestPW1b3_PackageInTakesTheOnePackageItsKindFits: an id's package is the
// one listed package whose rest has the id kind's shape, and none where two
// fit. Cases: a/b declares a type c with a method X beside a package a/b.c
// (eng-3's), a last element with a dot beside its own prefix, a type
// declared in a method, and a method two packages could hold. Break: take
// the longest listed prefix, and the first and last cases name the wrong
// package.
func TestPW1b3_PackageInTakesTheOnePackageItsKindFits(t *testing.T) {
	dg := &digests{byPkg: map[string][]string{"a/b": nil, "a/b.c": nil, "gopkg.in/yaml": nil, "gopkg.in/yaml.v3": nil}}
	for id, want := range map[string]string{
		"method:a/b.c.X":                "a/b",
		"func:a/b.c.X":                  "a/b.c",
		"func:gopkg.in/yaml.v3.Marshal": "gopkg.in/yaml.v3",
		"type:a/b.T.M$Local":            "a/b",
		"type:a/b.F$Local":              "a/b",
		"func:x/y.F":                    "",
		"pkg:a/b":                       "",
		"method:a/b.c.T.X":              "",
	} {
		if got := packageIn(dg, id); got != want {
			t.Errorf("packageIn(%q) = %q, want %q", id, got, want)
		}
	}
}

package pgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// writeTree writes files (slash paths to contents) under a fresh directory,
// symlink-resolved, and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// twoModules is a root module and a nested one, each with an interface call
// CHA resolves to a method in it, so both tiers have facts in both.
func twoModules(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"go.mod":       "module example.com/r\n\ngo 1.25.0\n",
		"p/p.go":       "package p\n\ntype I interface{ M() }\n\ntype T struct{}\n\nfunc (T) M() {}\n\nfunc Call(i I) { i.M() }\n",
		"p/p_test.go":  "package p\n",
		"sub/go.mod":   "module example.com/s\n\ngo 1.25.0\n",
		"sub/q/q.go":   "package q\n\ntype J interface{ N() }\n\ntype U struct{}\n\nfunc (U) N() {}\n\nfunc Call(j J) { j.N() }\n",
		"testdata/x/x": "not go\n",
	})
}

// TestPLW1a_ScanRootsFindsTheModulesASyncLoads (card PL-WIRE1 a): every
// go.mod at or under the root, root first, then in path order; testdata,
// vendor, dot and underscore directories are not walked; none is an error,
// and so is one past MaxModules. Break: walk the skipped directories, and a
// fixture module becomes a scan root.
func TestPLW1a_ScanRootsFindsTheModulesASyncLoads(t *testing.T) {
	gomod := "module example.com/x\n"
	root := writeTree(t, map[string]string{
		"go.mod": gomod, "sub/go.mod": gomod, "a/b/go.mod": gomod,
		"testdata/m/go.mod": gomod, "vendor/v/go.mod": gomod, ".rashomon/go.mod": gomod, "_tools/go.mod": gomod,
		"a/b/testdata/go.mod": gomod,
	})
	got, err := ScanRoots(root)
	want := []string{root, filepath.Join(root, "a", "b"), filepath.Join(root, "sub")}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("ScanRoots = %q, %v; want %q", got, err, want)
	}
	if got, err := ScanRoots(writeTree(t, map[string]string{"main.go": "package main\n"})); err == nil {
		t.Errorf("a tree with no go.mod: %q, want an error", got)
	}
	many := map[string]string{}
	for i := 0; i <= MaxModules; i++ {
		many["m"+strconv.Itoa(i)+"/go.mod"] = gomod
	}
	if got, err := ScanRoots(writeTree(t, many)); err == nil {
		t.Errorf("%d modules: %d roots, want an error past MaxModules", MaxModules+1, len(got))
	}
}

// TestPLW1b_SyncProgramsKeepsEveryModulesFacts: one reconcile per tier over
// all programs keeps both modules' PROVEN and ALGORITHMIC facts live, where
// syncing one program after the other retracts the first one's. Programs
// under two configs are refused before anything is written. Break: reconcile
// per program, and the root module's facts are tombstoned by the nested
// module's pass.
func TestPLW1b_SyncProgramsKeepsEveryModulesFacts(t *testing.T) {
	root := twoModules(t)
	r, s := loadDepsT(t, root), loadDepsT(t, filepath.Join(root, "sub"))
	var l Log
	pd, ad, err := SyncPrograms(&l, []*Program{r, s}, "c1")
	if err != nil || pd.Asserted == 0 || ad.Asserted != 2 || pd.Retracted+ad.Retracted != 0 {
		t.Fatalf("SyncPrograms: %+v %+v %v; want PROVEN facts and two may-call facts asserted, none retracted", pd, ad, err)
	}
	live := map[Triple]Tier{}
	for _, f := range l.Live() {
		live[f.Triple()] = f.Tier()
	}
	for _, want := range []struct {
		tr   Triple
		tier Tier
	}{
		{Triple{"pkg:example.com/r/p", Declares, "func:example.com/r/p.Call"}, Proven},
		{Triple{"pkg:example.com/s/q", Declares, "func:example.com/s/q.Call"}, Proven},
		{Triple{"func:example.com/r/p.Call", MayCall, "method:example.com/r/p.T.M"}, Algorithmic},
		{Triple{"func:example.com/s/q.Call", MayCall, "method:example.com/s/q.U.N"}, Algorithmic},
	} {
		if live[want.tr] != want.tier {
			t.Errorf("live %v at %v, want %v", want.tr, live[want.tr], want.tier)
		}
	}
	// The same programs again append nothing; one of them alone retracts the other's.
	if pd, ad, err := SyncPrograms(&l, []*Program{r, s}, "c2"); err != nil || pd != (Delta{}) || ad != (Delta{}) {
		t.Errorf("an unchanged resync: %+v %+v %v, want no records", pd, ad, err)
	}
	if pd, _, err := SyncPrograms(&l, []*Program{s}, "c3"); err != nil || pd.Retracted == 0 {
		t.Errorf("one program alone: %+v %v, want the other's facts retracted (why SyncPrograms takes them all)", pd, err)
	}
	n := l.Len()
	other := &Program{cfg: Config{GOOS: "plan9", GOARCH: "amd64"}, deps: true}
	if _, _, err := SyncPrograms(&l, []*Program{r, other}, "c4"); err == nil || l.Len() != n {
		t.Errorf("two configs: %v, %d records appended; want an error and none", err, l.Len()-n)
	}
	if _, _, err := SyncPrograms(&l, nil, "c5"); err == nil {
		t.Error("no program: want an error, not a retraction of every fact")
	}
}

// TestPLW1c_DigestsNameEveryCompiledFile: the digests file holds one line
// per Go file a program compiled, its path re-rooted at the sync root, its
// package, and the sha256 of its bytes (an independent hash here); no test
// file. Break: leave the path relative to the module, and the nested
// module's file is named as if it sat at the root.
func TestPLW1c_DigestsNameEveryCompiledFile(t *testing.T) {
	root := twoModules(t)
	b, err := EncodeDigests(root, []*Program{loadDepsT(t, root), loadDepsT(t, filepath.Join(root, "sub"))})
	if err != nil {
		t.Fatal(err)
	}
	sum := func(rel string) string {
		h := sha256.Sum256(mustRead(t, filepath.Join(root, filepath.FromSlash(rel))))
		return hex.EncodeToString(h[:])
	}
	want := DigestsHeader + "\n" +
		"example.com/r/p\tp/p.go\t" + sum("p/p.go") + "\n" +
		"example.com/s/q\tsub/q/q.go\t" + sum("sub/q/q.go") + "\n"
	if string(b) != want {
		t.Errorf("digests:\n%s\nwant:\n%s", b, want)
	}
}

// sumOf is the hex sha256 of s.
func sumOf(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// TestPLW1d_DigestsFormat: encodeDigests writes the header, then one line
// per file in path order, "package path sha256", tab-separated, and refuses a
// field the format cannot carry. internal/platform's parser is held to the
// same lines from its side (TestPLW1j_), and H-156 holds the two together
// through the real binary. Break: write the lines in input order, and the
// file's bytes depend on the walk.
func TestPLW1d_DigestsFormat(t *testing.T) {
	got, err := encodeDigests([]digestLine{
		{"example.com/m/b", "b/b.go", sumOf("package b\n")},
		{"example.com/m/a", "a/a.go", sumOf("package a\n")},
	})
	want := DigestsHeader + "\n" +
		"example.com/m/a\ta/a.go\t" + sumOf("package a\n") + "\n" +
		"example.com/m/b\tb/b.go\t" + sumOf("package b\n") + "\n"
	if err != nil || string(got) != want {
		t.Errorf("encodeDigests = %q, %v\nwant %q", got, err, want)
	}
	for _, bad := range []digestLine{{"p", "a\tb.go", "x"}, {"p", "a.go", ""}, {"p\n", "a.go", "x"}} {
		if b, err := encodeDigests([]digestLine{bad}); err == nil {
			t.Errorf("%q: encoded %q, want a refusal", bad, b)
		}
	}
}

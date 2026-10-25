package pgraph

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPL1_ResolvedSetEqualsGoList(t *testing.T) {
	// The repo root is resolved inside its subtest so a skip does not hide
	// the testdata case.
	for name, root := range map[string]string{"testdata": testModule, "repo": ""} {
		t.Run(name, func(t *testing.T) {
			if name == "repo" {
				root = repoRoot(t)
			}
			got := loadT(t, root).PackagePaths()
			want := goList(t, root)
			if len(want) == 0 {
				t.Fatal("oracle returned no packages")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("resolved set != go list ./...\n got=%v\nwant=%v", got, want)
			}
		})
	}
	// The package whose every file is excluded by build constraints is not
	// in go list's answer and so not in ours.
	for _, p := range loadT(t, testModule).PackagePaths() {
		if p == "example.test/mod/c" {
			t.Error("fully constrained package resolved")
		}
	}
}

func TestPL1_FilesKeyedByPathAndContentHash(t *testing.T) {
	dir := copyTree(t, testModule)
	p1 := loadT(t, dir)
	files := p1.Files()
	paths := map[string]string{}
	for _, f := range files {
		paths[f.Path] = f.SHA256
	}
	for _, want := range []string{"go.mod", "a/a.go", "b/b.go"} {
		if len(paths[want]) != 64 {
			t.Errorf("missing or malformed hash for %s: %q", want, paths[want])
		}
	}
	for _, not := range []string{"a/skip.go", "a/a_test.go", "c/c.go"} {
		if _, ok := paths[not]; ok {
			t.Errorf("%s is outside the build configuration but was ingested", not)
		}
	}
	for i := 1; i < len(files); i++ {
		if files[i-1].Path >= files[i].Path {
			t.Fatalf("files not strictly sorted at %d", i)
		}
	}

	// mtime does not reach the key.
	old := time.Unix(1_000_000_000, 0)
	if err := os.Chtimes(filepath.Join(dir, "a", "a.go"), old, old); err != nil {
		t.Fatal(err)
	}
	if loadT(t, dir).Digest() != p1.Digest() {
		t.Fatal("mtime changed the digest")
	}
	// Content does, and only that file's hash moves.
	src, _ := os.ReadFile(filepath.Join(dir, "b", "b.go"))
	os.WriteFile(filepath.Join(dir, "b", "b.go"), append(src, "\n// edit\n"...), 0o644)
	p3 := loadT(t, dir)
	if p3.Digest() == p1.Digest() {
		t.Fatal("content change left the digest")
	}
	for _, f := range p3.Files() {
		if (f.SHA256 != paths[f.Path]) != (f.Path == "b/b.go") {
			t.Errorf("hash of %s moved=%v", f.Path, f.SHA256 != paths[f.Path])
		}
	}
}

func TestPL1_NormalisePath(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "x"), 0o755)
	os.WriteFile(filepath.Join(root, "x", "f.go"), nil, 0o644)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "g.go"), nil, 0o644)
	os.Symlink(filepath.Join(outside, "g.go"), filepath.Join(root, "link.go"))
	for _, c := range []struct {
		abs  string
		want string
		ok   bool
	}{
		{filepath.Join(root, "x", "f.go"), "x/f.go", true},
		{filepath.Join(root, "x", "..", "x", "f.go"), "x/f.go", true},
		{filepath.Join(outside, "g.go"), "", false},
		{filepath.Join(root, "link.go"), "", false}, // resolves outside the root
		{filepath.Join(root, "missing.go"), "", false},
	} {
		got, err := normalisePath(root, c.abs)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("normalisePath(%s) = %q, %v; want %q ok=%v", c.abs, got, err, c.want, c.ok)
		}
	}
}

func TestPL1_CapsAndFailClosed(t *testing.T) {
	dir := copyTree(t, testModule)
	// A valid Go file one byte over the cap, so only the cap can refuse it.
	big := "package a\n//" + strings.Repeat("x", MaxSourceFileBytes)
	os.WriteFile(filepath.Join(dir, "a", "big.go"), []byte(big), 0o644)
	if _, err := Load(t.Context(), dir, Config{}); err == nil || !strings.Contains(err.Error(), "source cap") {
		t.Errorf("over-cap source file: %v", err)
	}
	// Exactly at the cap is accepted.
	os.WriteFile(filepath.Join(dir, "a", "big.go"), []byte(big[:MaxSourceFileBytes]), 0o644)
	if _, err := Load(t.Context(), dir, Config{}); err != nil {
		t.Errorf("at-cap source file refused: %v", err)
	}
	os.Remove(filepath.Join(dir, "a", "big.go"))
	os.WriteFile(filepath.Join(dir, "a", "bad.go"), []byte("package a\nfunc Bad() { undefined() }\n"), 0o644)
	if _, err := Load(t.Context(), dir, Config{}); err == nil {
		t.Error("type error did not fail the load")
	}
	if _, err := Load(t.Context(), t.TempDir(), Config{}); err == nil {
		t.Error("root without go.mod accepted")
	}
	for _, bad := range []Config{{Tags: []string{"a b"}}, {Tags: []string{"-x"}}, {GOOS: "li nux"}, {GOARCH: "../x"}} {
		if _, err := Load(t.Context(), testModule, bad); err == nil {
			t.Errorf("config %+v accepted", bad)
		}
	}
}

func TestPL1_ConfigCanonical(t *testing.T) {
	a := Config{GOOS: "linux", GOARCH: "amd64", Tags: []string{"b", "a", "b"}}.String()
	b := Config{GOOS: "linux", GOARCH: "amd64", Tags: []string{"a", "b"}}.String()
	if a != b || a != "linux/amd64/cgo=0/tags=a,b" {
		t.Errorf("canonical = %q, %q", a, b)
	}
}

// A dependency that does not type-check fails Load: a body that is missing
// would read as a dependency that calls nothing. The scan-root load, which
// reads dependencies from export data and does not look at their bodies, does
// not see it.
func TestPL3_LoadFailsClosedOnADependencyError(t *testing.T) {
	base := t.TempDir()
	write := func(path, src string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(base, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, path), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("dep/go.mod", "module example.test/dep\n\ngo 1.25.0\n")
	write("dep/dep.go", "package dep\n\nfunc F() int { return undefinedThing }\n\nfunc G() int { return 1 }\n")
	write("main/go.mod", "module example.test/main\n\ngo 1.25.0\n\nrequire example.test/dep v0.0.0\n\nreplace example.test/dep => "+filepath.Join(base, "dep")+"\n")
	write("main/main.go", "package main\n\nimport \"example.test/dep\"\n\nfunc main() { _ = dep.G() }\n")
	root := filepath.Join(base, "main")
	if _, err := LoadScanRoot(t.Context(), root, Config{}); err != nil {
		t.Fatalf("the scan-root load refused it: %v", err)
	}
	_, err := Load(t.Context(), root, Config{})
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), "dependency example.test/dep") || !strings.Contains(err.Error(), "undefinedThing") {
		t.Fatalf("Load = %v, want a LoadError naming the dependency and its error", err)
	}
}

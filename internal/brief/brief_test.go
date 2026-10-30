package brief

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// project is a directory holding files, each path slash-separated under it,
// with the user's Claude directory moved to an empty one so no file of the
// machine running the test is read.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// specAt is a compiled repository spec at root's spec path with trust tr,
// declaring a test gate of argv.
func specAt(root string, tr spec.Trust, argv ...string) *spec.Compiled {
	yes := true
	return &spec.Compiled{
		Sources: []spec.Source{{Path: filepath.Join(root, ".claude", "rashomon", "spec.json"), Digest: "sha256:ab12cd34ef56", Trust: tr}},
		Digest:  "sha256:ab12cd34ef56", Trust: tr,
		Gates: []spec.Declared{{ID: "spec:unit", Gate: spec.Gate{ID: "unit", Kind: "test", Argv: argv, Match: "prefix", Required: &yes}}},
	}
}

// namesRoot reports whether line is prefix and then root, or "..." and a
// tail of root where root is too long for the line.
func namesRoot(line, prefix, root string) bool {
	rest, ok := strings.CutPrefix(line, prefix)
	tail, cut := strings.CutPrefix(rest, "...")
	return ok && (rest == root || cut && tail != "" && strings.HasSuffix(root, tail))
}

// fact is b's fact of kind k; the test fails without one.
func fact(t *testing.T, b Brief, k string) Fact {
	t.Helper()
	for _, f := range b.Facts {
		if f.Kind == k {
			return f
		}
	}
	t.Fatalf("no %s fact in %+v", k, b.Facts)
	return Fact{}
}

// TestBR1_ThreeFactsOnARepoWithInstructionsAndATestCommand: a repository with
// CLAUDE.md, go.mod and a trusted spec gate gets an instructions line naming
// the file and its size, a build line naming go.mod and its module, and a
// tests line quoting the gate's argv with its trust; the rendered brief is at
// most eight lines of at most 80 columns, its first naming the root (its
// tail, where the root is too long for the line).
func TestBR1_ThreeFactsOnARepoWithInstructionsAndATestCommand(t *testing.T) {
	root := project(t, map[string]string{"CLAUDE.md": "# rules\n", "go.mod": "module github.com/x/y\n\ngo 1.25\n"})
	b := Build(root, "", Pointer{}, specAt(root, spec.Trusted, "go", "test", "./..."))
	if got := fact(t, b, KindInstructions).Text; got != "CLAUDE.md 8 B" {
		t.Errorf("instructions %q, want %q", got, "CLAUDE.md 8 B")
	}
	if got := fact(t, b, KindBuild).Text; got != "go.mod (module github.com/x/y)" {
		t.Errorf("build %q, want %q", got, "go.mod (module github.com/x/y)")
	}
	if got, want := fact(t, b, KindTests).Text, `spec declares "go test ./..." (trusted)`; got != want {
		t.Errorf("tests %q, want %q", got, want)
	}
	if !slices.Equal(b.Instructions, []string{"CLAUDE.md"}) || !slices.Equal(b.Manifests, []string{"go.mod"}) || b.Module != "github.com/x/y" {
		t.Errorf("fields Instructions %q, Manifests %q, Module %q; want CLAUDE.md, go.mod, github.com/x/y", b.Instructions, b.Manifests, b.Module)
	}
	lines := strings.Split(Render(b), "\n")
	if len(lines) > 8 || !namesRoot(lines[0], "rashomon brief: ", root) {
		t.Fatalf("rendered %d lines, first %q; want at most 8, the first naming %s", len(lines), lines[0], root)
	}
	for _, l := range lines {
		if utf8.RuneCountInString(l) > 80 {
			t.Errorf("line over 80 columns: %q", l)
		}
	}
}

// TestBR1b_NothingIsGuessed: a directory with Go and Python sources and no
// manifest, no spec and no index says none on every line: no manifest is
// invented from file extensions, and no package count without go.mod.
func TestBR1b_NothingIsGuessed(t *testing.T) {
	root := project(t, map[string]string{"main.go": "package main\n", "app.py": "print(1)\n", "lib/x.rs": "fn x() {}\n"})
	want := map[string]string{
		KindInstructions: "none (CLAUDE.md, .claude/CLAUDE.md, AGENTS.md, user CLAUDE.md)",
		KindBuild:        "no manifest at the root",
		KindTests:        "no spec gate; no test runner seen in this project yet",
		KindSpec:         "none",
		KindTask:         "none",
		KindPackages:     "not read (no go.mod)",
		KindVerify:       "rashomon status for coverage; rashomon report --session ID",
	}
	b := Build(root, "", Pointer{}, nil)
	if len(b.Facts) != len(want) {
		t.Fatalf("facts %+v, want one per kind of %v", b.Facts, want)
	}
	for k, w := range want {
		if got := fact(t, b, k).Text; got != w {
			t.Errorf("%s %q, want %q", k, got, w)
		}
	}
}

// TestBR1c_TheWalkIsBounded: a module whose package directory holds 3,000
// directories is walked to the entry cap and no further, and says so; a clock
// past the walk's budget stops it at the next read; a directory below the
// depth bound is not counted.
func TestBR1c_TheWalkIsBounded(t *testing.T) {
	files := map[string]string{"go.mod": "module m\n"}
	for i := range 3000 {
		files[fmt.Sprintf("pkg/d%04d/a.go", i)] = "package d\n"
	}
	root := project(t, files)
	t0 := time.Unix(0, 0)
	still := func() time.Time { return t0 }
	if _, entries, capped := walkGo(root, still); !capped || entries != MaxWalkEntries {
		t.Fatalf("walk read %d entries, capped %v; want it stopped at %d", entries, capped, MaxWalkEntries)
	}
	if b := Build(root, "", Pointer{}, nil); !strings.HasPrefix(fact(t, b, KindPackages).Text, "walk capped") || !b.WalkCapped {
		t.Errorf("packages %q, WalkCapped %v; want the walk said to be capped", fact(t, b, KindPackages).Text, b.WalkCapped)
	}
	calls := 0
	late := func() time.Time {
		calls++
		if calls == 1 {
			return t0
		}
		return t0.Add(WalkBudget)
	}
	if _, entries, capped := walkGo(root, late); !capped || entries >= MaxWalkEntries {
		t.Errorf("past its budget the walk read %d entries, capped %v; want it stopped before the entry cap", entries, capped)
	}
	deep := project(t, map[string]string{"go.mod": "module m\n", "a/x.go": "package a\n",
		"b/c/d/e/y.go": "package e\n", "b/c/d/e/f/z.go": "package f\n"})
	if dirs, _, capped := walkGo(deep, still); len(dirs) != 2 || capped {
		t.Errorf("walk found %q, capped %v; want a and b/c/d/e, depth 4, not b/c/d/e/f", dirs, capped)
	}
	if b := Build(deep, "", Pointer{}, nil); !slices.Equal(b.Dirs, []string{"a", "b/c/d/e"}) || b.GoDirs != 2 || b.WalkCapped {
		t.Errorf("Dirs %q, GoDirs %d, WalkCapped %v; want a and b/c/d/e, 2, uncapped", b.Dirs, b.GoDirs, b.WalkCapped)
	}
}

// TestBR1d_EveryLineNamesItsSource: every fact, in its found form and in its
// none form, carries the file or record it was read from.
func TestBR1d_EveryLineNamesItsSource(t *testing.T) {
	root := project(t, map[string]string{"CLAUDE.md": "x", "go.mod": "module m\n", "a/x.go": "package a\n"})
	sp := specAt(root, spec.Trusted, "go", "test", "./...")
	sp.Task = &spec.Task{ID: "t1", Goal: "ship the brief", Decision: "d"}
	store := t.TempDir()
	idx := &verify.ProjectIndex{ProjectKey: root, Entries: []verify.IndexEntry{
		{Family: "r@l", Kind: "test", Sessions: 3, LastSession: "s3", FirstSeen: 1, LastSeen: time.Now().UnixMilli()}}}
	if _, err := verify.WriteIndex(verify.IndexPath(store, root), idx); err != nil {
		t.Fatal(err)
	}
	empty := project(t, nil)
	for name, b := range map[string]Brief{"found": Build(root, store, Pointer{SessionID: "s1"}, sp), "none": Build(empty, "", Pointer{}, nil)} {
		if len(b.Facts) != 7 {
			t.Fatalf("%s: facts %+v, want seven", name, b.Facts)
		}
		for _, f := range b.Facts {
			if f.Source == "" {
				t.Errorf("%s: the %s line %q names no source", name, f.Kind, f.Text)
			}
		}
	}
}

// TestBR1h_UntrustedSpecSaysSo: an untrusted repository spec's line says so
// and names the verb that trusts it, and its gates are quoted as untrusted.
func TestBR1h_UntrustedSpecSaysSo(t *testing.T) {
	root := project(t, map[string]string{"go.mod": "module m\n"})
	b := Build(root, "", Pointer{}, specAt(root, spec.Untrusted, "go", "test", "./..."))
	if got, want := fact(t, b, KindSpec).Text, ".claude/rashomon/spec.json untrusted: run rashomon spec trust"; got != want {
		t.Errorf("spec %q, want %q", got, want)
	}
	if got := fact(t, b, KindTests).Text; !strings.HasSuffix(got, "(untrusted)") {
		t.Errorf("tests %q, want the gate marked untrusted", got)
	}
}

// TestBR1j_TheVerifyLineNamesItsOriginsCommand: the verify line names the
// plugin's slash command inside the plugin (D175), even with a session to
// name, the CLI's report of the payload's session from a settings install,
// and the status verb and the report's flag at a terminal; each fits its line.
func TestBR1j_TheVerifyLineNamesItsOriginsCommand(t *testing.T) {
	root := project(t, nil)
	id := "0b6c5f0e-8d6a-4c43-9a51-3f1f4e2a7c9d"
	for _, c := range []struct {
		report     Pointer
		text, from string
	}{
		{Pointer{Plugin: true, SessionID: id}, "/autosteer:report last", "the plugin's report command"},
		{Pointer{SessionID: id}, "rashomon report --session " + id, "the payload's session_id"},
		{Pointer{}, "rashomon status for coverage; rashomon report --session ID", "the rashomon command"},
	} {
		f := fact(t, Build(root, "", c.report, nil), KindVerify)
		if f.Text != c.text || f.Source != c.from {
			t.Errorf("%+v: verify %q from %q, want %q from %q", c.report, f.Text, f.Source, c.text, c.from)
		}
		if line := "  verify: " + f.Text; utf8.RuneCountInString(line) > Width {
			t.Errorf("%+v: %q is over %d columns", c.report, line, Width)
		}
	}
}

// TestBR1k_AnUnsignedBindingsFileSaysSo: under a bindings file no key signed
// (spec.ErrBindingsUnsigned), an untrusted repository spec gets the spec
// trust line, right after the spec line, naming the file, and under it the
// trust verb by this binary's path (D189), whole: the brief's line cap grows
// by one, to ten, in this case only. A file that is not the bindings' shape
// reads unsigned too, since the verb replaces it (D173b). A signed file, no
// file, a file that cannot be read (a directory in its place), a managed spec
// alone, no spec, and no store root (the working directory's file is not the
// store's) get none.
func TestBR1k_AnUnsignedBindingsFileSaysSo(t *testing.T) {
	root := project(t, map[string]string{"go.mod": "module m\n"})
	unsigned := []byte(`{"bindings":[],"mac":"` + strings.Repeat("0", 64) + `"}` + "\n")
	storeWith := func(files map[string][]byte) string {
		dir := t.TempDir()
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	key := bytes.Repeat([]byte{7}, spec.TrustKeyBytes)
	signed, err := spec.EncodeBindings(&spec.Bindings{}, key)
	if err != nil {
		t.Fatal(err)
	}
	st := storeWith(map[string][]byte{spec.TrustFile: unsigned})
	b := Build(root, st, Pointer{}, specAt(root, spec.Untrusted, "go", "test", "./..."))
	i := slices.IndexFunc(b.Facts, func(f Fact) bool { return f.Kind == KindSpecTrust })
	if i < 1 || b.Facts[i-1].Kind != KindSpec {
		t.Fatalf("facts %+v, want a spec trust line after the spec line", b.Facts)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want, next := "bindings file unsigned; re-trust from a terminal:", exe+" spec trust"
	if f := b.Facts[i]; f.Text != want || f.Next != next || f.Source != filepath.Join(st, spec.TrustFile) {
		t.Errorf("spec trust %q, next %q, from %q; want %q, %q, from %s", f.Text, f.Next, f.Source, want, next, filepath.Join(st, spec.TrustFile))
	}
	lines := strings.Split(Render(b), "\n")
	if len(lines) != 10 || lines[5] != "  spec trust: "+want || lines[6] != "    "+next {
		t.Errorf("rendered %q, want ten lines, the sixth the spec trust line and the seventh its command", lines)
	}
	if b := Build(root, storeWith(map[string][]byte{spec.TrustFile: []byte("[]\n")}), Pointer{}, specAt(root, spec.Untrusted, "go")); !slices.ContainsFunc(b.Facts, func(f Fact) bool { return f.Kind == KindSpecTrust }) {
		t.Errorf("a file not the bindings' shape: facts %+v, want the spec trust line (D173b)", b.Facts)
	}
	unreadable := t.TempDir()
	if err := os.Mkdir(filepath.Join(unreadable, spec.TrustFile), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(storeWith(map[string][]byte{spec.TrustFile: unsigned}))
	for name, c := range map[string]struct {
		store string
		sp    *spec.Compiled
	}{
		"signed":        {storeWith(map[string][]byte{spec.TrustFile: signed, spec.TrustKeyFile: key}), specAt(root, spec.Untrusted, "go")},
		"no file":       {storeWith(nil), specAt(root, spec.Untrusted, "go")},
		"unreadable":    {unreadable, specAt(root, spec.Untrusted, "go")},
		"managed alone": {st, specAt(root, spec.Managed, "go")},
		"no spec":       {st, nil},
		"no store root": {"", specAt(root, spec.Untrusted, "go")},
	} {
		if b := Build(root, c.store, Pointer{}, c.sp); slices.ContainsFunc(b.Facts, func(f Fact) bool { return f.Kind == KindSpecTrust }) {
			t.Errorf("%s: facts %+v, want no spec trust line", name, b.Facts)
		}
	}
}

// TestBR1l_TheTrustCommandIsThisBinary (D189): the command the brief names
// is the path this process runs as, quoted for a shell when it holds a space
// or a quote, and rashomon when there is no path.
func TestBR1l_TheTrustCommandIsThisBinary(t *testing.T) {
	defer func(f func() (string, error)) { executable = f }(executable)
	for _, c := range []struct {
		exe  string
		err  error
		want string
	}{
		{"/opt/x/bin/rashomon", nil, "/opt/x/bin/rashomon"},
		{"/Users/a b/.claude/plugins/autosteer/bin/rashomon", nil, "'/Users/a b/.claude/plugins/autosteer/bin/rashomon'"},
		{"/tmp/it's/rashomon", nil, `'/tmp/it'\''s/rashomon'`},
		{"", os.ErrNotExist, "rashomon"},
	} {
		executable = func() (string, error) { return c.exe, c.err }
		if got := self(); got != c.want {
			t.Errorf("self() with %q = %q, want %q", c.exe, got, c.want)
		}
	}
	executable = func() (string, error) { return "/opt/x/bin/rashomon", nil }
	if got, want := Resumed("/p"), "rashomon brief: resumed /p; /opt/x/bin/rashomon brief for the full brief"; got != want {
		t.Errorf("Resumed = %q, want %q", got, want)
	}
}

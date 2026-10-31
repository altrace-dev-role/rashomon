package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noGit is p's project with its .git taken away: a plain directory, as a
// terminal started in a non-git project finds it.
func noGit(t *testing.T, p specProject) {
	t.Helper()
	if err := os.Remove(filepath.Join(p.dir, ".git")); err != nil {
		t.Fatal(err)
	}
}

// TestSCL1_BareTrustFindsTheCwdSpecWithNoGit: in a plain terminal, with no
// .git above and no CLAUDE_PROJECT_DIR, `spec trust` and `spec untrust` name
// the spec of the directory Claude Code would be started in (it sets
// CLAUDE_PROJECT_DIR to that directory).
func TestSCL1_BareTrustFindsTheCwdSpecWithNoGit(t *testing.T) {
	p := newSpecProject(t, specUnit)
	noGit(t, p)
	atSpecTerminal(t, &person{answer: typedPrefix(t, specUnit)}, nil)
	code, out, errOut := specRun(t, "", "trust")
	if code != 0 || !strings.HasPrefix(out, "trusted "+p.path) || errOut != "" {
		t.Fatalf("bare trust with no .git: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := p.bindings(); len(got) != 1 || got[0].SpecPath != p.path {
		t.Fatalf("bindings %+v; want one at %s", got, p.path)
	}
	if code, out, errOut := specRun(t, "", "untrust"); code != 0 || !strings.HasPrefix(out, "untrusted "+p.path) {
		t.Errorf("bare untrust with no .git: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := p.bindings(); len(got) != 0 {
		t.Errorf("bindings after untrust %+v; want none", got)
	}
}

// TestSCL1c_TheWalkIsUnchangedWhereItWasFound: the cwd fallback only fills
// a gap. From a subdirectory of a git project the walk still reaches the
// project's spec, and a CLAUDE_PROJECT_DIR above cwd still bounds it; a cwd
// with no spec and no root above finds none.
func TestSCL1c_TheWalkIsUnchangedWhereItWasFound(t *testing.T) {
	p := newSpecProject(t, specUnit)
	sub := filepath.Join(p.dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	if got, err := specPath(nil); err != nil || got != p.path {
		t.Errorf("from a subdirectory of a git project: %q, %v; want %s", got, err, p.path)
	}
	noGit(t, p)
	if _, err := specPath(nil); err == nil {
		t.Error("a subdirectory with no .git, no CLAUDE_PROJECT_DIR and no spec of its own found one above it")
	}
	t.Setenv("CLAUDE_PROJECT_DIR", p.dir)
	if got, err := specPath(nil); err != nil || got != p.path {
		t.Errorf("with CLAUDE_PROJECT_DIR above cwd: %q, %v; want %s", got, err, p.path)
	}
}

// TestSCL1b_TheUndoLineNamesThePath: the trust print's undo line carries
// the spec's path, shell-quoted, so it undoes this spec from any directory.
func TestSCL1b_TheUndoLineNamesThePath(t *testing.T) {
	p := newSpecProject(t, specUnit)
	atSpecTerminal(t, &person{answer: typedPrefix(t, specUnit)}, nil)
	code, out, errOut := specRun(t, "", "trust")
	if code != 0 || errOut != "" {
		t.Fatalf("trust: exit %d, stderr %q", code, errOut)
	}
	want := "\n  undo: rashomon spec untrust " + shellWord(p.path) + "\n"
	if !strings.HasSuffix(out, want) {
		t.Errorf("trust printed %q; want it to end %q", out, want)
	}
	for in, w := range map[string]string{
		"/a/b/spec.json": "/a/b/spec.json",
		"/a b/spec.json": "'/a b/spec.json'",
		"/it's/s.json":   `'/it'\''s/s.json'`,
		"":               "''",
	} {
		if got := shellWord(in); got != w {
			t.Errorf("shellWord(%q) = %q, want %q", in, got, w)
		}
	}
}

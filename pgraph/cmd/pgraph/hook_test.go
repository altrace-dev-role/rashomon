package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repo is module(files) as a repository: a .git directory at its root, which
// is how the hook finds the project from anywhere inside it.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := module(t, files)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// payload is a PostToolUse payload naming cwd, as Claude Code writes it on
// the hook's stdin; "" leaves cwd out.
func payload(cwd string) string {
	p := map[string]string{"hook_event_name": "PostToolUse", "tool_name": "Edit"}
	if cwd != "" {
		p["cwd"] = cwd
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// hookRun runs `pgraph sync -hook` with in on its stdin and
// CLAUDE_PROJECT_DIR unset unless the test set it.
func hookRun(t *testing.T, in string, extra ...string) (int, string, string) {
	t.Helper()
	old := hookInput
	hookInput = strings.NewReader(in)
	defer func() { hookInput = old }()
	var out, errb bytes.Buffer
	code := run(append([]string{"sync", "-hook"}, extra...), &out, &errb)
	return code, out.String(), errb.String()
}

// TestPW1b_TheHookWritesSilentlyAndAlwaysExitsZero (card PW1b-ON-230):
// Claude Code runs `pgraph sync -hook` at session start and after each
// reply, so whatever happens it exits 0 and prints nothing on stdout (a
// SessionStart hook's stdout goes to the model); a sync it cannot write is
// one line on stderr and leaves the last pair as it was; a project with no
// go.mod at its root, or no .git above the call, is left alone; and -hook
// takes no root, since the payload names it. Break: let the hook exit 1 on
// a type error, print its summary, or sync a project with no go.mod, and
// this fails.
func TestPW1b_TheHookWritesSilentlyAndAlwaysExitsZero(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	root := repo(t, smallModule)
	code, out, errs := hookRun(t, payload(filepath.Join(root, "a")))
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("hook: exit %d, stdout %q, stderr %q; want 0 and silence", code, out, errs)
	}
	logPath := filepath.Join(root, ".pgraph", "pgraph.log")
	first := read(t, logPath)
	if len(first) == 0 {
		t.Fatal("the hook wrote an empty log")
	}
	write(t, filepath.Join(root, "a", "broken.go"), "package a\n\nvar Y int = \"s\"\n")
	code, out, errs = hookRun(t, payload(root))
	if code != 0 || out != "" || !strings.HasPrefix(errs, "pgraph: ") || strings.Count(errs, "\n") != 1 || len(errs) > 81 {
		t.Errorf("a type error: exit %d, stdout %q, stderr %q; want 0, silence, one line", code, out, errs)
	}
	if !bytes.Equal(read(t, logPath), first) {
		t.Error("a refused sync rewrote the log")
	}
	for name, files := range map[string]map[string]string{
		"go.mod below the root only": {"sub/go.mod": "module example.com/s\n", "sub/s.go": "package s\n"},
		"no go.mod":                  {"a/a.go": "package a\n"},
	} {
		dir := repo(t, files)
		if code, out, errs := hookRun(t, payload(dir)); code != 0 || out != "" || errs != "" {
			t.Errorf("%s: exit %d, %q %q; want 0 and silence", name, code, out, errs)
		}
		if entries, _ := filepath.Glob(filepath.Join(dir, "*", ".pgraph")); len(entries) > 0 {
			t.Errorf("%s: wrote %v", name, entries)
		}
		if _, err := os.Stat(filepath.Join(dir, ".pgraph")); err == nil {
			t.Errorf("%s: .pgraph was written", name)
		}
	}
	bare := module(t, smallModule)
	if code, _, _ := hookRun(t, payload(bare)); code != 0 {
		t.Errorf("no .git: exit %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(bare, ".pgraph")); err == nil {
		t.Error("no .git: .pgraph was written")
	}
	var o, e bytes.Buffer
	if code := run([]string{"sync", "-hook", root}, &o, &e); code != 2 {
		t.Errorf("-hook with a root: exit %d, want 2 (the payload names the project)", code)
	}
}

// TestPW1b_TheHookSyncsTheProjectTheCallRanIn: the project is the nearest
// .git above the payload's cwd, the root rashomon's recorder keys the
// session's project by (baseline.KeyOf), so the graph is where the Stop
// reads it even after the agent moved to another repository; a payload with
// no cwd falls back to CLAUDE_PROJECT_DIR. Break: read only
// CLAUDE_PROJECT_DIR, or only the payload, and this fails.
func TestPW1b_TheHookSyncsTheProjectTheCallRanIn(t *testing.T) {
	a, b := repo(t, smallModule), repo(t, smallModule)
	t.Setenv("CLAUDE_PROJECT_DIR", a)
	if code, _, errs := hookRun(t, payload(filepath.Join(b, "b"))); code != 0 || errs != "" {
		t.Fatalf("hook: exit %d, %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(b, ".pgraph", "pgraph.log")); err != nil {
		t.Error("the repository the call ran in was not synced")
	}
	if _, err := os.Stat(filepath.Join(a, ".pgraph")); err == nil {
		t.Error("CLAUDE_PROJECT_DIR was synced although the payload named another repository")
	}
	if code, _, errs := hookRun(t, payload("")); code != 0 || errs != "" {
		t.Fatalf("hook with no cwd: exit %d, %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(a, ".pgraph", "pgraph.log")); err != nil {
		t.Error("a payload with no cwd did not fall back to CLAUDE_PROJECT_DIR")
	}
}

// TestPW1b_ASyncRemovesWhatACutRunLeft: Claude Code ends a background hook
// still running when the session exits (se1/pw1b-async-probe.txt), so a run
// can stop inside writeAtomic and leave its temp file in .pgraph, three
// megabytes for this repository's log. The next sync, which holds the root
// lock so no live writer has one open, removes every such file and nothing
// else. Break: drop the sweep, or widen it past writeAtomic's names, and
// this fails.
func TestPW1b_ASyncRemovesWhatACutRunLeft(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	root := repo(t, smallModule)
	if code, _, errs := hookRun(t, payload(root)); code != 0 || errs != "" {
		t.Fatalf("first hook: exit %d, stderr %q", code, errs)
	}
	dir := filepath.Join(root, ".pgraph")
	cut := []string{".pgraph.log.tmp-123", ".pgraph.digests.tmp-9", ".pgraph.walk.tmp-77"}
	kept := []string{"pgraph.log.tmp-1", ".pgraph.log.tmpx", "..gitignore.tmp-5", ".notes"}
	for _, n := range append(append([]string(nil), cut...), kept...) {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("cut"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, ".pgraph.log.tmp-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	kept = append(kept, ".pgraph.log.tmp-dir")
	if err := os.WriteFile(filepath.Join(root, "a", "c.go"), []byte("package a\n\nfunc C() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := hookRun(t, payload(root)); code != 0 || errs != "" {
		t.Fatalf("hook after a Go edit: exit %d, stderr %q", code, errs)
	}
	for _, n := range cut {
		if _, err := os.Lstat(filepath.Join(dir, n)); err == nil {
			t.Errorf("%s, a cut run's temp file, is still in .pgraph after a sync", n)
		}
	}
	for _, n := range kept {
		if _, err := os.Lstat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s was removed: only writeAtomic's temp files go (%v)", n, err)
		}
	}
}

// TestPW1b_AnUnchangedTreeIsNotLoadedAgain: the hook fires after every
// reply, most of which change no Go file, so a tree whose walk the walk
// file already holds is not loaded: a README or a test file edit costs a
// walk, a Go edit a load. A tree whose sync was refused is not loaded again
// until it changes, so code that does not compile costs one load, not one
// per reply. A pair
// moved aside since is not current. A person's sync writes the walk file
// too. Break: drop the fast path, count test files, or forget a refusal,
// and the load count moves where it must not.
func TestPW1b_AnUnchangedTreeIsNotLoadedAgain(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	root := repo(t, smallModule)
	step := func(what string, wantLoads int) {
		t.Helper()
		before := loads
		if code, out, _ := hookRun(t, payload(root)); code != 0 || out != "" {
			t.Fatalf("%s: exit %d, stdout %q", what, code, out)
		}
		if got := loads - before; got != wantLoads {
			t.Errorf("%s: %d loads, want %d", what, got, wantLoads)
		}
	}
	step("first hook", 1)
	step("unchanged tree", 0)
	write(t, filepath.Join(root, "README.md"), "notes\n")
	step("a README edit", 0)
	write(t, filepath.Join(root, "a", "a_test.go"), "package a\n\nfunc helper() {}\n")
	step("a test file edit", 0)
	write(t, filepath.Join(root, "a", "later.go"), "package a\n\nfunc Later() {}\n")
	step("a Go edit", 1)
	write(t, filepath.Join(root, "a", "broken.go"), "package a\n\nvar Y int = \"s\"\n")
	step("code that does not compile", 1)
	step("the same code, unchanged", 0)
	if err := os.Remove(filepath.Join(root, "a", "broken.go")); err != nil {
		t.Fatal(err)
	}
	step("the code fixed", 1)
	if err := os.Rename(filepath.Join(root, ".pgraph", "pgraph.log"), filepath.Join(root, "moved.log")); err != nil {
		t.Fatal(err)
	}
	step("the log moved aside", 1)
	write(t, filepath.Join(root, "b", "more.go"), "package b\n\nfunc More() {}\n")
	if code, _, errs := sync(root); code != 0 {
		t.Fatalf("a person's sync: exit %d: %s", code, errs)
	}
	step("after a person's sync of the same tree", 0)
}

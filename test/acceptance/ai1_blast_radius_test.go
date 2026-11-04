package acceptance

// ARCH-INLINE1: after Claude edits a Go file, the post tells the model which
// other packages the platform graph shows calling into the file's package,
// so it learns what the edit may break before it claims done. The facts are
// PROVEN calls from a log the walk file vouches for, and only callers whose
// files are as the sync read them; the count is a lower bound ("at least"),
// and nothing is said where nothing is proven. It is said once per caller
// set: a second edit of the package, or a re-sync that left its callers as
// they were, says nothing.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ai1Files is a module whose package a is called by b (a function) and c (a
// package variable's initialiser); d calls nothing in a. b holds a test
// file, c none.
var ai1Files = map[string]string{
	"go.mod":      "module example.com/m\n\ngo 1.25.0\n",
	"a/a.go":      "package a\n\nfunc F() int { return 1 }\n",
	"a/a_test.go": "package a\n",
	"b/b.go":      "package b\n\nimport \"example.com/m/a\"\n\nfunc G() int { return a.F() }\n",
	"b/b_test.go": "package b\n",
	"c/c.go":      "package c\n\nimport \"example.com/m/a\"\n\nvar V = a.F()\n",
	"d/d.go":      "package d\n\nfunc D() int { return 4 }\n",
}

// ai1Prefix is the start of every blast-radius note.
const ai1Prefix = "rashomon record: the platform graph names other packages with code calling into the package this call edited"

// ai1Note is the blast-radius note an edit of package a gives on ai1Files.
const ai1Note = ai1Prefix + " (at least 2): 'b', 'c'; test files are in 1 of them."

// ai1Edit edits dir's file, leaving its bytes as they are, under prompt p1,
// and returns the note the post gave the model, "" for none.
func (e *env) ai1Edit(dir, id, file string) string {
	e.t.Helper()
	return e.ai1EditIn(dir, id, file, "p1")
}

// ai1EditIn is ai1Edit under prompt, as a call in a later turn is.
func (e *env) ai1EditIn(dir, id, file, prompt string) string {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	input := pw1b2Edit(dir, file)
	pre := map[string]any{"cwd": dir, "prompt_id": prompt}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, "Edit", input, pre), envs...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	post := map[string]any{"cwd": dir, "prompt_id": prompt, "tool_response": map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}}
	res := e.post(stPayload(e.t, "PostToolUse", id, "Edit", input, post), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	stdout := res.stdout
	if stdout == "" {
		return ""
	}
	var out struct {
		Hook struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		e.t.Fatalf("post stdout %q is not JSON: %v", stdout, err)
	}
	return out.Hook.Context
}

// ai1Write writes body to dir's file, outside the session.
func ai1Write(t *testing.T, dir, file, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ai1Walk is the walk file pgraph's last sync wrote in dir.
func ai1Walk(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".pgraph", "pgraph.walk"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestAI1_AGoEditTellsTheCallersOnce: after a sync, an edit of a/a.go tells
// the model that b and c call into its package, b holding a test file, as
// a lower bound; the next edit of the package says nothing. The log is read
// once per package until the next sync (A2's read gate): c's file changed
// since leaves the set the sync gave standing, so a third edit says nothing
// either. Break: drop the read marker, and the third edit reads again and
// names b alone (the print gate alone keeps the second edit silent).
func TestAI1_AGoEditTellsTheCallersOnce(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, ai1Files)
	h156Sync(t, dir)
	if got := e.ai1Edit(dir, "toolu_e1", "a/a.go"); got != ai1Note {
		t.Fatalf("the first edit's note = %q\nwant %q", got, ai1Note)
	}
	if got := e.ai1Edit(dir, "toolu_e2", "a/a.go"); got != "" {
		t.Errorf("the second edit of package a gave %q, want nothing", got)
	}
	ai1Write(t, dir, "c/c.go", "package c\n\nvar V = 3\n")
	if got := e.ai1Edit(dir, "toolu_e3", "a/a.go"); got != "" {
		t.Errorf("an edit after c changed, with no sync, gave %q, want nothing: the log was read again", got)
	}
}

// TestAI1_AReSyncSaysOnlyANewCallerSet: a re-sync that leaves a's callers as
// they were moves the log's sha256 and says nothing on the next turn's edit
// (the print gate, A2: the Stop re-syncs every turn); one that adds a caller
// tells the new set. Each sync is followed by a new prompt, as the Stop's is,
// since the router already dedupes a key within one prompt. Break: key the
// note on the log's version alone, and the next turn repeats it.
func TestAI1_AReSyncSaysOnlyANewCallerSet(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, ai1Files)
	h156Sync(t, dir)
	if got := e.ai1Edit(dir, "toolu_e1", "a/a.go"); got != ai1Note {
		t.Fatalf("premise: the first edit's note = %q, want %q", got, ai1Note)
	}
	before := ai1Walk(t, dir)
	ai1Write(t, dir, "f/f.go", "package f\n\nfunc H() int { return 6 }\n")
	h156Sync(t, dir)
	if ai1Walk(t, dir) == before {
		t.Fatal("premise: the re-sync left the walk file as it was")
	}
	if got := e.ai1EditIn(dir, "toolu_e2", "a/a.go", "p2"); got != "" {
		t.Errorf("an edit after a re-sync with the same callers gave %q, want nothing", got)
	}
	ai1Write(t, dir, "g/g.go", "package g\n\nimport \"example.com/m/a\"\n\nfunc K() int { return a.F() + 1 }\n")
	h156Sync(t, dir)
	want := ai1Prefix + " (at least 3): 'b', 'c', 'g'; test files are in 1 of them."
	if got := e.ai1EditIn(dir, "toolu_e3", "a/a.go", "p3"); got != want {
		t.Errorf("an edit after a caller was added gave %q\nwant %q", got, want)
	}
}

// TestAI1_ACallerEditedSinceTheSyncIsNotNamed: c's file changed after the
// sync, so its call into a may be gone; the note names b alone, still "at
// least". Break: skip the callers' freshness, and c is named from facts its
// file no longer vouches for.
func TestAI1_ACallerEditedSinceTheSyncIsNotNamed(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, ai1Files)
	h156Sync(t, dir)
	ai1Write(t, dir, "c/c.go", "package c\n\nvar V = 3\n")
	want := ai1Prefix + " (at least 1): 'b'; test files are in 1 of them."
	if got := e.ai1Edit(dir, "toolu_e1", "a/a.go"); got != want {
		t.Errorf("the note = %q\nwant %q", got, want)
	}
}

// TestAI1_ANoteNamingNoPackageIsNotSaid: a caller at the module's root is
// counted, never listed (a path slot holds no "."), so where the root is the
// one fresh caller the note would name no package, and it is not said (the
// DE's condition (a)); that log's answer stands until the next sync, as for
// no caller. A set naming a package counts the root in "and 1 more". Break:
// give the note a form with no list, and the first edit speaks; return the
// read marker on silence, and the second edit reads again.
func TestAI1_ANoteNamingNoPackageIsNotSaid(t *testing.T) {
	e := newSteerEnv(t)
	files := map[string]string{
		"go.mod":  ai1Files["go.mod"],
		"a/a.go":  ai1Files["a/a.go"],
		"b/b.go":  ai1Files["b/b.go"],
		"main.go": "package main\n\nimport \"example.com/m/a\"\n\nfunc main() { _ = a.F() }\n",
	}
	dir := gi1Project(t, files)
	h156Sync(t, dir)
	ai1Write(t, dir, "b/b.go", "package b\n")
	if got := e.ai1Edit(dir, "toolu_e1", "a/a.go"); got != "" {
		t.Errorf("an edit whose one fresh caller is the root gave %q, want nothing", got)
	}
	ai1Write(t, dir, "b/b.go", files["b/b.go"])
	if got := e.ai1EditIn(dir, "toolu_e2", "a/a.go", "p2"); got != "" {
		t.Errorf("an edit with no sync since gave %q, want nothing: the log was read again", got)
	}
	ai1Write(t, dir, "f/f.go", "package f\n\nfunc H() int { return 6 }\n")
	h156Sync(t, dir)
	want := ai1Prefix + " (at least 2): 'b' and 1 more; test files are in 0 of them."
	if got := e.ai1EditIn(dir, "toolu_e3", "a/a.go", "p3"); got != want {
		t.Errorf("an edit after a sync gave %q\nwant %q", got, want)
	}
}

// TestAI1_SilenceWhereNothingIsProven: each case says nothing to the model,
// never "no dependents": no sync; an edit of a test file; an edit of a
// package nothing calls into; a walk file that vouches for another log (a
// read mid-sync); record scope v4, which keeps no directory in the clear.
// Break: drop any one guard, and its case speaks.
func TestAI1_SilenceWhereNothingIsProven(t *testing.T) {
	cases := map[string]struct {
		file  string
		setup func(t *testing.T, e *env, dir string)
	}{
		"no sync":   {"a/a.go", func(*testing.T, *env, string) {}},
		"test file": {"a/a_test.go", func(t *testing.T, _ *env, dir string) { h156Sync(t, dir) }},
		"no caller": {"d/d.go", func(t *testing.T, _ *env, dir string) { h156Sync(t, dir) }},
		"torn pair": {"a/a.go", func(t *testing.T, _ *env, dir string) {
			h156Sync(t, dir)
			w := ai1Walk(t, dir)
			i := strings.Index(w, "log\t")
			ai1Write(t, dir, ".pgraph/pgraph.walk", w[:i+4]+strings.Repeat("0", 64)+w[i+68:])
		}},
		"record scope v4": {"a/a.go", func(t *testing.T, e *env, dir string) {
			h156Sync(t, dir)
			if err := os.WriteFile(filepath.Join(e.home, "record_scope"), []byte("v4\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir := gi1Project(t, ai1Files)
			c.setup(t, e, dir)
			if got := e.ai1Edit(dir, "toolu_e1", c.file); strings.HasPrefix(got, ai1Prefix) {
				t.Errorf("an edit of %s gave %q, want no blast-radius note", c.file, got)
			}
		})
	}
}

// TestAI1_AYieldedNoteComesWithTheNextEdit: an edit that leaves a green run
// of the declared gate behind raises stale-evidence and blast-radius in one
// call; the correction prints (78 over 25) and the blast-radius read's
// marker is returned, so the package's next edit, with the stale trigger
// spent, gives the callers. Break: keep the marker on a yield, and the
// callers are never told this sync.
func TestAI1_AYieldedNoteComesWithTheNextEdit(t *testing.T) {
	e := newSteerEnv(t)
	dir, path := dghProject(t, dghSpec)
	e.trust(path)
	for name, body := range ai1Files {
		ai1Write(t, dir, name, body)
	}
	h156Sync(t, dir)
	e.dghCall(dir, "toolu_g", "Bash", dghRun)
	e.backdate("toolu_g", 120_000)
	got := e.ai1Edit(dir, "toolu_e1", "a/a.go")
	if !strings.Contains(got, "gate 'unit' exited 0 earlier in this session") {
		t.Fatalf("premise: the edit after the green run gave %q, want the stale-evidence note", got)
	}
	if got := e.ai1Edit(dir, "toolu_e2", "a/a.go"); got != ai1Note {
		t.Errorf("the next edit gave %q\nwant %q", got, ai1Note)
	}
}

package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// pr1Tree is a home with no .git above it, holding three repositories the way
// the team's sessions hold their worktrees (card PROJECT-RESOLVE1): a and b
// with a .git directory, c with a .git file (a linked worktree's gitdir
// pointer), notes, a directory under no .git (docs-out/plans), and dollar, a
// repository literally named $R, so a line that names $R names a directory
// that is on disk only as the variable's spelling. Every path is physical, so
// the keys the hook records compare as spelled, except spelled, the temp
// dir as t.TempDir names it (under /var on macOS, a symlink to /private/var).
type pr1Tree struct{ home, a, b, c, notes, dollar, spelled string }

func newPR1Tree(t *testing.T) pr1Tree {
	t.Helper()
	spelled := t.TempDir()
	base, err := filepath.EvalSymlinks(spelled)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	tr := pr1Tree{spelled: spelled, home: home, a: filepath.Join(home, "src", "a"), b: filepath.Join(home, "src", "b"),
		c: filepath.Join(home, "src", "c"), notes: filepath.Join(home, "notes"), dollar: filepath.Join(home, "$R")}
	for _, d := range []string{filepath.Join(tr.a, ".git"), filepath.Join(tr.b, ".git"), filepath.Join(tr.b, "internal"), tr.c, tr.notes,
		filepath.Join(tr.dollar, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tr.c, ".git"), []byte("gitdir: "+filepath.Join(tr.a, ".git", "worktrees", "c")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	return tr
}

// pr1Project is the project the hook records for one call from cwd: its
// PreToolUse through Handler and its post through Post, at v5.
func pr1Project(t *testing.T, cwd, tool string, input map[string]any) string {
	t.Helper()
	f := newST(t)
	f.cwd = cwd
	f.call("toolu_p", tool, input, 0)
	run, err := f.st.ReadRun(stSession)
	if err != nil || len(run.Declarations) != 1 {
		t.Fatalf("read run: %v, %d declarations", err, len(run.Declarations))
	}
	if p := run.Declarations[0].Project; p != nil {
		return *p
	}
	return ""
}

func pr1Edit(path string) map[string]any {
	return map[string]any{"file_path": path, "old_string": "a", "new_string": "b"}
}

// TestPR1_TheProjectIsTheRepositoryTheCallRanIn: a call that names its own
// directory -- a leading `cd DIR &&`, a `git -C DIR` at the head of the line,
// a file tool's path -- inside a repository records that repository as its
// project, not the repository (or the home) its cwd is in. Each line is cut
// from the replayed team sessions (se2/l1, MANIFEST-1101-0930), which ran
// 295 of 9,207 calls this way, 54 of them from the home.
func TestPR1_TheProjectIsTheRepositoryTheCallRanIn(t *testing.T) {
	tr := newPR1Tree(t)
	// Premise: a call with nothing of its own is keyed on its cwd, a repo
	// root and a directory under no .git alike.
	if got := pr1Project(t, tr.b, "Bash", goTest); got != tr.b {
		t.Fatalf("premise: go test from b recorded project %q, want %q", got, tr.b)
	}
	if got := pr1Project(t, tr.home, "Bash", goTest); got != tr.home {
		t.Fatalf("premise: go test from the home recorded project %q, want %q", got, tr.home)
	}
	for _, c := range []struct {
		name, cwd, tool string
		input           map[string]any
		want            string
	}{
		{"eng-4: cd into another worktree", tr.a, "Bash", map[string]any{"command": "cd " + tr.b + " && git status --porcelain"}, tr.b},
		{"qa-1: cd ~/... from the home into a linked worktree", tr.home, "Bash", map[string]any{"command": "cd ~/src/c && go test ./internal/..."}, tr.c},
		{"de: a relative cd from a directory under no .git", filepath.Join(tr.home, "src"), "Bash", map[string]any{"command": "cd b && go vet ./..."}, tr.b},
		{"eng-3: Edit in a repository from the home", tr.home, "Edit", pr1Edit(filepath.Join(tr.b, "internal", "x.go")), tr.b},
		{"Write in another repository", tr.a, "Write", map[string]any{"file_path": filepath.Join(tr.b, "new.go"), "content": "package b\n"}, tr.b},
		{"Write into a new directory through the temp dir's spelling", tr.home, "Write",
			map[string]any{"file_path": filepath.Join(tr.spelled, "home", "src", "b", "newdir", "new.go"), "content": "package b\n"}, tr.b},
		{"qa-1: git -C from the home", tr.home, "Bash", map[string]any{"command": "git -C " + tr.b + " log --oneline -1"}, tr.b},
	} {
		if got := pr1Project(t, c.cwd, c.tool, c.input); got != c.want {
			t.Errorf("%s: project %q, want %q, the repository the call ran in", c.name, got, c.want)
		}
	}
}

// TestPR1b_AnUnprovenDirectoryKeepsTheCwdsProject: where the call's own
// directory is not known for certain, or is under no .git inside the cwd,
// the project stays the cwd's, as before the card: nothing new is recorded
// on a guess. Every cwd here is its own project key.
func TestPR1b_AnUnprovenDirectoryKeepsTheCwdsProject(t *testing.T) {
	tr := newPR1Tree(t)
	for _, c := range []struct {
		name, cwd, tool string
		input           map[string]any
	}{
		{"a cd into a variable", tr.a, "Bash", map[string]any{"command": "cd $W && go test ./..."}},
		{"a cd into a variable spelled like a repository", tr.home, "Bash", map[string]any{"command": "cd $R && go test ./..."}},
		{"a quoted cd target", tr.a, "Bash", map[string]any{"command": "cd '" + tr.b + "' && go test ./..."}},
		{"two roots in one line", tr.a, "Bash", map[string]any{"command": "cd " + tr.b + " && go test ./... && cd " + tr.c + " && go test ./..."}},
		{"a cd past the leading run", tr.a, "Bash", map[string]any{"command": "go vet ./... && cd " + tr.b + " && go test ./..."}},
		{"a cd into a directory under no .git inside the cwd", tr.home, "Bash", map[string]any{"command": "cd notes && ls"}},
		{"a cd into a directory not on disk", tr.a, "Bash", map[string]any{"command": "cd " + filepath.Join(tr.home, "gone") + " && ls"}},
		{"a file under no .git inside the cwd", tr.home, "Edit", pr1Edit(filepath.Join(tr.notes, "x.txt"))},
		{"git -C past the head", tr.a, "Bash", map[string]any{"command": "echo x && git -C " + tr.b + " status"}},
		{"git -C into a variable spelled like a repository", tr.home, "Bash", map[string]any{"command": "git -C $R status"}},
		{"git -C twice", tr.home, "Bash", map[string]any{"command": "git -C " + tr.b + " -C " + tr.c + " status"}},
		{"a line cut at the token bound", tr.a, "Bash", map[string]any{"command": "cd " + tr.b + " && echo" + strings.Repeat(" w", 70_000)}},
		{"a cd inside the same repository", tr.b, "Bash", map[string]any{"command": "cd internal && go test ./..."}},
	} {
		if got := pr1Project(t, c.cwd, c.tool, c.input); got != c.cwd {
			t.Errorf("%s: project %q, want the cwd's %q", c.name, got, c.cwd)
		}
	}
}

// TestPR1d_ARunProvenOutsideTheProjectBelongsToNone (manager 10:5x and
// 11:06, B305, QA-3's def3533d seq 350): from its worktree, QA-3 ran a test
// in a throwaway module under the temp dir, and `rashomon report` counted it
// as the worktree's test run. A call whose own directory is proven -- a
// leading cd or git -C, read whole, or a file tool's absolute path, read at
// its nearest directory on disk -- outside the cwd and under no .git belongs
// to no project: it is recorded with no project key, never as the cwd's.
// One write, one project, whichever tool made it. Controls: the same file
// under the cwd keeps the cwd's project.
func TestPR1d_ARunProvenOutsideTheProjectBelongsToNone(t *testing.T) {
	tr := newPR1Tree(t)
	fixture, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The temp dir as the test gets it, unresolved (/var/folders on macOS,
	// under /private): new/ is not on disk.
	notes := filepath.Join(t.TempDir(), "new", "dir", "notes.txt")
	write := func(path string) map[string]any { return map[string]any{"file_path": path, "content": "x\n"} }
	for _, c := range []struct {
		name, cwd, tool string
		input           map[string]any
		want            string
	}{
		{"qa-3: go test in a module under the temp dir", tr.a, "Bash",
			map[string]any{"command": "cd " + fixture + " && go test ./... -count=1 2>test.stderr.txt 1>test.stdout.txt; echo \"exit=$?\""}, ""},
		{"a cd into a directory under no .git beside the repository", tr.a, "Bash", map[string]any{"command": "cd " + tr.notes + " && ls"}, ""},
		{"git -C a directory under no .git", tr.a, "Bash", map[string]any{"command": "git -C " + tr.notes + " status"}, ""},
		{"a Write into a directory not yet on disk under the temp dir", tr.a, "Write", write(notes), ""},
		{"an Edit of a file under no .git beside the repository", tr.a, "Edit", pr1Edit(filepath.Join(tr.notes, "x.txt")), ""},
		{"control: the same Write under the cwd", tr.home, "Write", write(filepath.Join(tr.home, "new", "dir", "notes.txt")), tr.home},
	} {
		if got := pr1Project(t, c.cwd, c.tool, c.input); got != c.want {
			t.Errorf("%s: project %q, want %q: the directory is proven outside %q and under no .git, or inside it", c.name, got, c.want, c.cwd)
		}
	}
}

// TestPR1c_ACdIntoAnotherRepositoryBindsItsGate: a turn whose test run is
// `cd B && go test ./...` from repository A binds B's declared gate at the
// Stop. The run's location already folds the cd (startDirectory), and the
// gate's is the project root joined with its dir (declared.go), so with the
// project left at A the two never meet and B's gate reads not run.
func TestPR1c_ACdIntoAnotherRepositoryBindsItsGate(t *testing.T) {
	tr := newPR1Tree(t)
	yes := true
	sp := &spec.Compiled{
		Sources: []spec.Source{{Path: filepath.Join(tr.b, ".claude/rashomon/spec.json"), Digest: "sha256:0", Trust: spec.Trusted}},
		Trust:   spec.Trusted,
		Gates: []spec.Declared{{ID: "spec:unit", Gate: spec.Gate{ID: "unit", Kind: "test",
			Argv: []string{"go", "test", "./..."}, Match: "prefix", Required: &yes}}},
	}
	state := func(cwd, cmd string) (verify.State, verify.Reason) {
		f := newST(t)
		f.cwd = cwd
		f.call("toolu_e", "Edit", pr1Edit(filepath.Join(tr.b, "internal", "x.go")), 0)
		f.call("toolu_g", "Bash", map[string]any{"command": cmd}, 0)
		run, err := f.st.ReadRun(stSession)
		if err != nil {
			t.Fatal(err)
		}
		snap := verify.EvaluateStop(run, nil, sp,
			verify.Point{Kind: verify.PointStop, Seq: 1 << 40, SessionID: stSession}, time.UnixMilli(1_800_000_600_000),
			verify.StopInput{Key: f.st.Key()}).Snap
		for _, g := range snap.Gates {
			if g.Gate == "spec:unit" {
				return g.State, g.Reason
			}
		}
		t.Fatalf("no spec:unit row in %+v", snap.Gates)
		return "", ""
	}
	// Premise: the run binds the gate when it ran from B's own root.
	if s, why := state(tr.b, "go test ./..."); s != verify.Green {
		t.Fatalf("premise: go test from b reads %s (%s), want green", s, why)
	}
	if s, why := state(tr.a, "cd "+tr.b+" && go test ./..."); s != verify.Green {
		t.Errorf("cd b && go test from a reads %s (%s), want green: the run in b did not bind b's gate", s, why)
	}
}

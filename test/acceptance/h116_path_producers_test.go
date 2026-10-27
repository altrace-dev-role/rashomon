package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The path producers: every declaration names where the call ran and what
// file it edits. cwd is the payload's cwd as written; project is its project
// key, the repository root the novelty baseline keys on (symlinks resolved,
// then the nearest .git); file_path is the path an Edit, Write, MultiEdit or
// NotebookEdit input names, as written. None of the three is normalised by
// the hook: the record carries what the harness said, and resolving is the
// reader's. Each is bounded by the record's path cap, over which it is left
// out and its *_withheld key says oversize.
//
// Each test drives the hook binary with a real payload shape and reads the
// declaration back from the store.

// declaredPaths runs one PreToolUse call and returns its declaration.
func declaredPaths(t *testing.T, p payloadOpts) record {
	t.Helper()
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(p.build(t))
	decls := e.declarations(testSession)
	if len(decls) != 1 {
		t.Fatalf("read %d declarations, want 1", len(decls))
	}
	return decls[0]
}

// pathValue is a declaration's string field: its value and true, or false
// when the field is null. A key missing from the record fails the test,
// because a missing key is not a null.
func pathValue(t *testing.T, d record, key string) (string, bool) {
	t.Helper()
	v, present := d.fields[key]
	if !present {
		t.Fatalf("declaration has no %q key: %s", key, d.raw)
	}
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("declaration %q = %v (%T), want a string or null", key, v, v)
	}
	return s, true
}

func wantPath(t *testing.T, d record, key, want string) {
	t.Helper()
	got, ok := pathValue(t, d, key)
	if !ok || got != want {
		t.Errorf("%s = %q (present %v), want %q", key, got, ok, want)
	}
	if w, ok := pathValue(t, d, key+"_withheld"); ok {
		t.Errorf("%s_withheld = %q beside a recorded %s, want null", key, w, key)
	}
}

func wantNoPath(t *testing.T, d record, key, withheld string) {
	t.Helper()
	if got, ok := pathValue(t, d, key); ok {
		t.Errorf("%s = %q, want null", key, got)
	}
	w, ok := pathValue(t, d, key+"_withheld")
	switch {
	case withheld == "" && ok:
		t.Errorf("%s_withheld = %q, want null: nothing withheld it", key, w)
	case withheld != "" && w != withheld:
		t.Errorf("%s_withheld = %q (present %v), want %q", key, w, ok, withheld)
	}
}

// resolvedTempDir is a temporary directory with its own symlinks resolved
// (macOS keeps them under /var, itself a link to /private/var), so a test
// can say exactly what the resolved form of a path below it is.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// gitRepo makes root/repo with a .git directory and returns the repository
// root and a directory two levels inside it.
func gitRepo(t *testing.T, root string) (repo, inner string) {
	t.Helper()
	repo = filepath.Join(root, "repo")
	inner = filepath.Join(repo, "cmd", "tool")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return repo, inner
}

func TestPP1_DeclarationCarriesCWDAndProject(t *testing.T) {
	t.Run("cwd inside a repository", func(t *testing.T) {
		repo, inner := gitRepo(t, resolvedTempDir(t))
		p := defaultPayload()
		p.CWD = inner
		d := declaredPaths(t, p)
		wantPath(t, d, "cwd", inner)
		wantPath(t, d, "project", repo)
	})
	// Through a symlink, the project is the resolved repository root and the
	// cwd stays as the harness wrote it.
	t.Run("cwd through a symlink", func(t *testing.T) {
		root := resolvedTempDir(t)
		repo, inner := gitRepo(t, root)
		link := filepath.Join(root, "link")
		if err := os.Symlink(filepath.Join(repo, "cmd"), link); err != nil {
			t.Fatal(err)
		}
		cwd := filepath.Join(link, "tool")
		p := defaultPayload()
		p.CWD = cwd
		d := declaredPaths(t, p)
		wantPath(t, d, "cwd", cwd)
		wantPath(t, d, "project", repo)
		if cwd == inner {
			t.Fatal("premise: the linked cwd is the resolved one")
		}
	})
}

func TestPP2_EditAndWriteCarryFilePath(t *testing.T) {
	for _, tool := range []string{"Edit", "Write"} {
		t.Run(tool, func(t *testing.T) {
			const path = "/home/u/proj/internal/x.go"
			p := defaultPayload()
			p.ToolName = tool
			p.ToolInput = map[string]any{"file_path": path, "old_string": "a", "new_string": "b", "content": "c"}
			wantPath(t, declaredPaths(t, p), "file_path", path)
		})
	}
	// A relative path is stored as sent: resolving it against cwd is the
	// reader's, and so is cleaning it.
	t.Run("relative, as written", func(t *testing.T) {
		const path = "internal/../internal/./x.go"
		p := defaultPayload()
		p.ToolName = "Write"
		p.ToolInput = map[string]any{"file_path": path, "content": "c"}
		wantPath(t, declaredPaths(t, p), "file_path", path)
	})
}

func TestPP3_NotebookEditCarriesNotebookPath(t *testing.T) {
	const path = "/home/u/proj/analysis.ipynb"
	p := defaultPayload()
	p.ToolName = "NotebookEdit"
	p.ToolInput = map[string]any{"notebook_path": path, "new_source": "print(1)", "cell_id": "c1"}
	wantPath(t, declaredPaths(t, p), "file_path", path)
}

func TestPP4_MultiEditCarriesFilePath(t *testing.T) {
	const path = "/home/u/proj/internal/y.go"
	p := defaultPayload()
	p.ToolName = "MultiEdit"
	p.ToolInput = map[string]any{"file_path": path, "edits": []any{
		map[string]any{"old_string": "a", "new_string": "b"},
		map[string]any{"old_string": "c", "new_string": "d"},
	}}
	wantPath(t, declaredPaths(t, p), "file_path", path)
}

func TestPP5_BashCarriesNoFilePath(t *testing.T) {
	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "cat /home/u/proj/x.go > /home/u/proj/y.go", "file_path": "/home/u/proj/z.go"}
	wantNoPath(t, declaredPaths(t, p), "file_path", "")
}

// Read is in the label contract's table and not in the declaration's: its
// path earns a label and is not recorded.
func TestPP5b_ReadCarriesNoFilePathButKeepsItsLabel(t *testing.T) {
	p := defaultPayload()
	p.ToolName = "Read"
	p.ToolInput = map[string]any{"file_path": "/home/u/.ssh/id_rsa"}
	d := declaredPaths(t, p)
	wantNoPath(t, d, "file_path", "")
	if got := d.str("file_label"); got != "ssh-key" {
		t.Errorf("file_label = %q, want %q", got, "ssh-key")
	}
}

// Over the cap, a path is left out whole and its own *_withheld says so; a
// path at the cap is kept. The project row comes through the cwd: a project
// key longer than the cap can only be the unresolved cwd itself, because the
// resolved form and every ancestor are paths the system could stat, which
// PATH_MAX bounds below the cap.
func TestPP6_OversizePathsAreWithheld(t *testing.T) {
	const maxPathBytes = 4096
	over := "/" + strings.Repeat("o", maxPathBytes)
	atCap := "/" + strings.Repeat("k", maxPathBytes-1)

	t.Run("file_path", func(t *testing.T) {
		p := defaultPayload()
		p.ToolName = "Edit"
		p.ToolInput = map[string]any{"file_path": over, "old_string": "a", "new_string": "b"}
		d := declaredPaths(t, p)
		wantNoPath(t, d, "file_path", "oversize")
		wantPath(t, d, "cwd", p.CWD)
	})
	t.Run("file_path at the cap", func(t *testing.T) {
		p := defaultPayload()
		p.ToolName = "Edit"
		p.ToolInput = map[string]any{"file_path": atCap, "old_string": "a", "new_string": "b"}
		wantPath(t, declaredPaths(t, p), "file_path", atCap)
	})
	t.Run("cwd", func(t *testing.T) {
		p := defaultPayload()
		p.CWD = over
		p.ToolName = "Write"
		p.ToolInput = map[string]any{"file_path": "/home/u/proj/x.go", "content": "c"}
		d := declaredPaths(t, p)
		wantNoPath(t, d, "cwd", "oversize")
		wantPath(t, d, "file_path", "/home/u/proj/x.go")
	})
	t.Run("project", func(t *testing.T) {
		p := defaultPayload()
		p.CWD = over
		d := declaredPaths(t, p)
		wantNoPath(t, d, "project", "oversize")
	})
	t.Run("cwd and project at the cap", func(t *testing.T) {
		p := defaultPayload()
		p.CWD = atCap
		d := declaredPaths(t, p)
		wantPath(t, d, "cwd", atCap)
		wantPath(t, d, "project", atCap)
	})
}

func TestPP7_NoCWDLeavesBothNil(t *testing.T) {
	p := defaultPayload()
	p.CWD = ""
	d := declaredPaths(t, p)
	wantNoPath(t, d, "cwd", "")
	wantNoPath(t, d, "project", "")
}

// A cwd removed before the hook runs cannot be resolved, so the project is
// the walk over the path as written: through a link, the linked form of the
// repository root, not its resolved one.
func TestPP8_DeletedCWDFallsBackUnresolved(t *testing.T) {
	root := resolvedTempDir(t)
	repo, inner := gitRepo(t, root)
	link := filepath.Join(root, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(inner); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(link, "cmd", "tool")
	p := defaultPayload()
	p.CWD = cwd
	d := declaredPaths(t, p)
	wantPath(t, d, "cwd", cwd)
	wantPath(t, d, "project", link)
}

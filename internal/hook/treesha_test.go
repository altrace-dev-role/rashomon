package hook

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// hexSHA is data's sha256 as the post records it.
func hexSHA(data []byte) *string {
	sum := sha256.Sum256(data)
	s := hex.EncodeToString(sum[:])
	return &s
}

// putFile writes data at dir/name and returns its path.
func putFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func sameSHA(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// shown is a sha for a message: the hex, or nil.
func shown(s *string) string {
	if s == nil {
		return "nil"
	}
	return *s
}

// TestPG6c_PostRecordsTheWrittenFilesSHA: a named write's post records the
// sha256 of the file its input names, as the call left it (card P5A, D140):
// a file of exactly MaxHashBytes is hashed and one byte more is not; a
// relative path joins the payload's cwd; a NotebookEdit names notebook_path.
// A Bash call, a file gone and a session below record_scope v5 record none.
func TestPG6c_PostRecordsTheWrittenFilesSHA(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	exact := bytes.Repeat([]byte("x"), MaxHashBytes)
	over := bytes.Repeat([]byte("y"), MaxHashBytes+1)
	small := []byte("package a\n")
	exactPath := putFile(t, dir, "exact.go", exact)
	overPath := putFile(t, dir, "over.go", over)
	putFile(t, dir, "rel.go", small)
	nbPath := putFile(t, dir, "a.ipynb", small)
	for _, tc := range []struct {
		name  string
		tool  string
		input map[string]any
		v5    bool
		want  *string
	}{
		{"an Edit of 1 MiB", "Edit", map[string]any{"file_path": exactPath}, true, hexSHA(exact)},
		{"a Write over 1 MiB", "Write", map[string]any{"file_path": overPath}, true, nil},
		{"a relative path", "Edit", map[string]any{"file_path": "rel.go"}, true, hexSHA(small)},
		{"a NotebookEdit", "NotebookEdit", map[string]any{"notebook_path": nbPath}, true, hexSHA(small)},
		{"a Bash call", "Bash", map[string]any{"command": "sed -i s/a/b/ " + exactPath}, true, nil},
		{"a file gone", "Edit", map[string]any{"file_path": filepath.Join(dir, "gone.go")}, true, nil},
		{"below record_scope v5", "Edit", map[string]any{"file_path": exactPath}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := mustOpen(t)
			if tc.v5 {
				if err := store.WriteRecordScopeMarker(st.Root(), store.RecordScopeV5); err != nil {
					t.Fatal(err)
				}
			}
			payload, err := json.Marshal(map[string]any{
				"session_id": "s", "tool_use_id": "toolu_w", "tool_name": tc.tool, "hook_event_name": "PostToolUse",
				"cwd": dir, "tool_input": tc.input,
			})
			if err != nil {
				t.Fatal(err)
			}
			p := NewPost(st, time.Now)
			p.Close(&Signals{}, p.Capture(strings.NewReader(string(payload))))
			run, err := st.ReadRun("s")
			if err != nil || len(run.Executions) != 1 {
				t.Fatalf("read %v, %v; want one execution", run, err)
			}
			if got := run.Executions[0].ContentSHA256; !sameSHA(got, tc.want) {
				t.Errorf("content_sha256 %s, want %s", shown(got), shown(tc.want))
			}
		})
	}
}

// TestPG6d_TreeShasReadsEachTarget: the Stop's side of the binding (card P5A,
// D140) is each target's content now by its path: a file's sha256, each its
// own though one hasher reads them in turn; nil for a file gone, one over
// MaxHashBytes and a directory, content no run was bound to; no entry for a
// file that could not be read. No targets is no tree.
func TestPG6d_TreeShasReadsEachTarget(t *testing.T) {
	dir := t.TempDir()
	small := []byte("package a\n")
	other := []byte("package b\n")
	locked := putFile(t, dir, "locked.go", small)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := TreeShas([]verify.TreeTarget{
		{Path: "a.go", File: putFile(t, dir, "a.go", small)},
		{Path: "b.go", File: putFile(t, dir, "b.go", other)},
		{Path: "big.go", File: putFile(t, dir, "big.go", bytes.Repeat([]byte("y"), MaxHashBytes+1))},
		{Path: "gone.go", File: filepath.Join(dir, "gone.go")},
		{Path: "sub", File: sub},
		{Path: "locked.go", File: locked},
	})
	for path, want := range map[string]*string{"a.go": hexSHA(small), "b.go": hexSHA(other), "big.go": nil, "gone.go": nil, "sub": nil} {
		if sha, ok := got[path]; !ok || !sameSHA(sha, want) {
			t.Errorf("%s: %s, entry %t; want %s", path, shown(sha), ok, shown(want))
		}
	}
	if _, err := os.ReadFile(locked); err == nil {
		t.Log("locked.go is readable here (root, or no permission bits); its row is not checked")
	} else if sha, ok := got["locked.go"]; ok {
		t.Errorf("locked.go: %s; want no entry for a file that could not be read", shown(sha))
	}
	if TreeShas(nil) != nil {
		t.Error("TreeShas(nil) is a tree; want nil, the cap's unknown")
	}
	if tree := TreeShas([]verify.TreeTarget{}); tree == nil || len(tree) != 0 {
		t.Errorf("TreeShas of no target = %#v, want an empty tree", tree)
	}
}

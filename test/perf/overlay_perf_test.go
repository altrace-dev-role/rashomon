//go:build perf

package perf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// overlayCase is the PreToolUse overlay writer's case (W-M2.5c, 5c-2): the
// real hook entry on an Edit of a Go file under an index, in a store whose
// session-content marker is present, so the writer reads the file, splices
// the edit, resolves the content and appends a declared group. The payload is
// 40000-byte Edit of the project's file, hook.40KB's size.
const overlayCase = "hook.edit.overlay"

// overlayOld is the line the payload's Edit replaces (payload.EditPre's).
const overlayOld = "// handle1 processes one batch of records\n"

// overlayHook returns the payload builder and the control of the overlay
// case: a project with a git head, a synthetic index and a Go file of about
// 40 KiB holding overlayOld once; the store's marker is present. A premise
// run checks the run directory holds the overlay's byte counter, so a case
// whose writer stood down fails rather than reads light. control takes the
// marker away: the same Edit with no overlay.
func (r rig) overlayHook(t *testing.T, size int) (mk func(i int) []byte, control func()) {
	t.Helper()
	proj, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := graph.SaveFile(proj, graphtest.Synthetic(200), graphPromptHeader); err != nil {
		t.Fatal(err)
	}
	var src strings.Builder
	src.WriteString("package batch\n\n" + overlayOld + "func handle1() {}\n\n")
	for i := 0; src.Len() < 40<<10; i++ {
		fmt.Fprintf(&src, "// Func%04d is one more function of the file.\nfunc Func%04d(batch []int) int { return len(batch) + %d }\n\n", i, i, i)
	}
	file := filepath.Join(proj, "internal", "batch", "handle.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	// The Edit replaces overlayOld with a comment line padded so the whole
	// payload is exactly size bytes: a comment keeps the result Go the
	// resolver parses, which payload.EditPre's pad (statements outside a
	// function) does not.
	transcript := filepath.Join(proj, "transcript.jsonl")
	build := func(i, pad int) []byte {
		b, err := json.Marshal(map[string]any{
			"session_id": payload.Session, "transcript_path": transcript, "cwd": proj, "permission_mode": "default",
			"hook_event_name": "PreToolUse", "tool_name": "Edit", "tool_use_id": fmt.Sprintf("toolu_ov%02d", i),
			"tool_input": map[string]any{"file_path": file, "old_string": overlayOld,
				"new_string": "// " + strings.Repeat("x", pad) + "\n", "replace_all": false},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	fixed := len(build(0, 0))
	if fixed > size {
		t.Fatalf("the payload's fixed part, %d bytes, is over %d", fixed, size)
	}
	mk = func(i int) []byte {
		p := build(i, size-fixed)
		if len(p) != size {
			t.Fatalf("payload is %d bytes, want %d", len(p), size)
		}
		return p
	}
	if err := os.WriteFile(filepath.Join(r.home, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(rashomonBin, r.args([]string{"hook"})...)
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Env, cmd.Stderr = bytes.NewReader(mk(99)), r.env, &stderr
	if out, err := cmd.Output(); err != nil {
		t.Fatalf("rashomon hook: %v\n%s\n%s", err, out, stderr.Bytes())
	}
	st, err := store.Open(r.home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.RunDir(payload.Session), store.FileOverlayBytes)); err != nil {
		t.Fatalf("premise: the hook reserved no overlay bytes, so the writer read nothing: %v\n%s", err, stderr.Bytes())
	}
	return mk, func() {
		if err := os.Rename(filepath.Join(r.home, store.FileSessionContent), filepath.Join(r.home, store.FileSessionContent+".off")); err != nil {
			t.Fatal(err)
		}
	}
}

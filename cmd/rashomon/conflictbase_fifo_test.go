//go:build unix

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// cbFifoAt replaces p with a named pipe no one writes, and wakes any reader
// left on it when the test ends (internal/platform's fifoAt).
func cbFifoAt(t *testing.T, p string) {
	t.Helper()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(p, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
}

// cbEntry runs one hook entry on its own goroutine and gives its exit code
// and stderr, failing the test, named, if the entry has not returned after
// 5 s: a read held by a pipe is a FAIL at the deadline, never the package's
// timeout. The payload is built by the caller on the test's goroutine.
func cbEntry(t *testing.T, what string, run func(out, errOut *bytes.Buffer) int) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	done := make(chan struct{})
	go func() { defer close(done); code = run(&out, &errOut) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s had not returned after 5s", what)
	}
	return code, out.String(), errOut.String()
}

// TestConflictBase_ANamedPipeIsAMissNotAWait: a named pipe at the
// session's conflict base path is a miss for both of its readers, never a
// wait for a writer that does not come, through the real entries that read
// it. The prompt reads the base's head to keep a current base
// (readConflictBaseHead), misses, and writes a new base over the pipe,
// which then reads back. The post of a Go edit that crosses b1 reads the
// base (readConflictBase), misses, and records no conflict check and says
// nothing. Each entry returns inside the 5 s deadline. The controls, the
// same tree with the base a regular file: the prompt's base reads back,
// and the same post records the check and gives the note. Break: open
// either reader's path with os.Open, and its row fails at the deadline.
func TestConflictBase_ANamedPipeIsAMissNotAWait(t *testing.T) {
	t.Run("prompt", func(t *testing.T) {
		s := cfSession(t, store.RecordScopeV5, true)
		s.prompt("fix " + cfGraph)
		if _, ok := readConflictBase(s.basePath()); !ok {
			t.Fatal("control: the prompt wrote no base that reads back")
		}
		cbFifoAt(t, s.basePath())
		head := newBenchHead("UserPromptSubmit")
		head.CWD = s.project
		body := benchJSON(t, struct {
			benchHead
			Prompt   string `json:"prompt"`
			PromptID string `json:"prompt_id"`
		}{head, "fix " + cfGraph, "prompt-2"})
		code, _, stderr := cbEntry(t, "the prompt over a pipe at the base", func(out, errOut *bytes.Buffer) int {
			return cmdRecap([]string{"pending", install.Marker, s.id}, bytes.NewReader(body), out, errOut)
		})
		if code != exitOK || stderr != "" {
			t.Errorf("the prompt exited %d, stderr %q", code, stderr)
		}
		if fi, err := os.Lstat(s.basePath()); err != nil || !fi.Mode().IsRegular() {
			t.Errorf("after the prompt the base path is %v, %v; want a regular file written over the pipe", fi, err)
		}
		if _, ok := readConflictBase(s.basePath()); !ok {
			t.Error("the base the prompt wrote over the pipe does not read back")
		}
	})
	t.Run("post", func(t *testing.T) {
		ctl := cfSession(t, store.RecordScopeV5, true)
		ctl.prompt("fix " + cfGraph)
		if out := ctl.land("toolu_cf1", cfGraph, cfCross); !strings.Contains(out, cfNote) || len(ctl.run().ConflictChecks) != 1 {
			t.Fatalf("control, the base a regular file: post output %q, %d checks; want the note and one check", out, len(ctl.run().ConflictChecks))
		}
		s := cfSession(t, store.RecordScopeV5, true)
		s.prompt("fix " + cfGraph)
		cbFifoAt(t, s.basePath())
		path := filepath.Join(s.project, filepath.FromSlash(cfGraph))
		if err := os.WriteFile(path, []byte(cfCross), 0o644); err != nil {
			t.Fatal(err)
		}
		in, err := json.Marshal(map[string]any{"file_path": path, "content": cfCross})
		if err != nil {
			t.Fatal(err)
		}
		head := newBenchHead("PostToolUse")
		head.CWD = s.project
		body := benchJSON(t, benchToolPayload{benchHead: head, PromptID: "prompt-1", ToolName: "Write", ToolInput: in, ToolUseID: "toolu_cf1",
			ToolResponse: json.RawMessage(`{"stdout":"","stderr":"","interrupted":false,"isImage":false}`)})
		code, out, stderr := cbEntry(t, "the post over a pipe at the base", func(o, e *bytes.Buffer) int {
			return cmdPost([]string{install.Marker, s.id}, bytes.NewReader(body), o, e)
		})
		if code != exitOK || stderr != "" {
			t.Errorf("the post exited %d, stderr %q", code, stderr)
		}
		if strings.Contains(out, cfNote) {
			t.Errorf("a pipe at the base: the post gave the note %q; want a miss", out)
		}
		if cs := s.run().ConflictChecks; len(cs) != 0 {
			t.Errorf("a pipe at the base: %d conflict checks recorded; want none (a miss checks nothing)", len(cs))
		}
	})
}

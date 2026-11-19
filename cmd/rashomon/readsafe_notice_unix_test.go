//go:build unix

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The manifest notice rows that need a named pipe or a link loop in place
// of the project's manifest (READ-SAFE1a D7). Each entry runs under
// cbEntry's 5 s deadline, so a read the pipe holds is a FAIL named at the
// deadline, never the package's timeout. Payloads are built on the test's
// goroutine; the entry's goroutine calls nothing of the test.

// rsPromptAt is rsPrompt under cbEntry's deadline.
func rsPromptAt(t *testing.T, s *planSession, promptID, text string) string {
	t.Helper()
	body := rsPromptBody(t, s, promptID, text)
	code, out, errs := cbEntry(t, "prompt "+promptID, func(o, e *bytes.Buffer) int {
		return cmdRecap([]string{"pending", install.Marker, s.id}, bytes.NewReader(body), o, e)
	})
	if code != exitOK || errs != "" {
		t.Fatalf("prompt %s exited %d, stderr %q", promptID, code, errs)
	}
	return rsSystem(t, out)
}

// rsLandAt is rsLand under cbEntry's deadline.
func rsLandAt(t *testing.T, s *planSession, promptID, useID, rel, body string) {
	t.Helper()
	in := rsPostBody(t, s, promptID, useID, rel, body)
	code, _, errs := cbEntry(t, "post "+useID, func(o, e *bytes.Buffer) int {
		return cmdPost([]string{install.Marker, s.id}, bytes.NewReader(in), o, e)
	})
	if code != exitOK || errs != "" {
		t.Fatalf("post %s exited %d, stderr %q", useID, code, errs)
	}
}

// rsLinkLoop replaces p with a link to its own name, which every open
// follows until the kernel refuses it (ELOOP): a read that fails, not a
// file that is not regular.
func rsLinkLoop(t *testing.T, p string) {
	t.Helper()
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(p), p); err != nil {
		t.Fatal(err)
	}
}

// TestRS1a_D7_W1_ARefusedManifestIsSaidOnceInItsClass (u2, u4): with the
// project's manifest a named pipe no one writes, then a link loop, a Go
// edit lands through the real post entry and returns inside the deadline;
// its overlay still runs (the run holds the overlay byte count) and leaves
// the notice owed, the class marker present only for the loop. The next
// prompt says the line once, last, in the class: not a regular file for the
// pipe, read failed for the loop. A second edit owes it again, and prompts 3
// and 4 say nothing. Break: open the manifest with os.Open, and the pipe row
// fails at the deadline; owe every refusal as a failed read, and the pipe
// row says the wrong class; drop the TakeNotice of the printed marker, and
// prompt 3 says it again.
func TestRS1a_D7_W1_ARefusedManifestIsSaidOnceInItsClass(t *testing.T) {
	for _, c := range []struct {
		name       string
		plant      func(t *testing.T, p string)
		want       string
		readFailed bool
	}{
		{"named pipe", cbFifoAt, manifestNotRegular, false},
		{"link loop", rsLinkLoop, manifestReadFailed, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := cfSession(t, store.RecordScopeV5, true)
			if got := rsPromptAt(t, s, "prompt-1", "fix "+cfGraph); strings.Contains(got, manifestNoticeHead) {
				t.Fatalf("prompt 1, nothing owed: systemMessage %q, want no manifest line", got)
			}
			c.plant(t, filepath.Join(s.project, gopg.ManifestFile))
			rsLandAt(t, s, "prompt-1", "toolu_rs1", cfGraph, "package graph\n\nfunc W1() {}\n")
			if !rsHas(t, s.root, store.FileOverlayBytes) {
				t.Fatal("witness: the run has no overlay byte count, so the edit's overlay did not run")
			}
			if !rsHas(t, s.root, store.FileNoticeManifestOwed) {
				t.Fatal("the refused manifest left no owed marker")
			}
			if got := rsHas(t, s.root, store.FileNoticeManifestReadFailed); got != c.readFailed {
				t.Fatalf("class marker present %v, want %v", got, c.readFailed)
			}
			if got := rsPromptAt(t, s, "prompt-2", "continue"); !strings.HasSuffix(got, c.want) || strings.Count(got, manifestNoticeHead) != 1 {
				t.Fatalf("prompt 2: systemMessage %q, want %q once, last", got, c.want)
			}
			rsLandAt(t, s, "prompt-2", "toolu_rs2", cfGraph, "package graph\n\nfunc W1b() {}\n")
			for _, id := range []string{"prompt-3", "prompt-4"} {
				if got := rsPromptAt(t, s, id, "continue"); strings.Contains(got, manifestNoticeHead) {
					t.Errorf("%s: systemMessage %q, want the line said once per session", id, got)
				}
			}
		})
	}
}

// TestRS1a_D7_W2_BothNoticesShareOnePrintInOrder (u3): a pipe manifest
// owes the manifest notice through a landed edit, then the index grows one
// byte past graph.MaxGraphFileBytes (sparse, nothing written). The next
// prompt prints both lines in one object, the index line first and the
// manifest line last; the prompt after prints neither. Break: join the
// notices in the other order, and the suffix fails; take only one marker
// per print, and the next prompt says the other.
func TestRS1a_D7_W2_BothNoticesShareOnePrintInOrder(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	rsPromptAt(t, s, "prompt-1", "fix "+cfGraph)
	cbFifoAt(t, filepath.Join(s.project, gopg.ManifestFile))
	rsLandAt(t, s, "prompt-1", "toolu_rs_w2", cfGraph, "package graph\n\nfunc W2() {}\n")
	if !rsHas(t, s.root, store.FileNoticeManifestOwed) {
		t.Fatal("the pipe manifest left no owed marker")
	}
	index := filepath.Join(s.project, filepath.FromSlash(graph.GraphFile))
	if err := os.Truncate(index, graph.MaxGraphFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if got := rsPromptAt(t, s, "prompt-2", "continue"); !strings.HasSuffix(got, indexCapNotice+"\n"+manifestNotRegular) ||
		strings.Count(got, indexCapNotice) != 1 || strings.Count(got, manifestNoticeHead) != 1 {
		t.Fatalf("prompt 2: systemMessage %q, want the index line then the manifest line, each once, last", got)
	}
	if got := rsPromptAt(t, s, "prompt-3", "continue"); strings.Contains(got, indexCapNotice) || strings.Contains(got, manifestNoticeHead) {
		t.Errorf("prompt 3: systemMessage %q, want neither line again", got)
	}
}

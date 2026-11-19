package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The turn's once-per-session notices (READ-SAFE1a c5 and D7), through the
// real prompt and post entries. The rows that need a named pipe or a link
// loop at the manifest are in readsafe_notice_unix_test.go.

// rsPromptBody is the prompt entry's payload for text under promptID, in
// the plan session's project.
func rsPromptBody(t testing.TB, s *planSession, promptID, text string) []byte {
	t.Helper()
	head := newBenchHead("UserPromptSubmit")
	head.CWD = s.project
	return benchJSON(t, struct {
		benchHead
		Prompt   string `json:"prompt"`
		PromptID string `json:"prompt_id"`
	}{head, text, promptID})
}

// rsPostBody writes body to the project's file rel and is the post entry's
// payload for that Write under promptID and useID.
func rsPostBody(t testing.TB, s *planSession, promptID, useID, rel, body string) []byte {
	t.Helper()
	path := filepath.Join(s.project, filepath.FromSlash(rel))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	in, err := json.Marshal(map[string]any{"file_path": path, "content": body})
	if err != nil {
		t.Fatal(err)
	}
	head := newBenchHead("PostToolUse")
	head.CWD = s.project
	return benchJSON(t, benchToolPayload{benchHead: head, PromptID: promptID, ToolName: "Write", ToolInput: in, ToolUseID: useID,
		ToolResponse: json.RawMessage(`{"stdout":"","stderr":"","interrupted":false,"isImage":false}`)})
}

// rsPrompt runs the real prompt entry on text under promptID and returns
// its systemMessage, "" when it printed none.
func rsPrompt(t *testing.T, s *planSession, promptID, text string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := cmdRecap([]string{"pending", install.Marker, s.id}, bytes.NewReader(rsPromptBody(t, s, promptID, text)), &out, &errOut); code != exitOK || errOut.Len() != 0 {
		t.Fatalf("prompt %s exited %d, stderr %q", promptID, code, errOut.String())
	}
	return rsSystem(t, out.String())
}

// rsLand runs the real post entry on a Write of body to rel.
func rsLand(t *testing.T, s *planSession, promptID, useID, rel, body string) {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := cmdPost([]string{install.Marker, s.id}, bytes.NewReader(rsPostBody(t, s, promptID, useID, rel, body)), &out, &errOut); code != exitOK || errOut.Len() != 0 {
		t.Fatalf("post %s exited %d, stderr %q", useID, code, errOut.String())
	}
}

// rsSystem is the systemMessage of one entry's stdout, "" for none.
func rsSystem(t testing.TB, stdout string) string {
	t.Helper()
	if stdout == "" {
		return ""
	}
	var o struct {
		SystemMessage string `json:"systemMessage"`
	}
	if err := json.Unmarshal([]byte(stdout), &o); err != nil {
		t.Fatalf("the entry printed %q, not one JSON object: %v", stdout, err)
	}
	return o.SystemMessage
}

// rsMarkers are the four markers of the turn's notices.
var rsMarkers = []string{store.FileNoticeIndexCap, store.FileNoticeManifestOwed, store.FileNoticeManifestReadFailed, store.FileNoticeManifest}

// rsHas reports whether the session's run directory under root holds name.
func rsHas(t testing.TB, root, name string) bool {
	t.Helper()
	st, err := store.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Lstat(filepath.Join(st.RunDir(benchSessionID), name))
	return err == nil
}

// TestRS1a_C5_AnIndexOverItsCapIsSaidOncePerSession (c5): one session over
// an index one byte past graph.MaxGraphFileBytes, three prompts. Prompt 1
// also prints the lesson, the preflight's note, and the cap line arrives in
// the same object, after any other systemMessage line; its marker is then
// present. Prompts 2 and 3 do not say it. A second session says it once
// again. Control: an index the load refuses for any other reason says
// nothing. Break: drop the TakeNotice in take, and prompt 2 says it again;
// drop the OverCap of graphBriefWith, and prompt 1 does not say it.
func TestRS1a_C5_AnIndexOverItsCapIsSaidOncePerSession(t *testing.T) {
	e := newLessonEnv(t)
	e.ledger([][2]string{{"go-test-cwd", cwdLesson}}, "go-test-cwd")
	idx := filepath.Join(e.cwd, filepath.FromSlash(graph.GraphFile))
	if err := os.MkdirAll(filepath.Dir(idx), 0o755); err != nil {
		t.Fatal(err)
	}
	// Sparse: the size the open reads is one byte past the cap, and no
	// byte of it is written.
	f, err := os.Create(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(graph.MaxGraphFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(e.root)
	if err != nil {
		t.Fatal(err)
	}

	e.start("s-cap-a")
	p1 := e.prompt("s-cap-a", "prompt-1", "continue please")
	if p1.context != cwdNote {
		t.Fatalf("prompt 1: additionalContext %q, want the lesson, the prompt's other line", p1.context)
	}
	if !strings.HasSuffix(p1.system, indexCapNotice) || strings.Count(p1.system, indexCapNotice) != 1 {
		t.Fatalf("prompt 1: systemMessage %q, want the cap line once, last", p1.system)
	}
	if !st.NoticeTaken("s-cap-a", store.FileNoticeIndexCap) {
		t.Error("after prompt 1 the session has no index cap marker")
	}
	e.call("s-cap-a", "prompt-1", "toolu_cap_1")
	for _, id := range []string{"prompt-2", "prompt-3"} {
		if out := e.prompt("s-cap-a", id, "continue please"); strings.Contains(out.system, indexCapNotice) {
			t.Errorf("%s: systemMessage %q, want the cap line said once per session", id, out.system)
		}
	}
	e.start("s-cap-b")
	if out := e.prompt("s-cap-b", "prompt-1", "continue please"); strings.Count(out.system, indexCapNotice) != 1 {
		t.Errorf("a second session, prompt 1: systemMessage %q, want the cap line once again", out.system)
	}

	if err := os.WriteFile(idx, []byte("not an index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.start("s-cap-c")
	if out := e.prompt("s-cap-c", "prompt-1", "continue please"); strings.Contains(out.stdout, "size cap") || st.NoticeTaken("s-cap-c", store.FileNoticeIndexCap) {
		t.Errorf("control, an index refused under the cap: stdout %q, want no cap line and no marker", out.stdout)
	}
}

// TestRS1a_D7_W3_ARefusedPrintGivesTheMarkerBack (u5): with the manifest
// notice owed, a print the marshal refuses (a systemMessage that is not
// UTF-8) prints nothing and gives the printed marker back, so the next
// prompt, through the real entry, says the line, in the class the owed
// markers name (a failed read), and the one after does not. Break: drop
// the giveBack after a refused marshal, and the next prompt says nothing.
func TestRS1a_D7_W3_ARefusedPrintGivesTheMarkerBack(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	if got := rsPrompt(t, s, "prompt-1", "fix "+cfGraph); strings.Contains(got, manifestNoticeHead) {
		t.Fatalf("prompt 1, nothing owed: systemMessage %q, want no manifest line", got)
	}
	st, err := store.OpenExisting(s.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.RunDir(benchSessionID), 0o700); err != nil {
		t.Fatal(err)
	}
	st.OweManifestNotice(benchSessionID, true)
	if !st.NoticeTaken(benchSessionID, store.FileNoticeManifestOwed) || !st.NoticeTaken(benchSessionID, store.FileNoticeManifestReadFailed) {
		t.Fatal("OweManifestNotice(readFailed) made no owed marker or no class marker")
	}
	tn := turnNotes{st: st, session: benchSessionID, manifest: true}
	var out bytes.Buffer
	if err := printTurn(nil, &out, steer.Output{SystemMessage: "\xff"}, steer.Client{}, "", tn); err != nil || out.Len() != 0 {
		t.Fatalf("a refused print: err %v, stdout %q, want nil and nothing", err, out.String())
	}
	if st.NoticeTaken(benchSessionID, store.FileNoticeManifest) {
		t.Fatal("the refused print kept the printed marker")
	}
	if got := rsPrompt(t, s, "prompt-2", "continue"); !strings.HasSuffix(got, manifestReadFailed) || strings.Count(got, manifestNoticeHead) != 1 {
		t.Fatalf("prompt 2: systemMessage %q, want the read failed line once, last", got)
	}
	if !st.NoticeTaken(benchSessionID, store.FileNoticeManifest) {
		t.Error("after the line printed the session has no printed marker")
	}
	if got := rsPrompt(t, s, "prompt-3", "continue"); strings.Contains(got, manifestNoticeHead) {
		t.Errorf("prompt 3: systemMessage %q, want the line said once", got)
	}
}

// TestRS1a_D7_W4_NoRefusalMakesNoMarkerAndNoLine (u5): a session whose
// manifest reads, one Go edit landed through the post entry and three
// prompts, makes none of the four markers and prints neither line. The
// witness that the edit's overlay ran, the step that reads the manifest:
// the run holds the overlay's byte count (store.FileOverlayBytes).
func TestRS1a_D7_W4_NoRefusalMakesNoMarkerAndNoLine(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	var said []string
	said = append(said, rsPrompt(t, s, "prompt-1", "fix "+cfGraph))
	rsLand(t, s, "prompt-1", "toolu_rs_w4", cfGraph, "package graph\n\nfunc W4() {}\n")
	said = append(said, rsPrompt(t, s, "prompt-2", "continue"), rsPrompt(t, s, "prompt-3", "continue"))
	if !rsHas(t, s.root, store.FileOverlayBytes) {
		t.Fatal("witness: the run has no overlay byte count, so the edit's overlay did not run")
	}
	for i, got := range said {
		if strings.Contains(got, manifestNoticeHead) || strings.Contains(got, indexCapNotice) {
			t.Errorf("prompt %d: systemMessage %q, want neither notice", i+1, got)
		}
	}
	for _, m := range rsMarkers {
		if rsHas(t, s.root, m) {
			t.Errorf("the run holds %s with no refusal", m)
		}
	}
}

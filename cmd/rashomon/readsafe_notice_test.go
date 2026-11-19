package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The turn's once-per-session notice (READ-SAFE1a c5), through the real
// prompt and post entries.

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

// rsLand runs the real post entry on a Write of body to rel and returns its
// systemMessage, "" when it printed none.
func rsLand(t *testing.T, s *planSession, promptID, useID, rel, body string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := cmdPost([]string{install.Marker, s.id}, bytes.NewReader(rsPostBody(t, s, promptID, useID, rel, body)), &out, &errOut); code != exitOK || errOut.Len() != 0 {
		t.Fatalf("post %s exited %d, stderr %q", useID, code, errOut.String())
	}
	return rsSystem(t, out.String())
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

// rsMarkers are the turn's notice marker and the post's marker for its line
// that the project's manifest file was not read.
var rsMarkers = []string{store.FileNoticeIndexCap, store.UnreadNoticeName(gopg.ManifestFile)}

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

// TestRS1a_C5_ArmOffSaysNoNoticeAndTakesNoMarker (c5): the off arm
// withholds every hook print (card MEASURE1), the turn's notice too, and
// takes no printed marker, so the session still owes it. One session over an
// index one byte past graph.MaxGraphFileBytes. Prompt 1 alone runs under the
// off arm, on a lab store: it says no cap line, and the session holds no cap
// marker. Witness, the arm unset again: prompt 2 says the line once, so the
// setup owes it. Break: drop the off arm's return in promptTurnNotes, and
// prompt 1 says the line and takes the marker.
func TestRS1a_C5_ArmOffSaysNoNoticeAndTakesNoMarker(t *testing.T) {
	e := newLessonEnv(t)
	idx := filepath.Join(e.cwd, filepath.FromSlash(graph.GraphFile))
	if err := os.MkdirAll(filepath.Dir(idx), 0o755); err != nil {
		t.Fatal(err)
	}
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
	const session = "s-off-notes"
	e.start(session)

	// The variable names an arm only on a lab store (operatorArm).
	if err := store.WriteLab(e.root); err != nil {
		t.Fatal(err)
	}
	t.Setenv(hook.EnvSteerArm, "off")
	p1 := e.prompt(session, "prompt-1", "continue please")
	if strings.Contains(p1.system, indexCapNotice) {
		t.Errorf("prompt 1, arm off: systemMessage %q, want no cap line", p1.system)
	}
	if st.NoticeTaken(session, store.FileNoticeIndexCap) {
		t.Error("prompt 1, arm off: the session holds the cap marker, want no notice taken")
	}

	t.Setenv(hook.EnvSteerArm, "")
	e.call(session, "prompt-1", "toolu_x3_1")
	p2 := e.prompt(session, "prompt-2", "continue please")
	if strings.Count(p2.system, indexCapNotice) != 1 {
		t.Fatalf("witness, prompt 2 with the arm unset: systemMessage %q, want the cap line once", p2.system)
	}
	if !st.NoticeTaken(session, store.FileNoticeIndexCap) {
		t.Error("witness, prompt 2: the session has no cap marker after the line printed")
	}
}

// TestRS1a_C5_ARefusedPrintGivesTheMarkerBack (c5): a print the marshal
// refuses (a systemMessage that is not UTF-8) prints nothing and gives the
// cap marker back, so the next prompt over an index past its cap, through
// the real entry, says the line, and the one after does not. Prompt 2's
// marker is the witness that a take works in this run directory. Break: drop
// the giveBack after a refused marshal, and the refused print keeps the
// marker, so the next prompt says nothing.
func TestRS1a_C5_ARefusedPrintGivesTheMarkerBack(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	if got := rsPrompt(t, s, "prompt-1", "fix "+cfGraph); strings.Contains(got, indexCapNotice) {
		t.Fatalf("prompt 1, the index under its cap: systemMessage %q, want no cap line", got)
	}
	st, err := store.OpenExisting(s.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.RunDir(benchSessionID), 0o700); err != nil {
		t.Fatal(err)
	}
	tn := turnNotes{st: st, session: benchSessionID, indexCap: true}
	var out bytes.Buffer
	if err := printTurn(nil, &out, steer.Output{SystemMessage: "\xff"}, steer.Client{}, "", tn); err != nil || out.Len() != 0 {
		t.Fatalf("a refused print: err %v, stdout %q, want nil and nothing", err, out.String())
	}
	if st.NoticeTaken(benchSessionID, store.FileNoticeIndexCap) {
		t.Fatal("the refused print kept the cap marker")
	}
	// Sparse: the size the load reads is one byte past the cap.
	if err := os.Truncate(filepath.Join(s.project, filepath.FromSlash(graph.GraphFile)), graph.MaxGraphFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if got := rsPrompt(t, s, "prompt-2", "continue"); !strings.HasSuffix(got, indexCapNotice) || strings.Count(got, indexCapNotice) != 1 {
		t.Fatalf("prompt 2: systemMessage %q, want the cap line once, last", got)
	}
	if !st.NoticeTaken(benchSessionID, store.FileNoticeIndexCap) {
		t.Error("after the line printed the session has no cap marker")
	}
	if got := rsPrompt(t, s, "prompt-3", "continue"); strings.Contains(got, indexCapNotice) {
		t.Errorf("prompt 3: systemMessage %q, want the line said once", got)
	}
}

// TestRS1a_AManifestThatReadsSaysNothing: a session whose manifest reads,
// one Go edit landed through the post entry and three prompts, prints no cap
// line and no line that a file was not read, and makes neither marker
// (rsMarkers). The witness that the edit's overlay ran, the step that reads
// the manifest: the run holds the overlay's byte count
// (store.FileOverlayBytes).
func TestRS1a_AManifestThatReadsSaysNothing(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	said := [][2]string{{"prompt 1", rsPrompt(t, s, "prompt-1", "fix "+cfGraph)}}
	said = append(said, [2]string{"the post", rsLand(t, s, "prompt-1", "toolu_rs_w4", cfGraph, "package graph\n\nfunc W4() {}\n")})
	said = append(said, [2]string{"prompt 2", rsPrompt(t, s, "prompt-2", "continue")}, [2]string{"prompt 3", rsPrompt(t, s, "prompt-3", "continue")})
	if !rsHas(t, s.root, store.FileOverlayBytes) {
		t.Fatal("witness: the run has no overlay byte count, so the edit's overlay did not run")
	}
	for _, p := range said {
		if strings.Contains(p[1], " was not read (") || strings.Contains(p[1], indexCapNotice) {
			t.Errorf("%s: systemMessage %q, want no cap line and no unread line", p[0], p[1])
		}
	}
	for _, m := range rsMarkers {
		if rsHas(t, s.root, m) {
			t.Errorf("the run holds %s with no refusal", m)
		}
	}
}

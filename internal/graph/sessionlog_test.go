package graph

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

func testLog(t *testing.T, allowed bool) (*SessionLog, string) {
	t.Helper()
	root := t.TempDir()
	return OpenSessionLog(root, "github.com/x/y", RevCommit("c1"), allowed), root
}

// fullSession is a session of all six kinds and all nine predicates.
func fullSession(l *SessionLog) []Record {
	exit := 1
	return []Record{
		l.Session("s1"),
		l.Prompt("s1", 1, "make session expiry respect the org timeout"),
		l.Call("s1", "tu1", CallFields{Tool: "Bash", Command: "go test ./...", CWD: "/r", Exit: &exit, DurationMS: 5, Output: "FAIL x", Runner: true}),
		l.Call("s1", "tu2", CallFields{Tool: "Read", Path: "a/a.go"}),
		l.Call("s1", "tu3", CallFields{Tool: "Edit", Path: "a/a.go", Diff: "-old\n+new\n"}),
		l.Claim("s1", 1, "all tests pass", "pass", "unbacked"),
		l.Note("s1", 1, "PostToolUse", "gate go exited 1"),
		l.Verdict("s1", 1, "go", "failed"),
		l.Edge(SessionNodeID("s1"), References, PromptNodeID("s1", 1)),
		l.Edge(SessionNodeID("s1"), Calls, CallNodeID("s1", "tu1")),
		l.Edge(CallNodeID("s1", "tu2"), Reads, FileID("a/a.go")),
		l.Edge(CallNodeID("s1", "tu3"), Writes, FileID("a/a.go")),
		l.Edge(CallNodeID("s1", "tu1"), Declares, VerdictNodeID("s1", 1)),
		l.Edge(PromptNodeID("s1", 1), Contains, ClaimNodeID("s1", 1)),
		l.Edge(ClaimNodeID("s1", 1), Imports, CallNodeID("s1", "tu1")),
		l.Edge(SessionNodeID("s1"), Exposes, NoteNodeID("s1", 1)),
		l.Edge(NoteNodeID("s1", 1), Consumes, CallNodeID("s1", "tu2")),
	}
}

// TestSessionLog_AppendLoad (W-M2.11, 15(a)): two appends make one file
// with one header; Load into a store gives every node and edge back at the
// log's rev; the file is under the store root, never the code index.
func TestSessionLog_AppendLoad(t *testing.T) {
	l, root := testLog(t, true)
	recs := fullSession(l)
	if err := l.Append(recs[:5]...); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(recs[5:]...); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), `"schema"`); n != 1 {
		t.Errorf("%d header lines, want 1", n)
	}
	if strings.Contains(l.Path, GraphFile) || !strings.HasPrefix(l.Path, root) {
		t.Errorf("session log at %s, want under the store root %s and not %s", l.Path, root, GraphFile)
	}
	if l.Path != SessionLogPath(root, "github.com/x/y") {
		t.Errorf("path %s != SessionLogPath", l.Path)
	}
	s := NewStore()
	h, skipped, err := LoadSessionLog(s, bytes.NewReader(b))
	if err != nil || skipped != 0 || h.Repo != "github.com/x/y" || h.Rev.Commit != "c1" {
		t.Fatalf("header %+v err %v", h, err)
	}
	rev := RevCommit("c1")
	if got := len(s.Nodes(rev)); got != 8 {
		t.Errorf("%d nodes, want 8", got)
	}
	if got := len(s.Edges(rev)); got != 9 {
		t.Errorf("%d edges, want 9", got)
	}
	if e, ok := s.Edge(CallNodeID("s1", "tu2"), Reads, FileID("a/a.go"), rev); !ok || e.Tier != Observed {
		t.Errorf("reads_file edge = %+v, %v", e, ok)
	}
	n, _ := s.Node(CallNodeID("s1", "tu1"), rev)
	if n.Kind != KindCall || n.Attrs["command"] != "go test ./..." || n.Attrs["exit"] != "1" || n.Attrs["output"] != "FAIL x" {
		t.Errorf("call node = %+v", n)
	}
	for _, k := range SessionKinds() {
		if !IsSessionKind(k) {
			t.Errorf("%s not a session kind", k)
		}
	}
	if len(SessionKinds()) != 6 || IsSessionKind("unit") || IsSessionKind("lesson") {
		t.Errorf("session kinds = %v", SessionKinds())
	}
}

// TestSessionLog_TornTail: a kill mid-append leaves a part line; the next
// append starts a new line and Load skips the part.
func TestSessionLog_TornTail(t *testing.T) {
	l, _ := testLog(t, true)
	if err := l.Append(l.Session("s1")); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(l.Path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"Node":{"ID":"call:s1:tu`)
	f.Close()
	if err := l.Append(l.Prompt("s1", 1, "p")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(l.Path)
	s := NewStore()
	_, skipped, err := LoadSessionLog(s, bytes.NewReader(b))
	if err != nil || skipped != 1 {
		t.Fatalf("skipped %d, err %v; want the one part line skipped", skipped, err)
	}
	if got := len(s.Nodes(RevCommit("c1"))); got != 2 {
		t.Errorf("%d nodes after a torn tail, want 2", got)
	}
	if _, ok := s.Node(PromptNodeID("s1", 1), RevCommit("c1")); !ok {
		t.Errorf("the record appended after the part line did not load")
	}
	// the part line is now a middle line: the code index's Load refuses it,
	// which is why the session log has its own loader.
	if _, _, err := Load(bytes.NewReader(b)); err == nil {
		t.Errorf("Load accepted a file with a part line in the middle")
	}
}

// TestSessionLog_NoConsentWritesNothing (F3): a log opened without session
// consent refuses and creates no file or directory.
func TestSessionLog_NoConsentWritesNothing(t *testing.T) {
	l, root := testLog(t, false)
	err := l.Append(l.Session("s1"))
	if !errors.Is(err, ErrNoSessionConsent) {
		t.Fatalf("err = %v, want ErrNoSessionConsent", err)
	}
	if ents, _ := os.ReadDir(root); len(ents) != 0 {
		t.Errorf("store root holds %d entries after a refused append", len(ents))
	}
}

// TestSessionLog_Caps (15, caps): a command is cut at the store's command
// cap with command_cut set; an output keeps its tail; a runner's output is
// whole up to its own cap; cuts fall on rune boundaries.
func TestSessionLog_Caps(t *testing.T) {
	if SessionCommandCap != store.MaxCommandBytes {
		t.Errorf("SessionCommandCap %d != store.MaxCommandBytes %d", SessionCommandCap, store.MaxCommandBytes)
	}
	l, _ := testLog(t, true)
	big := strings.Repeat("é", SessionCommandCap) // 2 bytes each
	n := l.Call("s1", "tu", CallFields{Tool: "Bash", Command: big, Output: strings.Repeat("x", 20000) + "END"}).Node
	if got := len(n.Attrs["command"]); got > SessionCommandCap || n.Attrs["command_cut"] == "" {
		t.Errorf("command %d bytes, cut %q", got, n.Attrs["command_cut"])
	}
	if !strings.HasSuffix(n.Attrs["output"], "END") || len(n.Attrs["output"]) > SessionOutputTail || n.Attrs["output_cut"] == "" {
		t.Errorf("output tail %d bytes, cut %q", len(n.Attrs["output"]), n.Attrs["output_cut"])
	}
	for _, f := range []string{"command", "output"} {
		for _, r := range n.Attrs[f] {
			if r == '�' {
				t.Fatalf("%s cut inside a rune", f)
			}
		}
	}
	out := strings.Repeat("y", 100000)
	w := l.Call("s1", "tu2", CallFields{Tool: "Bash", Command: "go test", Output: out, Runner: true}).Node
	if w.Attrs["output"] != out || w.Attrs["output_cut"] != "" {
		t.Errorf("runner output not whole: %d bytes, cut %q", len(w.Attrs["output"]), w.Attrs["output_cut"])
	}
	short := l.Call("s1", "tu3", CallFields{Tool: "Bash", Command: "ls"}).Node
	if _, has := short.Attrs["command_cut"]; has {
		t.Errorf("an uncut command carries a cut marker")
	}
}

// TestSessionLog_NeverInCodeIndex (15(e)): the path is never the code
// index's, whatever the rid holds.
func TestSessionLog_NeverInCodeIndex(t *testing.T) {
	root := t.TempDir()
	for _, rid := range []string{"github.com/x/y", "../../.rashomon/graph", "path:ab12"} {
		p := SessionLogPath(root, rid)
		if filepath.Dir(p) != filepath.Join(root, "graph") || strings.HasSuffix(p, "graph.jsonl") {
			t.Errorf("rid %q -> %s", rid, p)
		}
	}
}

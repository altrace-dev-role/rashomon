package graph

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestGraph_MaxOverlayBytesEqualsStore: the store's reservation bound and
// the graph's are one number. store never imports graph, so a test here, where
// both are visible, holds them equal (SessionCommandCap's precedent).
func TestGraph_MaxOverlayBytesEqualsStore(t *testing.T) {
	if MaxOverlayBytes != store.MaxOverlayBytes {
		t.Errorf("graph.MaxOverlayBytes %d != store.MaxOverlayBytes %d", MaxOverlayBytes, store.MaxOverlayBytes)
	}
}

// TestGraph_ReadIndexHeaderReadsTheFirstLineOnly: the header of an index
// whose records are not even valid JSON is still read, and a file that is no
// index of this schema is refused.
func TestGraph_ReadIndexHeaderReadsTheFirstLineOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.jsonl")
	body := `{"schema":1,"repo":"r","rev":{"commit":"abc123"},"producers":{"go":"x"},"records":2}` + "\n" + "{not json at all\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := ReadIndexHeader(path)
	if err != nil || h.Rev.Commit != "abc123" || h.Repo != "r" {
		t.Errorf("ReadIndexHeader: %+v, %v", h, err)
	}
	if err := os.WriteFile(path, []byte(`{"schema":99,"repo":"r","rev":{"commit":"c"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadIndexHeader(path); err == nil {
		t.Error("a header of another schema was accepted")
	}
	if _, err := ReadIndexHeader(filepath.Join(dir, "absent")); err == nil {
		t.Error("an absent file was accepted")
	}
}

const childLogEnv = "RASHOMON_SESSIONLOG_CHILD"

// TestSessionLog_TwoProcessesAppendWholeGroups (C7): the session log has no
// lock, only one write(2) on an O_APPEND handle per Append, so two processes'
// groups can interleave as whole groups and never inside one. The test binary
// re-executes itself as the second writer (res7_test's shape): parent and
// child each append k groups of many records to one log, and every line of the
// log must parse into a whole group, 2k in all, none skipped. A two-goroutine
// test would not do: this atomicity is the kernel's, across processes.
func TestSessionLog_TwoProcessesAppendWholeGroups(t *testing.T) {
	const k, names = 40, 60
	appendK := func(l *SessionLog, session string, n int) {
		for i := 0; i < n; i++ {
			file := FileID("a/" + session + ".go")
			ns := make([]string, names)
			for j := range ns {
				ns[j] = session + "_" + strconv.Itoa(i) + "_" + strconv.Itoa(j) + strings.Repeat("x", 40)
			}
			g := ovGroup(session, "toolu_"+session+strconv.Itoa(i), OverlayDeclared, int64(i+1), file, "d"+strconv.Itoa(i), ns...)
			if err := l.AppendOverlayGroup(BuildOverlayGroup(g), nil); err != nil {
				t.Errorf("%s group %d: %v", session, i, err)
				return
			}
		}
	}
	if dir := os.Getenv(childLogEnv); dir != "" {
		appendK(OpenSessionLog(dir, "rid", RevCommit(ovCommit), true), "child", k)
		return
	}
	dir := t.TempDir()
	l := OpenSessionLog(dir, "rid", RevCommit(ovCommit), true)
	// The parent's first group (one of its k) makes the file, so the header
	// is written once.
	g := ovGroup("parent", "toolu_parent_first", OverlayDeclared, 1000, FileID("a/parent.go"), "dfirst", "f")
	appendGroup(t, l, g)
	cmd := exec.Command(os.Args[0], "-test.run=^TestSessionLog_TwoProcessesAppendWholeGroups$", "-test.count=1")
	cmd.Env = append(os.Environ(), childLogEnv+"="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	appendK(l, "parent", k-1)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("the child writer: %v", err)
	}
	f, err := os.Open(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, skipped, groups, err := LoadSessionLogGroups(NewStore(), f)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]int{}
	for _, g := range groups {
		by[g.Session]++
		if len(g.Recs) != 1+2*names && g.ToolUseID != "toolu_parent_first" {
			t.Errorf("group %s holds %d records, want %d", g.ToolUseID, len(g.Recs), 1+2*names)
		}
	}
	if skipped != 0 || len(groups) != 2*k || by["parent"] != k || by["child"] != k {
		t.Errorf("skipped %d lines, %d groups (%v), want 0 skipped and %d groups", skipped, len(groups), by, 2*k)
	}
}

package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

const (
	ovOrig   = "package pkg\n\nfunc Old() {}\n"
	ovCommit = "c1"
	ovRel    = "pkg/a.go"
	ovSess   = "sess-ov"
)

// ovFix is a repository with a code index (header and the file's own
// declarations at commit c1), one Go file, and a store whose session-content
// marker is whatever marker says ("" leaves it absent).
type ovFix struct {
	t    *testing.T
	st   *store.Store
	repo string
	file string
	rid  string
}

func newOvFix(t *testing.T, marker string) *ovFix {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	repo := t.TempDir()
	file := filepath.Join(repo, filepath.FromSlash(ovRel))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(ovOrig), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := graph.NewStore()
	recs, err := gopg.Resolver{}.Overlay(ovRel, []byte(ovOrig), graph.RevCommit(ovCommit), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.Node != nil {
			n := *r.Node
			n.Rev = graph.RevCommit(ovCommit)
			idx.AssertNode(n)
		}
		if r.Edge != nil {
			e := *r.Edge
			e.Rev = graph.RevCommit(ovCommit)
			idx.AssertEdge(e)
		}
	}
	if err := graph.SaveFile(repo, idx, graph.Header{Repo: "r", Rev: graph.HeaderRev{Commit: ovCommit}}); err != nil {
		t.Fatal(err)
	}
	if marker != "" {
		if err := os.WriteFile(filepath.Join(st.Root(), store.FileSessionContent), []byte(marker), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rid, _ := baseline.RepoID(repo)
	return &ovFix{t: t, st: st, repo: repo, file: file, rid: rid}
}

func (f *ovFix) payload(event, id, tool string, input map[string]any) string {
	f.t.Helper()
	p := map[string]any{"session_id": ovSess, "tool_use_id": id, "tool_name": tool, "hook_event_name": event,
		"cwd": f.repo, "tool_input": input}
	if event == "PostToolUse" {
		p["tool_response"] = map[string]any{"stdout": "ok"}
	}
	b, err := json.Marshal(p)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *ovFix) pre(id, tool string, input map[string]any) {
	f.t.Helper()
	h := New(f.st, time.Now)
	h.Close(&Signals{}, h.Capture(strings.NewReader(f.payload("PreToolUse", id, tool, input))))
}

func (f *ovFix) post(id, tool string, input map[string]any) {
	f.t.Helper()
	p := NewPost(f.st, time.Now)
	p.Close(&Signals{}, p.Capture(strings.NewReader(f.payload("PostToolUse", id, tool, input))))
}

func (f *ovFix) edit(oldS, newS string) map[string]any {
	return map[string]any{"file_path": f.file, "old_string": oldS, "new_string": newS}
}

// logPath is the session log of the fixture's repository.
func (f *ovFix) logPath() string { return graph.SessionLogPath(f.st.Root(), f.rid) }

// marker is one overlay group marker as the raw log holds it, with the
// number of record lines between it and the line before its group.
type ovMarker struct {
	attrs map[string]string
	lines int
}

// read returns the log's complete groups and its raw markers; exists is false
// when no log file was made.
func (f *ovFix) read() (groups []graph.OverlayGroup, markers []ovMarker, exists bool) {
	f.t.Helper()
	b, err := os.ReadFile(f.logPath())
	if os.IsNotExist(err) {
		return nil, nil, false
	}
	if err != nil {
		f.t.Fatal(err)
	}
	_, skipped, groups, err := graph.LoadSessionLogGroups(graph.NewStore(), bytes.NewReader(b))
	if err != nil || skipped != 0 {
		f.t.Fatalf("loading the log: skipped %d, %v", skipped, err)
	}
	n := 0
	for i, line := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
		if i == 0 {
			continue
		}
		var r graph.Record
		if err := json.Unmarshal(line, &r); err != nil {
			f.t.Fatal(err)
		}
		if r.Node != nil && r.Node.Kind == graph.KindOverlayGroup {
			markers = append(markers, ovMarker{r.Node.Attrs, n})
			n = 0
			continue
		}
		n++
	}
	for _, m := range markers {
		if m.attrs["seq"] == "0" || m.attrs["seq"] == "" {
			f.t.Errorf("a group marker carries seq %q", m.attrs["seq"])
		}
	}
	return groups, markers, true
}

func (f *ovFix) declSeqs() map[string]int64 {
	f.t.Helper()
	run, err := f.st.ReadRun(ovSess)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]int64{}
	for _, d := range run.Declarations {
		out[d.ToolUseID] = d.Seq
	}
	return out
}

func (f *ovFix) execSeqs() map[string]int64 {
	f.t.Helper()
	run, err := f.st.ReadRun(ovSess)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]int64{}
	for _, x := range run.Executions {
		if x.Seq != nil {
			out[x.ToolUseID] = *x.Seq
		}
	}
	return out
}

// TestHook_PreWritesDeclaredGroupAtTheDeclarationSeq: the group's marker names
// the declaration's seq, is declared, carries the call's tool_use_id, and its
// count is the records it holds.
func TestHook_PreWritesDeclaredGroupAtTheDeclarationSeq(t *testing.T) {
	f := newOvFix(t, "1\n")
	f.pre("toolu_pre", "Edit", f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}"))
	groups, markers, ok := f.read()
	if !ok || len(groups) != 1 || len(markers) != 1 {
		t.Fatalf("log exists %v, %d groups, %d markers; want one group", ok, len(groups), len(markers))
	}
	g, m := groups[0], markers[0]
	if want := f.declSeqs()["toolu_pre"]; want == 0 || g.Seq != want || m.attrs["seq"] != strconv.FormatInt(want, 10) {
		t.Errorf("group seq %d (marker %s), the declaration's is %d", g.Seq, m.attrs["seq"], want)
	}
	if g.Kind != graph.OverlayDeclared || g.ToolUseID != "toolu_pre" || g.Session != ovSess || g.Commit != ovCommit {
		t.Errorf("group %+v", g)
	}
	if m.attrs["count"] != strconv.Itoa(m.lines) || m.lines != len(g.Recs) || m.lines == 0 {
		t.Errorf("marker count %s, %d record lines, %d records", m.attrs["count"], m.lines, len(g.Recs))
	}
	if g.File != graph.FileID(ovRel) {
		t.Errorf("file %s", g.File)
	}
	want := ovOrig[:len(ovOrig)-len("func Old() {}\n")] + "func Old() {}\n\nfunc Added() {}\n"
	if g.Digest != graph.ContentDigest([]byte(want)) {
		t.Errorf("digest %s is not the edited content's", g.Digest)
	}
}

// TestHook_PostWritesLandedGroupAtTheExecutionSeq: the landed group is at the
// seq the execution landed at, from the bytes on disk after the tool.
func TestHook_PostWritesLandedGroupAtTheExecutionSeq(t *testing.T) {
	f := newOvFix(t, "1\n")
	in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")
	f.pre("toolu_x", "Edit", in)
	after := "package pkg\n\nfunc Old() {}\n\nfunc Added() {}\n\nfunc OnDisk() {}\n"
	if err := os.WriteFile(f.file, []byte(after), 0o644); err != nil {
		t.Fatal(err)
	}
	f.post("toolu_x", "Edit", in)
	groups, markers, _ := f.read()
	if len(groups) != 2 {
		t.Fatalf("%d groups, want the declared and the landed", len(groups))
	}
	g, m := groups[1], markers[1]
	want := f.execSeqs()["toolu_x"]
	if want == 0 || g.Seq != want || g.Kind != graph.OverlayLanded || g.ToolUseID != "toolu_x" || m.attrs["kind"] != "landed" {
		t.Errorf("landed group seq %d kind %s tool %s; the execution is at %d", g.Seq, g.Kind, g.ToolUseID, want)
	}
	if g.Digest != graph.ContentDigest([]byte(after)) {
		t.Error("the landed digest is not the digest of the file on disk")
	}
	if m.attrs["count"] != strconv.Itoa(m.lines) || m.lines != len(g.Recs) || m.lines == 0 {
		t.Errorf("marker count %s, %d record lines, %d records", m.attrs["count"], m.lines, len(g.Recs))
	}
}

// TestHook_EditSpliceNotExactWritesNoGroup: an old_string that is absent, and
// one that occurs twice without replace_all, write no group, where the
// declaration itself lands.
func TestHook_EditSpliceNotExactWritesNoGroup(t *testing.T) {
	f := newOvFix(t, "1\n")
	if err := os.WriteFile(f.file, []byte("package pkg\n\nvar a = 1\nvar b = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.pre("toolu_absent", "Edit", f.edit("not in the file", "x"))
	f.pre("toolu_twice", "Edit", f.edit("= 1", "= 2"))
	f.pre("toolu_multi", "MultiEdit", map[string]any{"file_path": f.file, "edits": []map[string]any{
		{"old_string": "var a", "new_string": "var c"}, {"old_string": "var a", "new_string": "var d"}}})
	if got := f.declSeqs(); len(got) != 3 || got["toolu_twice"] == 0 {
		t.Fatalf("declarations %v", got)
	}
	if _, _, ok := f.read(); ok {
		groups, _, _ := f.read()
		t.Errorf("a group was written for an inexact splice: %d groups", len(groups))
	}
	// Control: an exact splice of the same file writes its group.
	f.pre("toolu_exact", "Edit", f.edit("var a = 1", "var a = 2"))
	if groups, _, _ := f.read(); len(groups) != 1 {
		t.Errorf("control: %d groups, want 1", len(groups))
	}
}

// TestHook_MultiEditAppliesInOrder (C2): the second edit's old_string exists
// only in the first edit's result, and the group's digest is the digest of the
// running result; replace_all replaces every occurrence; the bytes are as on
// disk, byte-order mark and CRLF included.
func TestHook_MultiEditAppliesInOrder(t *testing.T) {
	cases := []struct {
		name, disk string
		edits      []map[string]any
		want       string
	}{
		{"in order", ovOrig, []map[string]any{
			{"old_string": "func Old() {}", "new_string": "func Mid() {}"},
			{"old_string": "func Mid() {}", "new_string": "func Mid() {}\n\nfunc Late() {}"}},
			"package pkg\n\nfunc Mid() {}\n\nfunc Late() {}\n"},
		{"replace_all over three", "package pkg\n\nvar a = 1\nvar b = 1\nvar c = 1\n", []map[string]any{
			{"old_string": "= 1", "new_string": "= 2", "replace_all": true}},
			"package pkg\n\nvar a = 2\nvar b = 2\nvar c = 2\n"},
		{"bytes as on disk", "\xef\xbb\xbfpackage pkg\r\n\r\nfunc Old() {}\r\n", []map[string]any{
			{"old_string": "Old", "new_string": "New"}},
			"\xef\xbb\xbfpackage pkg\r\n\r\nfunc New() {}\r\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newOvFix(t, "1\n")
			if err := os.WriteFile(f.file, []byte(c.disk), 0o644); err != nil {
				t.Fatal(err)
			}
			f.pre("toolu_me", "MultiEdit", map[string]any{"file_path": f.file, "edits": c.edits})
			groups, _, _ := f.read()
			if len(groups) != 1 {
				t.Fatalf("%d groups, want 1", len(groups))
			}
			if groups[0].Digest != graph.ContentDigest([]byte(c.want)) {
				t.Errorf("digest is not the running result's %q", c.want)
			}
		})
	}
}

func ovGoSource(n int) string {
	head := "package pkg\n\nfunc F() {}\n//"
	return head + strings.Repeat("a", n-len(head))
}

// TestHook_ContentOverCapWritesNoGroup (C2): a Write of MaxOverlayContentBytes+1
// bytes and a Post over a file of that size write no group; at exactly the cap
// both do.
func TestHook_ContentOverCapWritesNoGroup(t *testing.T) {
	for _, tc := range []struct {
		n     int
		kinds []string
	}{
		{MaxOverlayContentBytes + 1, nil},
		{MaxOverlayContentBytes, []string{"declared", "landed"}},
	} {
		f := newOvFix(t, "1\n")
		src := ovGoSource(tc.n)
		in := map[string]any{"file_path": f.file, "content": src}
		f.pre("toolu_w", "Write", in)
		if err := os.WriteFile(f.file, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		f.post("toolu_w", "Write", in)
		groups, _, _ := f.read()
		var kinds []string
		for _, g := range groups {
			kinds = append(kinds, g.Kind)
		}
		if !reflect.DeepEqual(kinds, tc.kinds) {
			t.Errorf("%d bytes: groups %v, want %v", tc.n, kinds, tc.kinds)
		}
	}
}

// counted replaces the three costly steps with counting wrappers.
func counted(t *testing.T) (read, header, resolve *int) {
	t.Helper()
	r, h, s := 0, 0, 0
	or, oh, os_ := overlayReadFile, overlayReadHeader, overlayResolve
	overlayReadFile = func(p string) ([]byte, error) { r++; return or(p) }
	overlayReadHeader = func(p string) (graph.Header, error) { h++; return oh(p) }
	overlayResolve = func(rel string, c []byte, b graph.Rev, q int64) ([]graph.Record, error) {
		s++
		return os_(rel, c, b, q)
	}
	t.Cleanup(func() { overlayReadFile, overlayReadHeader, overlayResolve = or, oh, os_ })
	return &r, &h, &s
}

// TestHook_NoConsentReadsNothing (C3): with the marker absent, the file read,
// the index header read and the resolver call run zero times, and no writer
// leaves a file in the run directory (no group, no tally); with it present,
// each runs. The control is the marker-present half.
func TestHook_NoConsentReadsNothing(t *testing.T) {
	read, header, resolve := counted(t)
	f := newOvFix(t, "")
	in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")
	f.pre("toolu_nc", "Edit", in)
	f.post("toolu_nc", "Edit", in)
	if *read != 0 || *header != 0 || *resolve != 0 {
		t.Errorf("marker absent: %d file reads, %d header reads, %d resolver calls; want none", *read, *header, *resolve)
	}
	if names := overlayRunFileNames(t, f); len(names) != 0 {
		t.Errorf("marker absent: the writers left %v in the run directory", names)
	}

	read, header, resolve = counted(t)
	g := newOvFix(t, "1\n")
	g.pre("toolu_c", "Edit", g.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}"))
	g.post("toolu_c", "Edit", g.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}"))
	if *read < 2 || *header < 2 || *resolve < 2 {
		t.Errorf("marker present: %d file reads, %d header reads, %d resolver calls; want each from both writers", *read, *header, *resolve)
	}
	if names := overlayRunFileNames(t, g); len(names) == 0 {
		t.Errorf("marker present: no overlay file in the run directory; the control would pass for nothing")
	}
}

// overlayRunFileNames lists the overlay's own files (overlay-*) in the
// session's run directory.
func overlayRunFileNames(t *testing.T, f *ovFix) []string {
	t.Helper()
	ents, err := os.ReadDir(f.st.RunDir(ovSess))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "overlay-") {
			out = append(out, e.Name())
		}
	}
	return out
}

// TestHook_NoOverlayOrSessionRecordBelowScope6 (ruling 2): with the marker
// absent, and with it "0", Pre and Post of an Edit of a Go file with an index
// present write no session log and no overlay file; with "1" the same calls
// write them.
func TestHook_NoOverlayOrSessionRecordBelowScope6(t *testing.T) {
	for _, tc := range []struct {
		marker  string
		written bool
	}{{"", false}, {"0\n", false}, {"1\n", true}} {
		f := newOvFix(t, tc.marker)
		in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")
		f.pre("toolu_s", "Edit", in)
		f.post("toolu_s", "Edit", in)
		_, err := os.Stat(f.logPath())
		if (err == nil) != tc.written {
			t.Errorf("marker %q: session log exists %v, want %v", tc.marker, err == nil, tc.written)
		}
		if names := overlayRunFileNames(t, f); (len(names) > 0) != tc.written {
			t.Errorf("marker %q: overlay files %v, want some %v", tc.marker, names, tc.written)
		}
	}
}

// TestHook_OverlayOnlyForGoWrites: a Bash call, a Read, and an Edit of a file
// the Go resolver does not take write nothing and count nothing.
func TestHook_OverlayOnlyForGoWrites(t *testing.T) {
	f := newOvFix(t, "")
	md := filepath.Join(f.repo, "README.md")
	if err := os.WriteFile(md, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.pre("toolu_md", "Edit", map[string]any{"file_path": md, "old_string": "hello", "new_string": "bye"})
	f.pre("toolu_b", "Bash", map[string]any{"command": "ls"})
	f.pre("toolu_r", "Read", map[string]any{"file_path": f.file})
	f.post("toolu_md", "Edit", map[string]any{"file_path": md, "old_string": "hello", "new_string": "bye"})
	if names := overlayRunFileNames(t, f); len(names) != 0 {
		t.Errorf("overlay files %v for calls the overlay never concerns", names)
	}
}

// TestOverlay_HookGroupsRoundTrip (H5): the groups Pre and Post wrote, loaded
// and replayed, give the store ApplyOverlay gives live for the same content.
// The declared group is dropped at replay as the landed one for the same call
// supersedes it, so the live store is the landed content at the landed seq.
func TestOverlay_HookGroupsRoundTrip(t *testing.T) {
	f := newOvFix(t, "1\n")
	in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")
	f.pre("toolu_rt", "Edit", in)
	after := "package pkg\n\nfunc Old() {}\n\nfunc Added() {}\n"
	if err := os.WriteFile(f.file, []byte(after), 0o644); err != nil {
		t.Fatal(err)
	}
	f.post("toolu_rt", "Edit", in)

	replayed, _, err := graph.LoadFile(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.logPath())
	if err != nil {
		t.Fatal(err)
	}
	_, _, groups, err := graph.LoadSessionLogGroups(replayed, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if n := graph.ReplayOverlays(replayed, groups, ovSess, ovCommit); n != 1 {
		t.Fatalf("replay applied %d groups, want the landed one", n)
	}

	seq := f.execSeqs()["toolu_rt"]
	live, _, err := graph.LoadFile(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	rev := graph.RevOverlay(ovCommit, seq)
	recs, err := gopg.Resolver{}.Overlay(ovRel, []byte(after), graph.RevCommit(ovCommit), seq)
	if err != nil {
		t.Fatal(err)
	}
	live.ApplyOverlay(graph.FileID(ovRel), []byte(after), recs, rev, gopg.Resolver{}.OverlayCarried())
	if !reflect.DeepEqual(live.Nodes(rev), replayed.Nodes(rev)) || !reflect.DeepEqual(live.Edges(rev), replayed.Edges(rev)) {
		t.Errorf("replayed store differs from the live one:\nlive %v\nreplayed %v", live.Nodes(rev), replayed.Nodes(rev))
	}
	if !reflect.DeepEqual(live.Overlays(), replayed.Overlays()) {
		t.Errorf("overlays differ: %+v vs %+v", live.Overlays(), replayed.Overlays())
	}
}

// TestOverlay_DeclaredEditAppearsAtCommitOv1 (AC-M2.4, unfiltered replay): a
// symbol a declared Edit adds is present to a reader at RevOverlay(commit, 1),
// the declaration's seq, after the log is loaded and replayed with no
// pre-filter (the disk-digest pre-filter is W-M2.4's).
func TestOverlay_DeclaredEditAppearsAtCommitOv1(t *testing.T) {
	f := newOvFix(t, "1\n")
	f.pre("toolu_d", "Edit", f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}"))
	s, _, err := graph.LoadFile(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.logPath())
	if err != nil {
		t.Fatal(err)
	}
	_, _, groups, err := graph.LoadSessionLogGroups(s, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if n := graph.ReplayOverlays(s, groups, ovSess, ovCommit); n != 1 {
		t.Fatalf("replay applied %d", n)
	}
	added := graph.SymID(gopg.Lang, "pkg", "func", "Added")
	if _, ok := s.Node(added, graph.RevCommit(ovCommit)); ok {
		t.Error("the added symbol is in the index before the overlay")
	}
	if _, ok := s.Node(added, graph.RevOverlay(ovCommit, 1)); !ok {
		t.Error("the added symbol is absent at commit ov1")
	}
}

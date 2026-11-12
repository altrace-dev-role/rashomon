package graph

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const ovCommit = "c1"

// ovRecs is a resolver's records for file with the named functions: a file
// node whose digest attr is dig, a sym per name and a Declares edge each.
func ovRecs(file ID, dig string, names ...string) []Record {
	rev := RevOverlay(ovCommit, 0)
	out := []Record{{Node: &Node{ID: file, Kind: "file", Attrs: map[string]string{"digest": dig}, Tier: Algorithmic, Conf: 0.8, Rev: rev}}}
	for _, n := range names {
		id := SymID("go", "a", "func", n)
		out = append(out,
			Record{Node: &Node{ID: id, Kind: "sym", Attrs: map[string]string{"name": n, "symkind": "func"}, Tier: Algorithmic, Conf: 0.8, Rev: rev}},
			Record{Edge: &Edge{Src: file, Pred: Declares, Dst: id, Tier: Algorithmic, Conf: 0.8, Rev: rev}})
	}
	return out
}

func ovGroup(sid, tu, kind string, seq int64, file ID, dig string, names ...string) OverlayGroup {
	return OverlayGroup{Session: sid, ToolUseID: tu, Kind: kind, File: file, Digest: dig, Commit: ovCommit, Seq: seq, Recs: ovRecs(file, dig, names...)}
}

// ovIndex is the code index at ovCommit: a.go declares f and g, f calls g.
func ovIndex() *Store {
	s := NewStore()
	c := RevCommit(ovCommit)
	file := FileID("a/a.go")
	s.AssertNode(Node{ID: file, Kind: "file", Attrs: map[string]string{"digest": "old"}, Tier: Proven, Conf: 1, Rev: c})
	for _, n := range []string{"f", "g"} {
		id := SymID("go", "a", "func", n)
		s.AssertNode(Node{ID: id, Kind: "sym", Attrs: map[string]string{"name": n}, Tier: Proven, Conf: 1, Rev: c})
		s.AssertEdge(Edge{Src: file, Pred: Declares, Dst: id, Tier: Proven, Conf: 1, Rev: c})
	}
	s.AssertEdge(Edge{Src: SymID("go", "a", "func", "f"), Pred: Calls, Dst: SymID("go", "a", "func", "g"), Tier: Proven, Conf: 1, Rev: c})
	return s
}

func appendGroup(t *testing.T, l *SessionLog, g OverlayGroup) {
	t.Helper()
	if err := l.AppendOverlayGroup(BuildOverlayGroup(g), nil); err != nil {
		t.Fatal(err)
	}
}

func loadGroups(t *testing.T, l *SessionLog, s *Store) ([]OverlayGroup, int) {
	t.Helper()
	b, err := os.ReadFile(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	_, skipped, groups, err := LoadSessionLogGroups(s, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return groups, skipped
}

// TestOverlayRecords_CutMarked: a group over MaxOverlayRecords keeps at most
// that many records before its marker, the file node says _cut=true, no kept
// edge points at a dropped symbol, and a group under the cap is unmarked.
func TestOverlayRecords_CutMarked(t *testing.T) {
	file := FileID("a/a.go")
	var names []string
	for i := 0; i < MaxOverlayRecords; i++ {
		names = append(names, "f"+strconv.Itoa(i))
	}
	recs := BuildOverlayGroup(ovGroup("s1", "tu", OverlayDeclared, 5, file, "d", names...))
	body := recs[:len(recs)-1]
	if len(body) > MaxOverlayRecords {
		t.Errorf("%d records before the marker, want at most %d", len(body), MaxOverlayRecords)
	}
	if got := recs[len(recs)-1].Node.Attrs["count"]; got != strconv.Itoa(len(body)) {
		t.Errorf("marker count %s, want %d", got, len(body))
	}
	kept := map[ID]bool{}
	for _, r := range body {
		if r.Node != nil {
			kept[r.Node.ID] = true
			if r.Node.Kind == "file" && r.Node.Attrs["_cut"] != "true" {
				t.Errorf("file node of a cut group lacks _cut=true: %v", r.Node.Attrs)
			}
		}
	}
	for _, r := range body {
		if r.Edge != nil && (!kept[r.Edge.Src] || !kept[r.Edge.Dst]) {
			t.Errorf("edge to a dropped symbol: %s -> %s", r.Edge.Src, r.Edge.Dst)
		}
	}
	small := BuildOverlayGroup(ovGroup("s1", "tu", OverlayDeclared, 5, file, "d", "f"))
	if _, has := small[0].Node.Attrs["_cut"]; has {
		t.Errorf("an uncut group carries _cut")
	}
}

// TestOverlayRecords_TornGroupDropped: a group with no marker, a group a
// kill cut at a line boundary before the next group, and one cut mid-line
// are not applied, the groups around them are, and the load does not fail.
func TestOverlayRecords_TornGroupDropped(t *testing.T) {
	file := FileID("a/a.go")
	l, _ := testLog(t, true)
	if err := l.Append(l.Session("s1")); err != nil {
		t.Fatal(err)
	}
	appendGroup(t, l, ovGroup("s1", "t1", OverlayDeclared, 1, file, "d1", "f"))
	// a group cut after its records, before its marker, ending on a line
	// feed: the next group follows it directly.
	torn := BuildOverlayGroup(ovGroup("s1", "t2", OverlayDeclared, 2, file, "d2", "f", "x"))
	if err := l.Append(torn[:len(torn)-1]...); err != nil {
		t.Fatal(err)
	}
	appendGroup(t, l, ovGroup("s1", "t3", OverlayDeclared, 3, file, "d3", "f", "y"))
	// a group cut in the middle of a line, then a healed append
	third := BuildOverlayGroup(ovGroup("s1", "t4", OverlayDeclared, 4, file, "d4", "f"))
	if err := l.Append(third[:2]...); err != nil {
		t.Fatal(err)
	}
	fh, _ := os.OpenFile(l.Path, os.O_APPEND|os.O_WRONLY, 0)
	fh.WriteString(`{"Node":{"ID":"sym:go:a:func:`)
	fh.Close()
	appendGroup(t, l, ovGroup("s1", "t5", OverlayDeclared, 5, file, "d5", "f", "z"))

	s := NewStore()
	groups, skipped := loadGroups(t, l, s)
	var seqs []int64
	for _, g := range groups {
		seqs = append(seqs, g.Seq)
	}
	if !reflect.DeepEqual(seqs, []int64{1, 3, 5}) {
		t.Errorf("complete groups at seqs %v, want [1 3 5]", seqs)
	}
	if skipped != 1 {
		t.Errorf("skipped %d, want the one part line", skipped)
	}
	if got := len(groups[1].Recs); got != 5 {
		t.Errorf("seq 3 group holds %d records, want its own 5 (file, two syms, two edges)", got)
	}
	if n := len(s.Nodes(RevOverlay(ovCommit, 99))); n != 1 {
		t.Errorf("%d nodes asserted by the load, want only the session node", n)
	}
}

// TestOverlayRecords_ReplayScopedToSession: another session's groups, and
// groups at another commit, are not applied.
func TestOverlayRecords_ReplayScopedToSession(t *testing.T) {
	file := FileID("a/a.go")
	mine := ovGroup("s1", "t1", OverlayDeclared, 1, file, "d1", "f", "h1")
	other := ovGroup("s2", "t2", OverlayDeclared, 2, file, "d2", "f", "h2")
	stale := ovGroup("s1", "t3", OverlayDeclared, 3, file, "d3", "f", "h3")
	stale.Commit = "c0"
	s := ovIndex()
	if n := ReplayOverlays(s, []OverlayGroup{mine, other, stale}, "s1", ovCommit); n != 1 {
		t.Errorf("applied %d, want 1", n)
	}
	rev := RevOverlay(ovCommit, 9)
	for id, want := range map[ID]bool{SymID("go", "a", "func", "h1"): true, SymID("go", "a", "func", "h2"): false, SymID("go", "a", "func", "h3"): false} {
		if _, ok := s.Node(id, rev); ok != want {
			t.Errorf("%s live = %v, want %v", id, ok, want)
		}
	}
	if ov := s.Overlays(); len(ov) != 1 || ov[0].Seq != 1 || ov[0].Digest != "d1" {
		t.Errorf("overlays = %+v", ov)
	}
}

// TestOverlayRecords_NeverInIndexFile (15(e)): the group goes to the session
// log under the store root; the code index file is not read or written.
func TestOverlayRecords_NeverInIndexFile(t *testing.T) {
	repo := t.TempDir()
	idx := ovIndex()
	if err := SaveFile(repo, idx, Header{Repo: "r", Rev: HeaderRev{Commit: ovCommit}}); err != nil {
		t.Fatal(err)
	}
	ip := filepath.Join(repo, filepath.FromSlash(GraphFile))
	before, err := os.ReadFile(ip)
	if err != nil {
		t.Fatal(err)
	}
	l, root := testLog(t, true)
	appendGroup(t, l, ovGroup("s1", "t1", OverlayDeclared, 1, FileID("a/a.go"), "d", "f", "n"))
	after, _ := os.ReadFile(ip)
	if !bytes.Equal(before, after) {
		t.Errorf("the code index file changed")
	}
	b, _ := os.ReadFile(l.Path)
	if !strings.Contains(string(b), KindOverlayGroup) || strings.HasSuffix(l.Path, "graph.jsonl") || !strings.HasPrefix(l.Path, root) {
		t.Errorf("group not in the session log under the store root: %s", l.Path)
	}
	if _, err := os.Stat(filepath.Join(root, GraphFile)); err == nil {
		t.Errorf("a code index file appeared under the store root")
	}
}

// TestOverlay_ReplayEqualsLive: append, load, replay gives the same store
// as ApplyOverlay run live: the same nodes and edges at the overlay revision
// (a retraction of the dropped symbol included) and the same overlay record.
func TestOverlay_ReplayEqualsLive(t *testing.T) {
	file := FileID("a/a.go")
	g := ovGroup("s1", "t1", OverlayDeclared, 4, file, ContentDigest([]byte("src")), "f", "n")
	g.Carried = Carried{Declares, Imports} // not every predicate: Calls is unknown to this extractor
	rev := RevOverlay(ovCommit, 4)

	live := ovIndex()
	live.ApplyOverlay(file, []byte("src"), g.Recs, rev, g.Carried)

	l, _ := testLog(t, true)
	appendGroup(t, l, g)
	replayed := ovIndex()
	groups, _ := loadGroups(t, l, replayed)
	if n := ReplayOverlays(replayed, groups, "s1", ovCommit); n != 1 {
		t.Fatalf("applied %d", n)
	}
	if !reflect.DeepEqual(live.Nodes(rev), replayed.Nodes(rev)) || !reflect.DeepEqual(live.Edges(rev), replayed.Edges(rev)) {
		t.Errorf("replayed store differs from the live one:\nlive %v\nreplayed %v", live.Nodes(rev), replayed.Nodes(rev))
	}
	if !reflect.DeepEqual(live.Overlays(), replayed.Overlays()) {
		t.Errorf("overlays differ: %+v vs %+v", live.Overlays(), replayed.Overlays())
	}
	if _, ok := replayed.Node(SymID("go", "a", "func", "g"), rev); ok {
		t.Errorf("g was dropped by the edit and must be retracted at the overlay")
	}
	f := SymID("go", "a", "func", "f")
	if es := replayed.Out(f, rev, Calls); len(es) != 1 || es[0].Dst != SymID("go", "a", "func", "g") {
		t.Errorf("f's call is a predicate the extractor does not carry and must stand at the overlay: %+v", es)
	}
	if got := groups[0].Carried.String(); got != "declares,imports" {
		t.Errorf("recorded carried set = %q", got)
	}
}

// TestApplyOverlayDigest_EqualsApplyOverlay: the digest variant writes what
// ApplyOverlay writes for the same content.
func TestApplyOverlayDigest_EqualsApplyOverlay(t *testing.T) {
	file := FileID("a/a.go")
	recs := ovRecs(file, "x", "f")
	a, b := ovIndex(), ovIndex()
	rev := RevOverlay(ovCommit, 2)
	carried := Carried{Declares, Calls}
	oa := a.ApplyOverlay(file, []byte("src"), recs, rev, carried)
	ob := b.ApplyOverlayDigest(file, ContentDigest([]byte("src")), recs, rev, carried)
	if oa != ob || !reflect.DeepEqual(a.Nodes(rev), b.Nodes(rev)) || !reflect.DeepEqual(a.Edges(rev), b.Edges(rev)) {
		t.Errorf("ApplyOverlayDigest differs: %+v vs %+v", oa, ob)
	}
}

func callRec(l *SessionLog, tu string, exit int) Record {
	return l.Call("s1", tu, CallFields{Tool: "Edit", Path: "a/a.go", Exit: &exit})
}

// TestOverlayRecords_DeclaredDroppedOnFailedCall: a declared group whose
// call carries a non-zero exit is not applied; one with exit 0, and one
// with no call node yet (in flight), are.
func TestOverlayRecords_DeclaredDroppedOnFailedCall(t *testing.T) {
	file := FileID("a/a.go")
	l, _ := testLog(t, true)
	s := ovIndex()
	s.AssertNode(*callRec(l, "bad", 1).Node)
	s.AssertNode(*callRec(l, "ok", 0).Node)
	groups := []OverlayGroup{
		ovGroup("s1", "bad", OverlayDeclared, 1, file, "d1", "f", "failed"),
		ovGroup("s1", "ok", OverlayDeclared, 2, file, "d2", "f", "fine"),
		ovGroup("s1", "inflight", OverlayDeclared, 3, file, "d3", "f", "fine", "pending"),
	}
	if n := ReplayOverlays(s, groups, "s1", ovCommit); n != 2 {
		t.Errorf("applied %d, want 2", n)
	}
	rev := RevOverlay(ovCommit, 9)
	for name, want := range map[string]bool{"failed": false, "fine": true, "pending": true} {
		if _, ok := s.Node(SymID("go", "a", "func", name), rev); ok != want {
			t.Errorf("%s live = %v, want %v", name, ok, want)
		}
	}
}

// TestOverlayRecords_LandedReplacesDeclared: the landed group of the same
// call replaces the declared one by seq: what the declared content added
// and the landed content did not is gone at the landed seq.
func TestOverlayRecords_LandedReplacesDeclared(t *testing.T) {
	file := FileID("a/a.go")
	s := ovIndex()
	groups := []OverlayGroup{
		ovGroup("s1", "t1", OverlayDeclared, 1, file, "d1", "f", "wanted"),
		ovGroup("s1", "t1", OverlayLanded, 5, file, "d2", "f", "actual"),
	}
	if n := ReplayOverlays(s, groups, "s1", ovCommit); n != 1 {
		t.Errorf("applied %d, want only the landed group", n)
	}
	rev := RevOverlay(ovCommit, 9)
	if _, ok := s.Node(SymID("go", "a", "func", "wanted"), rev); ok {
		t.Errorf("the declared-only symbol is live after the landed overlay")
	}
	if _, ok := s.Node(SymID("go", "a", "func", "actual"), rev); !ok {
		t.Errorf("the landed symbol is absent")
	}
}

// TestOverlayRecords_LandedMarkerOnlyPromotes: a landed group of just a
// marker (the landed digest was the declared one) applies the declared
// group's records at the landed seq.
func TestOverlayRecords_LandedMarkerOnlyPromotes(t *testing.T) {
	file := FileID("a/a.go")
	l, _ := testLog(t, true)
	appendGroup(t, l, ovGroup("s1", "t1", OverlayDeclared, 1, file, "d1", "f", "wanted"))
	appendGroup(t, l, OverlayGroup{Session: "s1", ToolUseID: "t1", Kind: OverlayLanded, File: file, Digest: "d1", Commit: ovCommit, Seq: 6, OfSeq: 1})
	s := ovIndex()
	groups, _ := loadGroups(t, l, s)
	if len(groups) != 2 || len(groups[1].Recs) != 0 {
		t.Fatalf("groups = %+v", groups)
	}
	if n := ReplayOverlays(s, groups, "s1", ovCommit); n != 1 {
		t.Errorf("applied %d, want 1", n)
	}
	if _, ok := s.Node(SymID("go", "a", "func", "wanted"), RevOverlay(ovCommit, 5)); ok {
		t.Errorf("the symbol is live before the landed seq")
	}
	if _, ok := s.Node(SymID("go", "a", "func", "wanted"), RevOverlay(ovCommit, 6)); !ok {
		t.Errorf("the promoted symbol is absent at the landed seq")
	}
}

// TestOverlayLog_BoundNeverReadsFile: the reserve callback sees the encoded
// size before the file exists, a refusal writes nothing, and no consent
// calls reserve not at all.
func TestOverlayLog_BoundNeverReadsFile(t *testing.T) {
	l, root := testLog(t, true)
	recs := BuildOverlayGroup(ovGroup("s1", "t1", OverlayDeclared, 1, FileID("a/a.go"), "d", "f"))
	var got int64
	full := func(n int64) error {
		got = n
		if _, err := os.Stat(l.Path); err == nil {
			t.Errorf("the log existed when reserve ran")
		}
		return ErrOverlayFull
	}
	if err := l.AppendOverlayGroup(recs, full); !errors.Is(err, ErrOverlayFull) {
		t.Fatalf("err = %v", err)
	}
	if got < 100 {
		t.Errorf("reserve saw %d bytes", got)
	}
	if ents, _ := os.ReadDir(root); len(ents) != 0 {
		t.Errorf("a refused group created %d entries", len(ents))
	}
	var c OverlayCounter
	if err := l.AppendOverlayGroup(recs, c.Reserve); err != nil || c.Used != got {
		t.Errorf("counter used %d err %v, want %d", c.Used, err, got)
	}
	off, _ := testLog(t, false)
	called := false
	if err := off.AppendOverlayGroup(recs, func(int64) error { called = true; return nil }); !errors.Is(err, ErrNoSessionConsent) || called {
		t.Errorf("no consent: err %v, reserve called %v", err, called)
	}
}

// TestOverlayLog_CounterArithmetic: the bound is on the sum, a refused
// reserve changes nothing, and exactly the max is allowed.
func TestOverlayLog_CounterArithmetic(t *testing.T) {
	c := OverlayCounter{Used: MaxOverlayBytes - 10}
	if err := c.Reserve(11); !errors.Is(err, ErrOverlayFull) || c.Used != MaxOverlayBytes-10 {
		t.Errorf("over the max: err %v used %d", err, c.Used)
	}
	if err := c.Reserve(10); err != nil || c.Used != MaxOverlayBytes {
		t.Errorf("exactly the max: err %v used %d", err, c.Used)
	}
	if MaxOverlayBytes != 4<<20 {
		t.Errorf("bound = %d, want 4 MiB", MaxOverlayBytes)
	}
}

// TestSessionLog_TornFragmentThenHealedLoadsWithCountOne: a part line, then
// a healed append, loads the healed record and counts one skipped line.
func TestSessionLog_TornFragmentThenHealedLoadsWithCountOne(t *testing.T) {
	l, _ := testLog(t, true)
	if err := l.Append(l.Session("s1")); err != nil {
		t.Fatal(err)
	}
	fh, _ := os.OpenFile(l.Path, os.O_APPEND|os.O_WRONLY, 0)
	fh.WriteString(`{"Node":{"ID":"call:s1:t`)
	fh.Close()
	if err := l.Append(l.Prompt("s1", 1, "p")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(l.Path)
	s := NewStore()
	_, skipped, err := LoadSessionLog(s, bytes.NewReader(b))
	if err != nil || skipped != 1 {
		t.Fatalf("skipped %d err %v, want 1 and nil", skipped, err)
	}
	if _, ok := s.Node(PromptNodeID("s1", 1), RevCommit("c1")); !ok {
		t.Errorf("the healed record did not load")
	}
}

// TestOverlayLog_LoaderDropsAnOldLinesSigAndProv: a group an earlier build
// wrote, its records with their Prov, its symbols with a sig and its marker
// with a Prov, loads as the same group written now: no record keeps either.
// BuildOverlayGroup keeps neither and leaves the group it is given as it
// was. Witness: the old file holds the sig and the Prov.
func TestOverlayLog_LoaderDropsAnOldLinesSigAndProv(t *testing.T) {
	prov := Prov{Extractor: "go/parser", Version: "v1"}
	g := ovGroup("s1", "toolu_old", OverlayLanded, 1, FileID("a/a.go"), "new", "f", "h")
	for _, r := range g.Recs {
		if r.Node != nil {
			r.Node.Prov = prov
			if r.Node.Kind == "sym" {
				r.Node.Attrs["sig"] = "func ( oldsigword int )"
			}
		}
		if r.Edge != nil {
			r.Edge.Prov = prov
		}
	}
	built := BuildOverlayGroup(g)
	sigs := 0
	for _, r := range g.Recs {
		if r.Node != nil && r.Node.Attrs["sig"] != "" && reflect.DeepEqual(r.Node.Prov, prov) {
			sigs++
		}
	}
	if sigs != 2 {
		t.Errorf("the group built from keeps %d symbols with a sig and a Prov, want 2", sigs)
	}
	for _, r := range built {
		if r.Node != nil && (r.Node.Attrs["sig"] != "" || !reflect.DeepEqual(r.Node.Prov, Prov{})) {
			t.Errorf("built node %s keeps attrs %v and prov %+v", r.Node.ID, r.Node.Attrs, r.Node.Prov)
		}
		if r.Edge != nil && !reflect.DeepEqual(r.Edge.Prov, Prov{}) {
			t.Errorf("built edge %s %s %s keeps prov %+v", r.Edge.Src, r.Edge.Pred, r.Edge.Dst, r.Edge.Prov)
		}
	}

	marker := *built[len(built)-1].Node
	marker.Prov = Prov{Extractor: "session", Version: "v1"}
	old := OpenSessionLog(t.TempDir(), "rid", RevCommit(ovCommit), true)
	if err := old.Append(append(append([]Record(nil), g.Recs...), Record{Node: &marker})...); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(old.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"oldsigword", "go/parser", `"Extractor":"session"`} {
		if !bytes.Contains(b, []byte(w)) {
			t.Errorf("witness: the old log holds no %q", w)
		}
	}
	now := OpenSessionLog(t.TempDir(), "rid", RevCommit(ovCommit), true)
	appendGroup(t, now, g)
	oldGroups, oskip := loadGroups(t, old, NewStore())
	nowGroups, nskip := loadGroups(t, now, NewStore())
	if len(oldGroups) != 1 || oskip != 0 || nskip != 0 {
		t.Fatalf("old log: %d groups, %d skipped; new log: %d skipped", len(oldGroups), oskip, nskip)
	}
	if !reflect.DeepEqual(oldGroups, nowGroups) {
		t.Errorf("the old line loads as %+v, the new one as %+v", oldGroups, nowGroups)
	}
	if len(oldGroups[0].Recs) != len(g.Recs) {
		t.Errorf("the old group loads %d records, want %d", len(oldGroups[0].Recs), len(g.Recs))
	}
	for _, r := range oldGroups[0].Recs {
		if r.Node != nil && (r.Node.Attrs["sig"] != "" || !reflect.DeepEqual(r.Node.Prov, Prov{})) {
			t.Errorf("loaded node %s keeps attrs %v and prov %+v", r.Node.ID, r.Node.Attrs, r.Node.Prov)
		}
		if r.Edge != nil && !reflect.DeepEqual(r.Edge.Prov, Prov{}) {
			t.Errorf("loaded edge %s %s %s keeps prov %+v", r.Edge.Src, r.Edge.Pred, r.Edge.Dst, r.Edge.Prov)
		}
	}
}

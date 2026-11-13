package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

var ubRev = graph.RevCommit("50942026abcd")

// ubBrief is a unit brief as UnitBrief returns one: a consumer line cut at
// its name cap, so its slot carries counts, a decision and a lesson.
func ubBrief(t testing.TB) engine.Brief {
	t.Helper()
	b := engine.Rebind(ubRev, []engine.Line{
		{Key: "file:brief/brief.py", Slot: slot.NewCodeLine(slot.CodeFact{Path: "brief/brief.py", Role: slot.RoleConsumer,
			Names: []slot.Ident{"b1", "b2", "b3", "b4", "b5"}})},
		{Key: "dec:D1", Slot: slot.NewKnowledgeLine(slot.KnowledgeFact{Kind: slot.KnowledgeDecision, ID: "dec:D1",
			Path: "store/loader.py", Clause: "rows load in key order"})},
		{Key: "les:L1", Slot: slot.NewKnowledgeLine(slot.KnowledgeFact{Kind: slot.KnowledgeLesson, ID: "les:L1",
			Title: "a reload dropped rows", Source: "incidents/L1.md"})},
	})
	if b.Empty() || b.Kept != 3 {
		t.Fatalf("fixture brief kept %d lines", b.Kept)
	}
	return b
}

// unitBriefOf is data read as the post reads a file at the index's
// commit: scanned, then its lines decoded and rebound.
func unitBriefOf(data []byte) (storedUnitBrief, error) {
	sc, err := scanUnitBrief(data)
	if err != nil {
		return storedUnitBrief{}, err
	}
	return sc.brief()
}

// TestUnitBrief_FileRoundTripsTheBrief (W-M2.3 hook half, D2): a unit
// brief encoded to its file and decoded renders the same message and the
// same record form, keeps its lines' keys in order, and carries the unit,
// first row, tier and prompt id it was written with. Witness: the
// consumer line's counts survive, so its "and 2 more" tail is in both
// messages. Break: drop More from the line's field set, and the tail goes.
func TestUnitBrief_FileRoundTripsTheBrief(t *testing.T) {
	digest := func(s string) string { return strings.Repeat("0a", 32) }
	b := ubBrief(t)
	data, err := encodeUnitBrief(b, "g:r1#unit:py:store", "g:r1#file:brief/brief.py", graph.Algorithmic, "prompt-7")
	if err != nil {
		t.Fatal(err)
	}
	got, err := unitBriefOf(data)
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, data)
	}
	if got.Message.Text() != b.Message.Text() {
		t.Errorf("message\n got %q\nwant %q", got.Message.Text(), b.Message.Text())
	}
	if !strings.Contains(got.Message.Text(), "and 2 more") {
		t.Errorf("witness: the consumer line's counts did not survive: %q", got.Message.Text())
	}
	gr, err1 := got.Record(digest)
	wr, err2 := b.Record(digest)
	if err1 != nil || err2 != nil || gr.Text() != wr.Text() {
		t.Errorf("record\n got %q (%v)\nwant %q (%v)", gr.Text(), err1, wr.Text(), err2)
	}
	var keys []graph.ID
	for _, l := range got.Lines[:got.Kept] {
		keys = append(keys, l.Key)
	}
	if want := []graph.ID{"file:brief/brief.py", "dec:D1", "les:L1"}; !slices.Equal(keys, want) {
		t.Errorf("keys %v, want %v", keys, want)
	}
	if got.Unit != "g:r1#unit:py:store" || got.First != "g:r1#file:brief/brief.py" || got.Tier != graph.Algorithmic || got.PromptID != "prompt-7" {
		t.Errorf("header = %q %q %q %q", got.Unit, got.First, got.Tier, got.PromptID)
	}
	if got.Prompt != "" || len(got.Tokens) != 0 {
		t.Errorf("a read brief holds prompt words: %q %v", got.Prompt, got.Tokens)
	}
}

// TestUnitBrief_EncodeRefusesWhatItCannotRebuild (D2): the writer stores
// only a brief Rebind renders back as it was, under a header that names
// it, so a file it writes is never one the reader refuses. Each case is an
// error and no bytes. Break: drop the seeds check, and the prompt brief's
// case encodes.
func TestUnitBrief_EncodeRefusesWhatItCannotRebuild(t *testing.T) {
	const unit, first = "g:r1#unit:py:store", "g:r1#file:brief/brief.py"
	seeded := ubBrief(t)
	seeded.Seeds = []engine.Seed{{}}
	wide := ubBrief(t)
	wide.Neighborhood = 4
	overlay := engine.Rebind(graph.RevOverlay("50942026abcd", 3), ubBrief(t).Lines)
	if overlay.Empty() {
		t.Fatal("fixture: the overlay brief kept no line")
	}
	for name, c := range map[string]struct {
		b           engine.Brief
		unit, first graph.ID
		tier        graph.Tier
	}{
		"an empty brief":                   {engine.Brief{}, unit, first, graph.Algorithmic},
		"an overlay revision":              {overlay, unit, first, graph.Algorithmic},
		"a prompt brief's seeds":           {seeded, unit, first, graph.Algorithmic},
		"a prompt brief's neighborhood":    {wide, unit, first, graph.Algorithmic},
		"a local unit id":                  {ubBrief(t), "unit:py:store", first, graph.Algorithmic},
		"a first row of another repo":      {ubBrief(t), unit, "g:r2#file:brief/brief.py", graph.Algorithmic},
		"a first row that is not line one": {ubBrief(t), unit, "g:r1#dec:D1", graph.Algorithmic},
		"an unknown tier":                  {ubBrief(t), unit, first, graph.Tier("guessed")},
	} {
		if data, err := encodeUnitBrief(c.b, c.unit, c.first, c.tier, "p"); err == nil {
			t.Errorf("%s: encoded %d B", name, len(data))
		}
	}
}

// TestUnitBrief_WriteIsWholeAndPrivate (D2): the writer makes the session's
// directory 0700, the file 0600, replaces an earlier file whole, and
// leaves no temporary file beside it. Break: make the directory 0755, and
// it fails.
func TestUnitBrief_WriteIsWholeAndPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), graph.UnitBriefSubdir, "sess")
	path := filepath.Join(dir, "x-"+strings.Repeat("0a", 16)+".json")
	for _, data := range [][]byte{[]byte(`{"first":1}`), []byte(`{"second":2}`)} {
		if err := writeUnitBrief(path, data); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("read back %q (%v), want %q", got, err, data)
		}
	}
	for p, want := range map[string]os.FileMode{dir: 0o700, filepath.Dir(dir): 0o700, path: 0o600} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v (%v), want %v", p, fi.Mode().Perm(), err, want)
		}
	}
	if ents, err := os.ReadDir(dir); err != nil || len(ents) != 1 {
		t.Errorf("the directory holds %d entries (%v), want the one file", len(ents), err)
	}
}

// TestUnitBrief_FileShapeIsClosed (D2, R5): the file's keys are a fixed
// set, so a renamed slot field turns this red rather than changing the
// file; a file of another version, with a field the reader does not know,
// with two facts on one line, or with anything after its one object is a
// miss, never a wrong line.
func TestUnitBrief_FileShapeIsClosed(t *testing.T) {
	data, err := encodeUnitBrief(ubBrief(t), "g:r1#unit:py:store", "g:r1#file:brief/brief.py", graph.Algorithmic, "p")
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	if got, want := sortedKeys(top), []string{"first", "lines", "prompt", "rev", "tier", "unit", "v"}; !slices.Equal(got, want) {
		t.Errorf("file keys %v, want %v", got, want)
	}
	var lines []map[string]json.RawMessage
	if err := json.Unmarshal(top["lines"], &lines); err != nil {
		t.Fatal(err)
	}
	if got, want := sortedKeys(lines[0]), []string{"code", "key", "more"}; !slices.Equal(got, want) {
		t.Errorf("line keys %v, want %v", got, want)
	}
	var code, more map[string]json.RawMessage
	_ = json.Unmarshal(lines[0]["code"], &code)
	_ = json.Unmarshal(lines[0]["more"], &more)
	if got, want := sortedKeys(code), []string{"Band", "Basis", "Calls", "Consts", "Imports", "Lessons", "Names", "Path", "Reads", "Role", "Writes"}; !slices.Equal(got, want) {
		t.Errorf("code fact keys %v, want %v", got, want)
	}
	if got, want := sortedKeys(more), []string{"Calls", "Consts", "Consumers", "Imports", "Lessons", "Names", "Readers", "Reads", "Records", "Writers", "Writes"}; !slices.Equal(got, want) {
		t.Errorf("count keys %v, want %v", got, want)
	}
	for name, bad := range map[string][]byte{
		"another version":          bytes.Replace(data, []byte(`"v":1`), []byte(`"v":2`), 1),
		"an unknown field":         bytes.Replace(data, []byte(`"v":1`), []byte(`"v":1,"extra":0`), 1),
		"an unknown fact field":    bytes.Replace(data, []byte(`"code":{`), []byte(`"code":{"Extra":0,`), 1),
		"two facts":                bytes.Replace(data, []byte(`"code":{`), []byte(`"table":{"Table":"t"},"code":{`), 1),
		"a line with no fact":      withoutLineField(t, data, "code"),
		"a trailing value":         append(slices.Clone(data), []byte(` {}`)...),
		"no lines":                 bytes.Replace(data, top["lines"], []byte(`[]`), 1),
		"a line bind refuses":      bytes.Replace(data, []byte(`"Path":"brief/brief.py"`), []byte(`"Path":""`), 1),
		"a name bind leaves out":   bytes.Replace(data, []byte(`"b1"`), []byte(`"1b"`), 1),
		"a clause bind blanks":     bytes.Replace(data, []byte(`"Clause":"rows load in key order"`), []byte(`"Clause":"`+strings.Repeat("a", 161)+`"`), 1),
		"an unknown tier":          bytes.Replace(data, []byte(`"tier":"algorithmic"`), []byte(`"tier":"guessed"`), 1),
		"a first row not line one": bytes.Replace(data, []byte(`"first":"g:r1#file:brief/brief.py"`), []byte(`"first":"g:r1#dec:D1"`), 1),
	} {
		if bytes.Equal(bad, data) {
			t.Fatalf("%s: the case did not change the file", name)
		}
		if _, err := unitBriefOf(bad); err == nil {
			t.Errorf("%s: decoded", name)
		}
	}
}

// withoutLineField is data with field taken out of its first line, so the
// line keeps its key and counts and the decoder sees no unknown field.
func withoutLineField(t *testing.T, data []byte, field string) []byte {
	t.Helper()
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	delete(top["lines"].([]any)[0].(map[string]any), field)
	out, err := json.Marshal(top)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sortedKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestUnitBrief_ReadIsBounded (R, D3): the reader takes at most
// maxUnitBriefBytes+1 bytes; a file over the cap and a torn file are
// misses (an error, no panic), and a file at the cap's size of a valid
// brief reads.
func TestUnitBrief_ReadIsBounded(t *testing.T) {
	dir := t.TempDir()
	data, err := encodeUnitBrief(ubBrief(t), "g:r1#unit:py:store", "g:r1#file:brief/brief.py", graph.Algorithmic, "p")
	if err != nil {
		t.Fatal(err)
	}
	ok := filepath.Join(dir, "ok.json")
	if err := os.WriteFile(ok, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if sc, _, err := readUnitBrief(ok); err != nil {
		t.Errorf("a valid file: %v", err)
	} else if _, err := sc.brief(); err != nil {
		t.Errorf("a valid file's lines: %v", err)
	}
	over := filepath.Join(dir, "over.json")
	padded := append(slices.Clone(data), bytes.Repeat([]byte(" "), maxUnitBriefBytes+1-len(data))...)
	if err := os.WriteFile(over, padded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readUnitBrief(over); err == nil {
		t.Error("a file over the cap was read")
	}
	torn := filepath.Join(dir, "torn.json")
	if err := os.WriteFile(torn, data[:len(data)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readUnitBrief(torn); err == nil {
		t.Error("a torn file was read")
	}
	if _, _, err := readUnitBrief(filepath.Join(dir, "none.json")); err == nil {
		t.Error("a missing file was read")
	}
}

// TestUnitBrief_MaxShapedBriefIsUnderTheCap (D3): the widest briefs
// deliver keeps encode under maxUnitBriefBytes, so the writer's refusal
// over the cap never drops a brief the model could have been given. Every
// string the file holds is in a slot charset that JSON writes byte for byte
// (idents, paths, knowledge ids, approved text: no quote, backslash, <, >
// or &), so a file is its rendered text plus a fixed cost per line, and the
// widest file is five lines each as wide as five still render. Per kind,
// the test widens every item until deliver keeps fewer than five lines and
// encodes the widest five-line brief; a kind with no five-line brief is a
// failure, not a skip. The sizes are logged for the PR line. Break: lower
// the cap to 2 KiB, and it fails.
func TestUnitBrief_MaxShapedBriefIsUnderTheCap(t *testing.T) {
	rid := strings.Repeat("r", 64)
	unit := graph.GlobalID(rid, graph.ID("unit:py:"+strings.Repeat("u/", 60)))
	for name, shape := range maxBriefShapes() {
		widest, width := widestBrief(shape)
		if widest.Kept != 5 {
			t.Fatalf("%s: deliver keeps no five-line brief", name)
		}
		data, err := encodeUnitBrief(widest, unit, graph.GlobalID(rid, widest.FirstKey()), graph.Algorithmic, strings.Repeat("p", 64))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Logf("%s: widest five-line brief at width %d, message %d B, file %d B of %d", name, width, len(widest.Message.Text()), len(data), maxUnitBriefBytes)
		if len(data) > maxUnitBriefBytes {
			t.Errorf("%s: a kept brief encodes to %d B, over the cap %d", name, len(data), maxUnitBriefBytes)
		}
	}
}

// maxBriefShapes is the widest briefs' line builders by kind
// (TestUnitBrief_MaxShapedBriefIsUnderTheCap): each gives, for a width w,
// five lines whose every item is w wide up to its slot's cap.
func maxBriefShapes() map[string]func(int) []engine.Line {
	word := func(c byte, w, max int) string { return string(c) + strings.Repeat("n", min(w, max)-1) }
	id := func(i, w int) slot.KnowledgeID {
		return slot.KnowledgeID("les:" + word(byte('A'+i), w, 60))
	}
	path := func(w int) slot.RelPath { return slot.RelPath(word('p', w, 120) + "/f.py") }
	know := func(w int) []engine.Line {
		var lines []engine.Line
		for i := 0; i < 5; i++ {
			text := slot.Approved(word('a', w, 160))
			lines = append(lines, engine.Line{Key: graph.ID(id(i, w)), Slot: slot.NewKnowledgeLine(slot.KnowledgeFact{
				Kind: slot.KnowledgeLesson, ID: id(i, w), Path: path(w), Title: text, Clause: text,
				Source: slot.Path(word('s', w, 120) + ".md")})})
		}
		return lines
	}
	code := func(w int) []engine.Line {
		var names []slot.Ident
		for j := 0; j < 5; j++ {
			names = append(names, slot.Ident(word(byte('a'+j), w, 64)))
		}
		var lines []engine.Line
		for i := 0; i < 5; i++ {
			lines = append(lines, engine.Line{Key: graph.ID("file:" + string(rune('a'+i))), Slot: slot.NewCodeLine(slot.CodeFact{
				Path: path(w), Names: names, Calls: names, Imports: names,
				Lessons: []slot.KnowledgeID{id(0, w), id(1, w), id(2, w)},
				Consts:  []slot.ConstFact{{Name: names[0], Value: 1 << 40}, {Name: names[1], Value: 2}, {Name: names[2], Value: 3}},
				Writes:  []slot.TableRef{{Table: names[0], Cols: names}, {Table: names[1], Cols: names}, {Table: names[2]}},
				Reads:   []slot.TableRef{{Table: names[3], Cols: names}, {Table: names[4], Cols: names}, {Table: names[0]}}})})
		}
		return lines
	}
	return map[string]func(int) []engine.Line{"knowledge": know, "code": code}
}

// widestBrief is shape's brief at the widest width deliver keeps all five
// lines at, bound at ubRev, and that width; Kept is under five where no
// width does.
func widestBrief(shape func(int) []engine.Line) (engine.Brief, int) {
	var widest engine.Brief
	width := 0
	for w := 1; w <= 160; w++ {
		b := engine.Rebind(ubRev, shape(w))
		if b.Kept < 5 {
			break
		}
		widest, width = b, w
	}
	return widest, width
}

// TestBriefUnits_EachUnitOnceInLineOrderAtMostEight (D1): the units a
// prompt stores briefs for are the units of its brief's keyed lines, each
// once, in line order, at most maxUnitBriefs; a keyless line (the
// not-known line) names none. Break: drop the dedup, and unit:a is listed
// twice.
func TestBriefUnits_EachUnitOnceInLineOrderAtMostEight(t *testing.T) {
	var lines []engine.Line
	for _, k := range []graph.ID{"unit:a", "unit:b", "unit:a", "", "unit:c", "unit:d", "unit:e", "unit:f", "unit:g", "unit:h", "unit:i", "unit:j"} {
		lines = append(lines, engine.Line{Key: k})
	}
	got := briefUnits(graph.NewStore(), ubRev, engine.Brief{Lines: lines})
	want := []graph.ID{"unit:a", "unit:b", "unit:c", "unit:d", "unit:e", "unit:f", "unit:g", "unit:h"}
	if !slices.Equal(got, want) {
		t.Errorf("units %v, want %v", got, want)
	}
}

// TestGraphBrief_StoresEachUnitsBriefWhenConsented (D1, D2, D3): with the
// session-content marker set, the prompt hook stores the brief UnitBrief
// gives each unit of its brief's keyed lines, under the session's
// unit-brief directory by the unit's file name, and no other file; each
// reads back to that brief, names its unit and first row by global id and
// the prompt id, and holds no word of the prompt. Without the marker the
// prompt makes no unit-brief directory. Witness: the fixture's brief stores
// at least one. Break: drop the marker check, and the unconsented store
// has the directory.
func TestGraphBrief_StoresEachUnitsBriefWhenConsented(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	cwd := filepath.Join(dir, "services", "auth")
	const prompt = "make session expiry in sessions.py respect the org-level timeout override."
	consented, unconsented := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(consented, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gb, ok := graphBrief(consented, cwd, "s1", "p1", []byte(prompt))
	if !ok {
		t.Fatal("no brief")
	}
	if _, ok := graphBrief(unconsented, cwd, "s1", "p1", []byte(prompt)); !ok {
		t.Fatal("no brief")
	}
	if _, err := os.Stat(filepath.Join(unconsented, graph.StoreDir, graph.UnitBriefSubdir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("an unconsented prompt made the unit-brief directory (%v)", err)
	}
	root := graph.FindIndex(cwd)
	s, h, err := graph.LoadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	rev := graph.RevCommit(h.Rev.Commit)
	opt := hookScorer()
	opt.Rel = relTo(cwd, root)
	sessDir := graph.UnitBriefDir(consented, "s1")
	want := map[string]graph.ID{}
	var keys []graph.ID
	for _, l := range gb.Lines {
		keys = append(keys, l.Key)
	}
	t.Logf("prompt brief: %d lines %v, kept %d", len(gb.Lines), keys, gb.Kept)
	for _, u := range briefUnits(s, rev, gb.Brief) {
		ub := engine.UnitBrief(s, rev, u, gb.Brief, opt)
		t.Logf("%s: unit brief %d lines, kept %d, decisions %v", u, len(ub.Lines), ub.Kept, ub.Decisions)
		if ub.Empty() {
			continue
		}
		name := graph.UnitBriefFileName(root, u)
		want[name] = u
		path := filepath.Join(sessDir, name)
		sc, _, err := readUnitBrief(path)
		if err != nil {
			t.Errorf("%s: %v", u, err)
			continue
		}
		got, err := sc.brief()
		if err != nil {
			t.Errorf("%s: lines: %v", u, err)
			continue
		}
		if got.Message.Text() != ub.Message.Text() {
			t.Errorf("%s: stored\n%s\nwant\n%s", u, got.Message.Text(), ub.Message.Text())
		}
		if got.Unit != graph.GlobalID(h.Repo, u) || got.First != graph.GlobalID(h.Repo, ub.FirstKey()) || got.PromptID != "p1" {
			t.Errorf("%s: header %q %q %q", u, got.Unit, got.First, got.PromptID)
		}
		if raw, _ := os.ReadFile(path); bytes.Contains(raw, []byte("respect")) {
			t.Errorf("%s: the file holds a word of the prompt", u)
		}
		t.Logf("%s: %d lines, first %s", u, got.Kept, got.First)
	}
	if len(want) == 0 {
		t.Fatal("witness: no unit of the brief has a unit brief, so this tests nothing")
	}
	ents, err := os.ReadDir(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
		if _, ok := want[e.Name()]; !ok {
			t.Errorf("the session directory holds %s, no unit's brief", e.Name())
		}
	}
	if len(names) != len(want) {
		t.Errorf("the session directory holds %v, want the %d unit briefs", names, len(want))
	}
}

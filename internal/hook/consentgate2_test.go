package hook

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

const (
	cg2PyRel   = "inv/a.py"
	cg2PyOrig  = "def run():\n    return 1\n"
	cg2PyAdd   = "\n\ndef probe(a, b=2):\n    return run()\n\n\nCG2_PROBE = \"brindle\"\n"
	cg2PyProbe = "CG2_PROBE"
	cg2PyValue = "brindle"
)

// The ids of the edited file's symbols, written out from the id form
// sym:<lang>:<unit>:<symkind>:<module>.<name> (graph.SymID) and the module
// inv.a, the file's path under the unit inv with no suffix.
const (
	cg2RunID   = "sym:py:inv:func:inv.a.run"
	cg2ProbeID = "sym:py:inv:func:inv.a.probe"
	cg2ConstID = "sym:py:inv:const:inv.a.CG2_PROBE"
)

// cg2PyFix is newOvFix with a configuration declaring the py unit inv, an
// index header naming this build's py producer, so the py reader is indexed
// and configured there, and inv/a.py on disk; it returns the file's
// absolute path.
func cg2PyFix(t *testing.T) (*ovFix, string) {
	t.Helper()
	f := newOvFix(t, "1\n")
	cfg := filepath.Join(f.repo, filepath.FromSlash(index.ConfigFile))
	if err := os.WriteFile(cfg, []byte(`{"units": [{"path": "inv", "modules": ["inv"], "lang": "py"}]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hdr := graph.Header{Repo: "r", Rev: graph.HeaderRev{Commit: ovCommit},
		Producers: map[string]string{pylite.Lang: pylite.Resolver{}.Producer()}}
	if err := graph.SaveFile(f.repo, graph.NewStore(), hdr); err != nil {
		t.Fatal(err)
	}
	py := filepath.Join(f.repo, filepath.FromSlash(cg2PyRel))
	if err := os.MkdirAll(filepath.Dir(py), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(py, []byte(cg2PyOrig), 0o644); err != nil {
		t.Fatal(err)
	}
	return f, py
}

// pyEdit is pyEditTo with the edit that adds the def probe, which calls
// run, and the constant CG2_PROBE.
func (f *ovFix) pyEdit(id, py string) *Post {
	f.t.Helper()
	return f.pyEditTo(id, py, cg2PyOrig+cg2PyAdd)
}

// pyEditTo is an Edit of py from cg2PyOrig to next, as the hook runs it:
// the PreToolUse, the file as the edit leaves it, the PostToolUse. It
// returns the post, closed.
func (f *ovFix) pyEditTo(id, py, next string) *Post {
	f.t.Helper()
	in := map[string]any{"file_path": py, "old_string": cg2PyOrig, "new_string": next}
	f.pre(id, "Edit", in)
	if err := os.WriteFile(py, []byte(next), 0o644); err != nil {
		f.t.Fatal(err)
	}
	p := NewPost(f.st, time.Now)
	p.Close(&Signals{}, p.Capture(strings.NewReader(f.payload("PostToolUse", id, "Edit", in))))
	return p
}

// cg2GoEdit is the fixture's Edit of its .go file, Old to New, as the hook
// runs it.
func (f *ovFix) cg2GoEdit(id string) {
	f.t.Helper()
	in := f.edit("func Old() {}", "func New() {}")
	f.pre(id, "Edit", in)
	if err := os.WriteFile(f.file, []byte("package pkg\n\nfunc New() {}\n"), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.post(id, "Edit", in)
}

// cg2Groups is the kinds of the log's groups of the .py file, sorted, and
// the number of its groups of the .go file.
func (f *ovFix) cg2Groups() (py []string, goGroups int) {
	f.t.Helper()
	groups, _, _ := f.read()
	for _, g := range groups {
		switch g.File {
		case graph.FileID(cg2PyRel):
			py = append(py, g.Kind)
		case graph.FileID(ovRel):
			goGroups++
		}
	}
	slices.Sort(py)
	return py, goGroups
}

// cg2Asked is what a stub of ConsentScopeOnFile was asked: the root of each
// call, in order.
type cg2Asked struct{ roots []string }

// cg2Scope sets ConsentScopeOnFile, for the test, to a stub that stands for
// consent.json, answering scope (the highest scope_version on file) and err,
// and returns what it is asked.
func cg2Scope(t *testing.T, scope int, err error) *cg2Asked {
	t.Helper()
	old := ConsentScopeOnFile
	asked := &cg2Asked{}
	ConsentScopeOnFile = func(root string) (int, error) {
		asked.roots = append(asked.roots, root)
		return scope, err
	}
	t.Cleanup(func() { ConsentScopeOnFile = old })
	return asked
}

// cg2NoScope leaves ConsentScopeOnFile unset for the test, as in a probe
// built without it.
func cg2NoScope(t *testing.T) {
	t.Helper()
	old := ConsentScopeOnFile
	ConsentScopeOnFile = nil
	t.Cleanup(func() { ConsentScopeOnFile = old })
}

// cg2Lines is every record of the session graph log as written, after its
// header line, the group markers included, in the order the log holds them.
func (f *ovFix) cg2Lines() []graph.Record {
	f.t.Helper()
	b, err := os.ReadFile(f.logPath())
	if err != nil {
		f.t.Fatal(err)
	}
	var out []graph.Record
	for i, line := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
		if i == 0 {
			continue
		}
		var r graph.Record
		if err := json.Unmarshal(line, &r); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// cg2Raw is cg2Lines with the group markers left out.
func (f *ovFix) cg2Raw() []graph.Record {
	f.t.Helper()
	var out []graph.Record
	for _, r := range f.cg2Lines() {
		if r.Node != nil && r.Node.Kind == graph.KindOverlayGroup {
			continue
		}
		out = append(out, r)
	}
	return out
}

// cg2PyKept is, for each kind of record a .py group keeps, the attribute
// names the session graph log holds of it under a consent of scope 9 in
// this release, each named by the KEPT FROM YOUR SESSIONS sentence of that
// screen: a file node's module and SHA-256 ("with the path and module of
// the file in the repository, its SHA-256"; the path is its id), a
// declaration's kind and line ("with its kind, its line"), a call's line
// ("with the line and number of arguments of each call"). The sentence also
// names a declaration's parameter count and a call's argument count; this
// release keeps neither (ruling x371 item 5), so a later card adds them
// under the same screen. Every other kind of record keeps none: a declares,
// imports or references edge is its two names alone. The list is closed
// and per kind of record, so an attribute a later version of the reader
// writes, or a name kept for one kind written on another, is left out of
// the log until the text names it there.
var cg2PyKept = map[string][]string{
	"node file":  {"digest", "module"},
	"node sym":   {"line", "symkind"},
	"edge calls": {"line"},
}

// cg2Rec is r's kind of record as cg2PyKept keys it ("node file", "edge
// calls"), a name for it in a message, and its attributes.
func cg2Rec(r graph.Record) (key, name string, attrs map[string]string) {
	switch {
	case r.Node != nil:
		return "node " + r.Node.Kind, string(r.Node.ID), r.Node.Attrs
	case r.Edge != nil:
		return "edge " + string(r.Edge.Pred), string(r.Edge.Src) + " -> " + string(r.Edge.Dst), r.Edge.Attrs
	}
	return "", "", nil
}

// cg2Stated fails for each attribute of recs that cg2PyKept does not name
// for its kind of record.
func cg2Stated(t *testing.T, recs []graph.Record) {
	t.Helper()
	for _, r := range recs {
		key, name, attrs := cg2Rec(r)
		for attr := range attrs {
			if !slices.Contains(cg2PyKept[key], attr) {
				t.Errorf("the session graph log keeps %q on the %s %s; the consent text names %v for it", attr, key, name, cg2PyKept[key])
			}
		}
	}
}

// TestConsentGate2_APythonEditUnderAYesOfNineKeepsItsRecords: under a
// configured py unit whose index header names this build's py producer, a
// session whose content is kept, and a consent whose highest scope_version
// on file is 9 (ConsentScopeOnFile, asked of the store's root), an Edit of
// a .py file keeps its declared and landed groups in the session graph log,
// and each record of them carries what cg2PyKept names and no more, written
// out here from the edited file: each def's kind and line, the constant's
// kind and line, the call's line, the file's module and SHA-256; never a
// signature, a parameter or argument count, the constant's value, or an
// attribute cg2PyKept does not name. Base (card x315): the .py edit keeps
// nothing. Breaks: leave pylite unkept; ask the consent of a root other
// than the store's; keep an attribute past cg2PyKept, or drop one it names.
func TestConsentGate2_APythonEditUnderAYesOfNineKeepsItsRecords(t *testing.T) {
	asked := cg2Scope(t, 9, nil)
	f, py := cg2PyFix(t)
	f.pyEdit("toolu_cg2n", py)
	if kinds, _ := f.cg2Groups(); !slices.Equal(kinds, []string{"declared", "landed"}) {
		t.Fatalf("the session graph log holds groups %v of %s; want the declared and the landed", kinds, cg2PyRel)
	}
	if len(asked.roots) == 0 {
		t.Error("the .py edit asked ConsentScopeOnFile nothing; want the store's root")
	}
	for _, root := range asked.roots {
		if root != f.st.Root() {
			t.Errorf("ConsentScopeOnFile was asked of %q; want the store's root %q", root, f.st.Root())
		}
	}
	sum := sha256.Sum256([]byte(cg2PyOrig + cg2PyAdd))
	want := map[string]map[string]string{
		"file:" + cg2PyRel: {"digest": hex.EncodeToString(sum[:]), "module": "inv.a"},
		cg2RunID:           {"symkind": "func", "line": "1"},
		cg2ProbeID:         {"symkind": "func", "line": "5"},
		cg2ConstID:         {"symkind": "const", "line": "9"},
	}
	recs := f.cg2Raw()
	cg2Stated(t, recs)
	seen, calls := map[string]int{}, 0
	for _, r := range recs {
		if r.Node != nil {
			if w, ok := want[string(r.Node.ID)]; ok {
				seen[string(r.Node.ID)]++
				if !maps.Equal(r.Node.Attrs, w) {
					t.Errorf("the session graph log keeps %s with %v; want %v", r.Node.ID, r.Node.Attrs, w)
				}
			}
		}
		if r.Edge != nil && r.Edge.Pred == graph.Calls {
			calls++
			w := map[string]string{"line": "6"}
			if string(r.Edge.Src) != cg2ProbeID || string(r.Edge.Dst) != cg2RunID || !maps.Equal(r.Edge.Attrs, w) {
				t.Errorf("the session graph log keeps the call %s -> %s with %v; want %s -> %s with %v",
					r.Edge.Src, r.Edge.Dst, r.Edge.Attrs, cg2ProbeID, cg2RunID, w)
			}
		}
	}
	for id := range want {
		if seen[id] != 2 {
			t.Errorf("the session graph log keeps %s %d times; want once in each group", id, seen[id])
		}
	}
	if calls != 2 {
		t.Errorf("the session graph log keeps %d call edges; want probe's call of run once in each group", calls)
	}
	b, err := os.ReadFile(f.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(cg2PyValue)) {
		t.Errorf("the session graph log holds the constant's value %q:\n%s", cg2PyValue, b)
	}
}

// TestConsentGate2_APythonEditUnderAYesBelowNineKeepsNothing: the same edit
// under a consent whose highest scope_version on file is 8, the screen that
// names Go files alone, keeps no group of the .py file and no row naming a
// py symbol or the constant, while a .go edit in the same session keeps its
// declared and landed groups: a Go file's edits are kept from the screen
// that first named them, a Python file's from 9. Base: the scope-9 test
// above. Breaks: state pylite kept from 8, or with no scope of its own. The
// scope compared is pylite's own constant, never this build's screen, so a
// later screen moves no .py edit's start. Its premise row: gopg states not
// kept, and the .go edit keeps none.
func TestConsentGate2_APythonEditUnderAYesBelowNineKeepsNothing(t *testing.T) {
	cg2Scope(t, 8, nil)
	f, py := cg2PyFix(t)
	f.pyEdit("toolu_cg2p", py)
	f.cg2GoEdit("toolu_cg2g")
	kinds, goGroups := f.cg2Groups()
	if len(kinds) != 0 {
		t.Errorf("the session graph log holds groups %v of %s; want none", kinds, cg2PyRel)
	}
	if goGroups != 2 {
		t.Fatalf("premise: the .go edit kept %d groups; want the declared and the landed", goGroups)
	}
	b, err := os.ReadFile(f.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("sym:py:")) || bytes.Contains(b, []byte(cg2PyProbe)) {
		t.Errorf("the session graph log holds a row of the .py file's symbols:\n%s", b)
	}
}

// TestConsentGate2_APythonEditWithNoScopeReadKeepsNothing: where the scope
// on file is not known, ConsentScopeOnFile unset (a probe built without it)
// or answering 9 with an error (a consent.json it could not read), the .py
// edit keeps no group, while the .go edit keeps its two: a scope not read is
// no yes of 9. Breaks: keep the group where the reader is unset; take the 9
// past the error.
func TestConsentGate2_APythonEditWithNoScopeReadKeepsNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(t *testing.T)
	}{
		{"unset", cg2NoScope},
		{"an unreadable consent", func(t *testing.T) { cg2Scope(t, 9, errors.New("consent.json: not read")) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.set(t)
			f, py := cg2PyFix(t)
			f.pyEdit("toolu_cg2u", py)
			f.cg2GoEdit("toolu_cg2v")
			kinds, goGroups := f.cg2Groups()
			if len(kinds) != 0 {
				t.Errorf("the session graph log holds groups %v of %s; want none", kinds, cg2PyRel)
			}
			if goGroups != 2 {
				t.Fatalf("premise: the .go edit kept %d groups; want the declared and the landed", goGroups)
			}
		})
	}
}

// TestConsentGate2_AGoEditNeverReadsTheScope: a .go edit keeps its two
// groups and asks ConsentScopeOnFile nothing. Go's reader states no scope
// of its own, so its edits are kept under the session-content marker alone,
// as before, and the read of consent.json a .py edit makes stays off the Go
// path the hook's allocation budgets measure (test/perf/budgets.json).
// Break: read the scope on file for every reader.
func TestConsentGate2_AGoEditNeverReadsTheScope(t *testing.T) {
	asked := cg2Scope(t, 9, nil)
	f := newOvFix(t, "1\n")
	f.cg2GoEdit("toolu_cg2o")
	if _, goGroups := f.cg2Groups(); goGroups != 2 {
		t.Fatalf("premise: the .go edit kept %d groups; want the declared and the landed", goGroups)
	}
	if len(asked.roots) != 0 {
		t.Errorf("the .go edit asked ConsentScopeOnFile %d times; want none", len(asked.roots))
	}
}

// cg2UnkeptPy is pylite's reader stating its files not kept (lang.Kept)
// while still stating the scope and list pylite states (lang.KeptFrom).
type cg2UnkeptPy struct{ pylite.Resolver }

// SessionKept is false.
func (cg2UnkeptPy) SessionKept() bool { return false }

// TestConsentGate2_AReaderThatStatesNotKeptKeepsNothing: with cg2UnkeptPy
// as the hook's reader, under a yes of 9, a .py edit keeps no group: a
// reader's statement that its files are not kept stands before any scope it
// states, so a yes that reaches the scope does not override it. Breaks: read
// a reader as kept whatever it states (lang.Registry.SessionKept); drop the
// writer's check of it.
func TestConsentGate2_AReaderThatStatesNotKeptKeepsNothing(t *testing.T) {
	cg2WireAs(t, cg2UnkeptPy{}, func(py pylite.Resolver) lang.Resolver { return cg2UnkeptPy{py} })
	cg2Scope(t, 9, nil)
	f, py := cg2PyFix(t)
	f.pyEdit("toolu_cg2k", py)
	if kinds, _ := f.cg2Groups(); len(kinds) != 0 {
		t.Errorf("the session graph log holds groups %v of %s; want none", kinds, cg2PyRel)
	}
}

// cg2PlantPy is pylite's configured reader adding to each record it writes
// two sorts of attribute the log must leave out: "end" (a def's last line,
// which a later version of the reader may write), which no kind's list
// names, and each name cg2PyKept lists for some kind of record, set to
// cg2Planted, on every record whose own kind's list does not name it (a
// declares edge given "line", a file node given "symkind"). They are the
// witnesses that the log keeps what the text names for each kind of record
// and no more, whatever the reader writes.
type cg2PlantPy struct{ pylite.Resolver }

// cg2Planted is the value cg2PlantPy gives a listed name on a kind of
// record whose list does not name it.
const cg2Planted = "planted"

// Overlay is pylite's, each record given "end" and the planted names.
func (r cg2PlantPy) Overlay(rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error) {
	recs, err := r.Resolver.Overlay(rel, content, base, seq)
	for _, rec := range recs {
		key, _, attrs := cg2Rec(rec)
		if rec.Node != nil {
			rec.Node.Attrs = cg2Plant(key, attrs)
		}
		if rec.Edge != nil {
			rec.Edge.Attrs = cg2Plant(key, attrs)
		}
	}
	return recs, err
}

// cg2Plant is a copy of attrs, a record of kind key's, with "end" set and
// each name cg2PyKept lists for some kind of record but not for key set to
// cg2Planted.
func cg2Plant(key string, attrs map[string]string) map[string]string {
	out := maps.Clone(attrs)
	if out == nil {
		out = map[string]string{}
	}
	out["end"] = "99"
	for _, names := range cg2PyKept {
		for _, name := range names {
			if !slices.Contains(cg2PyKept[key], name) {
				out[name] = cg2Planted
			}
		}
	}
	return out
}

// cg2WirePlanted makes cg2PlantPy the hook's only reader for the test.
func cg2WirePlanted(t *testing.T) {
	cg2WireAs(t, cg2PlantPy{}, func(py pylite.Resolver) lang.Resolver { return cg2PlantPy{py} })
}

// cg2WireAs makes zero, registered indexed, the hook's only reader for the
// test, configured as wrap of this build's own py step's (index.Wire).
func cg2WireAs(t *testing.T, zero lang.Resolver, wrap func(pylite.Resolver) lang.Resolver) {
	t.Helper()
	old := overlayWire
	overlayWire = func() *lang.Registry {
		var reg lang.Registry
		reg.RegisterIndexed(zero, func(root string) (lang.Resolver, error) {
			r, _, err := index.Wire().Claim(root, cg2PyRel)
			if err != nil {
				return nil, err
			}
			py, ok := r.(pylite.Resolver)
			if !ok {
				return nil, errors.New("premise: this build's py step configured no pylite.Resolver")
			}
			return wrap(py), nil
		})
		return &reg
	}
	t.Cleanup(func() { overlayWire = old })
}

// TestConsentGate2_TheLogKeepsNoAttributeTheTextDoesNotName: with
// cg2PlantPy as the hook's reader, under a yes of 9, a .py edit keeps its
// declared and landed groups, no record of which carries "end" in the
// session graph log, while every record of the post's landed group, the
// one the conflict check reads in memory, still carries it: the list
// filters a copy of the reader's records for the log alone. Breaks: keep
// every attribute the reader writes (no list); filter the records the post
// holds.
func TestConsentGate2_TheLogKeepsNoAttributeTheTextDoesNotName(t *testing.T) {
	cg2WirePlanted(t)
	cg2Scope(t, 9, nil)
	f, py := cg2PyFix(t)
	p := f.pyEdit("toolu_cg2e", py)
	if kinds, _ := f.cg2Groups(); !slices.Equal(kinds, []string{"declared", "landed"}) {
		t.Fatalf("premise: the session graph log holds groups %v of %s; want the declared and the landed", kinds, cg2PyRel)
	}
	for _, r := range f.cg2Raw() {
		if key, name, attrs := cg2Rec(r); attrs["end"] != "" {
			t.Errorf("the session graph log keeps \"end\" on the %s %s; no kind's list names it", key, name)
		}
	}
	ends := 0
	for _, r := range p.landed.Group.Recs {
		if _, _, attrs := cg2Rec(r); attrs["end"] == "99" {
			ends++
		}
	}
	if n := len(p.landed.Group.Recs); n == 0 || ends != n {
		t.Errorf("the post's landed group carries \"end\" on %d of its %d records; want every record whole in memory", ends, n)
	}
}

// TestConsentGate2_ANameKeptForOneKindOfRecordIsLeftOutOnAnother: with
// cg2PlantPy as the hook's reader, under a yes of 9, no record of a .py
// edit's groups in the session graph log carries a planted name, though the
// post's landed group carries one on every kind of record it holds: a file
// node's "line" and "symkind", a sym node's "digest" and "module", a calls
// edge's "symkind", a declares edge's every listed name. The list is closed
// per kind of record (ruling x371 item 8c), never one set of names for all.
// Break: keep a name any kind lists on every kind.
func TestConsentGate2_ANameKeptForOneKindOfRecordIsLeftOutOnAnother(t *testing.T) {
	cg2WirePlanted(t)
	cg2Scope(t, 9, nil)
	f, py := cg2PyFix(t)
	p := f.pyEdit("toolu_cg2c", py)
	if kinds, _ := f.cg2Groups(); !slices.Equal(kinds, []string{"declared", "landed"}) {
		t.Fatalf("premise: the session graph log holds groups %v of %s; want the declared and the landed", kinds, cg2PyRel)
	}
	planted := map[string]bool{}
	for _, r := range p.landed.Group.Recs {
		key, _, attrs := cg2Rec(r)
		for _, v := range attrs {
			if v == cg2Planted {
				planted[key] = true
			}
		}
	}
	for _, key := range []string{"node file", "node sym", "edge declares", "edge calls"} {
		if !planted[key] {
			t.Fatalf("premise: the post's landed group carries no planted name on a %s", key)
		}
	}
	for _, r := range f.cg2Raw() {
		key, name, attrs := cg2Rec(r)
		for attr, v := range attrs {
			if v == cg2Planted {
				t.Errorf("the session graph log keeps %q on the %s %s; cg2PyKept names it for another kind of record alone", attr, key, name)
			}
		}
	}
}

// cg2Many is cg2PyOrig and n more defs, each returning 1: two records each
// (the def and its declares edge), so past graph.MaxOverlayRecords once n
// passes 200.
func cg2Many(n int) string {
	var b strings.Builder
	b.WriteString(cg2PyOrig)
	for i := range n {
		fmt.Fprintf(&b, "\n\ndef many_%03d():\n    return 1\n", i)
	}
	return b.String()
}

// TestConsentGate2_APythonEditPastTheBoundKeepsTheCutAndAWholeMarker: under
// a yes of 9, an edit of a .py file to 251 defs, past
// graph.MaxOverlayRecords records, keeps its declared and landed groups, in
// each of which the file node carries the bound's "_cut" beside its module
// and SHA-256, and the closing marker carries exactly what a Go group's
// marker carries (session_id, tool_use_id, file, digest, kind, seq, count,
// carried), its count the number of records the group wrote: the list
// applies to the reader's records before graph.BuildOverlayGroup, which
// adds both (ruling x371 items 6 and 8d). Break: filter the built group,
// and the cut and the marker's attributes are dropped.
func TestConsentGate2_APythonEditPastTheBoundKeepsTheCutAndAWholeMarker(t *testing.T) {
	cg2Scope(t, 9, nil)
	f, py := cg2PyFix(t)
	next := cg2Many(250)
	f.pyEditTo("toolu_cg2b", py, next)
	sum := sha256.Sum256([]byte(next))
	digest := hex.EncodeToString(sum[:])
	file := map[string]string{"_cut": "true", "digest": digest, "module": "inv.a"}
	var kinds []string
	var group []graph.Record
	for _, r := range f.cg2Lines() {
		if r.Node == nil || r.Node.Kind != graph.KindOverlayGroup {
			group = append(group, r)
			continue
		}
		recs, attrs := group, r.Node.Attrs
		group = nil
		if attrs["file"] != string(graph.FileID(cg2PyRel)) {
			continue
		}
		kinds = append(kinds, attrs["kind"])
		files := 0
		for _, rec := range recs {
			if rec.Node != nil && rec.Node.Kind == "file" {
				files++
				if !maps.Equal(rec.Node.Attrs, file) {
					t.Errorf("the %s group keeps the file node with %v; want %v", attrs["kind"], rec.Node.Attrs, file)
				}
			}
		}
		if files != 1 {
			t.Errorf("the %s group keeps %d file nodes; want one", attrs["kind"], files)
		}
		want := map[string]string{"session_id": ovSess, "tool_use_id": "toolu_cg2b", "file": string(graph.FileID(cg2PyRel)),
			"digest": digest, "kind": attrs["kind"], "seq": attrs["seq"], "count": strconv.Itoa(len(recs)),
			"carried": (pylite.Resolver{}).OverlayCarried().String()}
		if !maps.Equal(attrs, want) {
			t.Errorf("the %s group's marker carries %v; want %v", attrs["kind"], attrs, want)
		}
		if seq, err := strconv.ParseInt(attrs["seq"], 10, 64); err != nil || seq <= 0 {
			t.Errorf("the %s group's marker carries seq %q; want a positive number", attrs["kind"], attrs["seq"])
		}
	}
	slices.Sort(kinds)
	if !slices.Equal(kinds, []string{"declared", "landed"}) {
		t.Fatalf("the session graph log holds groups %v of %s; want the declared and the landed", kinds, cg2PyRel)
	}
}

// TestConsentGate2_AKeptReaderOtherThanGoCarriesItsOwnSet: under a yes of
// 9, a .py edit keeps a landed group of the file carrying pylite's set of
// predicates, the claiming reader's, not Go's. It holds the property
// TestLANGREG1_APythonEditWritesAnOverlayGroup held before a .py edit kept
// nothing (card x315). Break: carry the Go resolver's set for every
// language.
func TestConsentGate2_AKeptReaderOtherThanGoCarriesItsOwnSet(t *testing.T) {
	cg2Scope(t, 9, nil)
	f, py := cg2PyFix(t)
	f.pyEdit("toolu_cg2k", py)
	groups, _, _ := f.read()
	landed := 0
	for _, g := range groups {
		if g.File != graph.FileID(cg2PyRel) || g.Kind != graph.OverlayLanded {
			continue
		}
		landed++
		if got, want := g.Carried.String(), (pylite.Resolver{}).OverlayCarried().String(); got != want {
			t.Errorf("the .py group carries %q; want pylite's %q", got, want)
		}
	}
	if landed != 1 {
		t.Fatalf("premise: %d landed groups of %s; want one", landed, cg2PyRel)
	}
}

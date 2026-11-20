package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

const (
	cg2PyRel   = "inv/a.py"
	cg2PyOrig  = "def run():\n    return 1\n"
	cg2PyAdd   = "\n\nCG2_PROBE = \"brindle\"\n"
	cg2PyProbe = "CG2_PROBE"
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

// pyEdit is an Edit of py that adds the constant CG2_PROBE, as the hook
// runs it: the PreToolUse, the file as the edit leaves it, the PostToolUse.
func (f *ovFix) pyEdit(id, py string) {
	f.t.Helper()
	in := map[string]any{"file_path": py, "old_string": cg2PyOrig, "new_string": cg2PyOrig + cg2PyAdd}
	f.pre(id, "Edit", in)
	if err := os.WriteFile(py, []byte(cg2PyOrig+cg2PyAdd), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.post(id, "Edit", in)
}

// TestConsentGate2_APythonEditKeepsNothingInTheSessionLog: under a
// configured py unit whose index header names this build's py producer, a
// session whose content is kept edits a .py file to add a constant, then a
// .go file. What a yes keeps of a session's edits is named for Go files
// alone (internal/disclose/paragraphs.go, the KEPT FROM YOUR SESSIONS
// paragraph), so the session graph log holds no group of the .py file and
// no row naming a py symbol or the constant, while the .go edit keeps its
// declared and landed groups. Base: the .py edit keeps its groups, the
// constant's node among them. Breaks: let the writer keep a group whatever
// the claiming reader states (Registry.SessionKept), or state pylite kept.
func TestConsentGate2_APythonEditKeepsNothingInTheSessionLog(t *testing.T) {
	f, py := cg2PyFix(t)
	f.pyEdit("toolu_cg2p", py)
	in := f.edit("func Old() {}", "func New() {}")
	f.pre("toolu_cg2g", "Edit", in)
	if err := os.WriteFile(f.file, []byte("package pkg\n\nfunc New() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.post("toolu_cg2g", "Edit", in)
	groups, _, exists := f.read()
	if !exists {
		t.Fatal("premise: the .go edit kept no session graph log")
	}
	goGroups := 0
	for _, g := range groups {
		switch g.File {
		case graph.FileID(cg2PyRel):
			t.Errorf("the session graph log holds a %s group of %s; want none", g.Kind, cg2PyRel)
		case graph.FileID(ovRel):
			goGroups++
		}
	}
	if goGroups != 2 {
		t.Errorf("premise: the .go edit kept %d groups; want the declared and the landed", goGroups)
	}
	b, err := os.ReadFile(f.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("sym:py:")) || bytes.Contains(b, []byte(cg2PyProbe)) {
		t.Errorf("the session graph log holds a row of the .py file's symbols:\n%s", b)
	}
}

// cg2KeptPy is pylite's reader stating that a session's edits of its files
// are kept (lang.Kept): the witness for a kept reader other than Go's,
// which this build does not have.
type cg2KeptPy struct{ pylite.Resolver }

// SessionKept is true.
func (cg2KeptPy) SessionKept() bool { return true }

// TestConsentGate2_AKeptReaderOtherThanGoCarriesItsOwnSet: with cg2KeptPy
// registered, indexed, as the only reader, configured by this build's own
// py step, a .py edit keeps a landed group of the file carrying pylite's set
// of predicates, the claiming reader's, not Go's. It holds the property
// TestLANGREG1_APythonEditWritesAnOverlayGroup held before a .py edit kept
// nothing (card x315). Break: carry the Go resolver's set for every
// language.
func TestConsentGate2_AKeptReaderOtherThanGoCarriesItsOwnSet(t *testing.T) {
	old := overlayWire
	overlayWire = func() *lang.Registry {
		var reg lang.Registry
		reg.RegisterIndexed(cg2KeptPy{}, func(root string) (lang.Resolver, error) {
			r, _, err := index.Wire().Claim(root, cg2PyRel)
			return r, err
		})
		return &reg
	}
	t.Cleanup(func() { overlayWire = old })
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

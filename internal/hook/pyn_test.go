package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// pynUnindexedPy is the line an edit of a .py file says under an index whose
// py records another version of the reader wrote, spelled out by hand.
const pynUnindexedPy = "rashomon: the index here holds no py records written by this version of the py reader, so edits to .py files carry no unit, caller or conflict facts until rashomon graph index runs."

// TestPYN_AnIndexOfTheSecondPyliteSaysTheUnindexedLine: an index whose
// header names pylite/2 as the producer of its py records holds the source
// text that reader wrote (constant values, parameter defaults, keyword
// arguments), so under it, with a yes on file of 9 (the scope pylite states
// its files kept from, lang.KeptFrom, so the edit passes the gate:
// keptGoEdit, card x381), the first edit of a .py file says the unindexed
// line, a second .py file says nothing, and no overlay group is kept, as for
// any older reader. Break: leave the reader's version at 2, so that index
// reads as current and nothing tells the user to build it again.
func TestPYN_AnIndexOfTheSecondPyliteSaysTheUnindexedLine(t *testing.T) {
	cg2Scope(t, 9, nil)
	f := newOvFix(t, "1\n")
	cfg := filepath.Join(f.repo, filepath.FromSlash(index.ConfigFile))
	if err := os.WriteFile(cfg, []byte(`{"units": [{"path": "inv", "modules": ["inv"], "lang": "py"}]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hdr := graph.Header{Repo: "r", Rev: graph.HeaderRev{Commit: ovCommit},
		Producers: map[string]string{pylite.Lang: "pylite/2"}}
	if err := graph.SaveFile(f.repo, graph.NewStore(), hdr); err != nil {
		t.Fatal(err)
	}
	b := f.kindPost(ovSess, "toolu_pn1", "inv/a.py")
	if len(b) == 0 {
		t.Fatalf("the first .py edit under an index of pylite/2 said nothing; want %q", pynUnindexedPy)
	}
	if got := kindSaid(t, b); got != pynUnindexedPy {
		t.Errorf("the first .py edit said %q; want %q", got, pynUnindexedPy)
	}
	if b := f.kindPost(ovSess, "toolu_pn2", "inv/b.py"); b != nil {
		t.Errorf("a second .py file in the session printed %q; want nothing", b)
	}
	if groups, _, _ := f.read(); len(groups) != 0 {
		t.Errorf("the .py edits under the index of pylite/2 kept %d overlay groups; want none", len(groups))
	}
}

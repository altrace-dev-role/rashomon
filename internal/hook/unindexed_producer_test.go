package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// TestUnindexedLanguage_AnOlderReadersIndexIsUnindexed: under an index
// whose header names a py producer other than this build's reader's
// (pylite/1), with a configuration that declares a py unit, and a yes on
// file of 9 (the scope pylite states its files kept from, lang.KeptFrom, so
// the edit passes the gate: keptGoEdit, card x381), an edit of a .py file
// says the language's line as a system message alone, once in the session,
// and keeps no overlay group: that index's py records are another reader's,
// and an overlay over them would mix the two readers' ids. The line holds
// no byte of the header. Break: test only that the header names py (the
// edit is overlaid, a group is kept, and nothing is said), or print the
// header's string.
func TestUnindexedLanguage_AnOlderReadersIndexIsUnindexed(t *testing.T) {
	cg2Scope(t, 9, nil)
	f := newOvFix(t, "1\n")
	cfg := filepath.Join(f.repo, filepath.FromSlash(index.ConfigFile))
	if err := os.WriteFile(cfg, []byte(`{"units": [{"path": "inv", "modules": ["inv"], "lang": "py"}]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hdr := graph.Header{Repo: "r", Rev: graph.HeaderRev{Commit: ovCommit},
		Producers: map[string]string{pylite.Lang: "pylite/1"}}
	if err := graph.SaveFile(f.repo, graph.NewStore(), hdr); err != nil {
		t.Fatal(err)
	}
	want := unindexedLineOf(pylite.Lang, ".py")
	if got := kindSaid(t, f.kindPost(ovSess, "toolu_pv1", "inv/a.py")); got != want {
		t.Errorf("the first .py edit said %q; want %q", got, want)
	}
	if b := f.kindPost(ovSess, "toolu_pv2", "inv/b.py"); b != nil {
		t.Errorf("a second .py file in the session printed %q; want nothing", b)
	}
	if groups, _, _ := f.read(); len(groups) != 0 {
		t.Errorf("the .py edits under the older reader's index kept %d overlay groups; want none", len(groups))
	}
}

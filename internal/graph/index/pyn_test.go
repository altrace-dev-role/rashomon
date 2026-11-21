package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// pynApp holds five canary strings, each where the Python reader of
// pylite/2 copied source text into a record: the right-hand side of a
// constant, the default of a parameter, the value of a keyword argument, a
// quoted value in an SQL statement and a word of a prose line that holds the
// word update (the last two as the columns of a reads or writes edge).
const pynApp = `API_TOKEN = "pyn-canary-const-d41f"


def connect(host, secret="pyn-canary-default-82be", timeout: int = 5):
    return host


def run():
    return connect("h", secret="pyn-canary-kwarg-c7a9")


def audit(db, log):
    db.execute("SELECT id FROM users WHERE token = 'pyn_canary_sql_5e3a'")
    log.warning("failed to update billing record for pyn_canary_prose_7b1d")
`

// TestPYN_TheIndexAndTheOverlayKeepNoSourceText: the index rashomon graph
// index writes (Build, then SaveFile, the two calls of cmd/rashomon
// graph.go) and the records an edit's overlay returns hold no byte of a
// constant's value, a parameter's default, a keyword argument's text or a
// string literal's words, and no record derived from a string literal (no
// table node, no reads or writes edge), so a secret written in a Python file
// stays out of .rashomon/graph.jsonl. The premises are that the reader read
// the file: the constant, both functions and the call are in the index, and
// the constant and the call are in the overlay. Break: write any of the five
// into a record (a const value, a sig with its defaults, a kw.NAME on a
// call, the cols of an SQL-like string), or a table with its reads or writes.
func TestPYN_TheIndexAndTheOverlayKeepNoSourceText(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "py"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "py", "app.py"), []byte(pynApp), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Units: []Unit{{Path: "py", ModRoot: "py", Modules: []string{"app"}, Lang: pylite.Lang}}}
	res, err := Build(root, "c1", cfg)
	if err != nil {
		t.Fatal(err)
	}
	const (
		token   graph.ID = "sym:py:py:const:app.API_TOKEN"
		connect graph.ID = "sym:py:py:func:app.connect"
		run     graph.ID = "sym:py:py:func:app.run"
	)
	rev := graph.RevCommit("c1")
	for _, id := range []graph.ID{token, connect, run} {
		if _, ok := res.Store.Node(id, rev); !ok {
			t.Fatalf("premise: the index has no node %s", id)
		}
	}
	if _, ok := res.Store.Edge(run, graph.Calls, connect, rev); !ok {
		t.Fatalf("premise: the index has no call from %s to %s", run, connect)
	}
	if err := graph.SaveFile(root, res.Store, res.Header); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(graph.GraphFile)))
	if err != nil {
		t.Fatal(err)
	}
	recs, err := python(cfg).Overlay("py/app.py", []byte(pynApp), rev, 1)
	if err != nil {
		t.Fatal(err)
	}
	var ovToken, ovCall bool
	for _, r := range recs {
		ovToken = ovToken || (r.Node != nil && r.Node.ID == token)
		ovCall = ovCall || (r.Edge != nil && r.Edge.Src == run && r.Edge.Pred == graph.Calls && r.Edge.Dst == connect)
	}
	if !ovToken || !ovCall {
		t.Fatalf("premise: the overlay has the constant %v and the call %v; want both", ovToken, ovCall)
	}
	for _, n := range res.Store.Nodes(rev) {
		if n.Kind == "table" {
			t.Errorf("the index has the table node %s; want no record from a string literal", n.ID)
		}
	}
	for _, e := range res.Store.Edges(rev) {
		if e.Pred == graph.Reads || e.Pred == graph.Writes {
			t.Errorf("the index has the %s edge from %s to %s; want no record from a string literal", e.Pred, e.Src, e.Dst)
		}
	}
	for _, r := range recs {
		if r.Node != nil && r.Node.Kind == "table" {
			t.Errorf("the overlay has the table node %s; want no record from a string literal", r.Node.ID)
		}
		if r.Edge != nil && (r.Edge.Pred == graph.Reads || r.Edge.Pred == graph.Writes) {
			t.Errorf("the overlay has the %s edge from %s to %s; want no record from a string literal", r.Edge.Pred, r.Edge.Src, r.Edge.Dst)
		}
	}
	overlay, err := json.Marshal(recs)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ where, text string }{
		{"the right-hand side of a constant", "pyn-canary-const-d41f"},
		{"the default of a parameter", "pyn-canary-default-82be"},
		{"the value of a keyword argument", "pyn-canary-kwarg-c7a9"},
		{"a quoted value in an SQL statement", "pyn_canary_sql_5e3a"},
		{"a word of a prose line holding update", "pyn_canary_prose_7b1d"},
	} {
		if strings.Contains(string(saved), c.text) {
			t.Errorf("the saved index holds %s, %s; want none of it", c.where, c.text)
		}
		if strings.Contains(string(overlay), c.text) {
			t.Errorf("the overlay records hold %s, %s; want none of it", c.where, c.text)
		}
	}
}

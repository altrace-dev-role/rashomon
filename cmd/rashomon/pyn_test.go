package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// pynSource is the fixture of internal/graph/index/pyn_test.go: five canary
// strings, each where the Python reader of pylite/2 copied source text into
// a record.
const pynSource = `API_TOKEN = "pyn-canary-const-d41f"


def connect(host, secret="pyn-canary-default-82be", timeout: int = 5):
    return host


def run():
    return connect("h", secret="pyn-canary-kwarg-c7a9")


def audit(db, log):
    db.execute("SELECT id FROM users WHERE token = 'pyn_canary_sql_5e3a'")
    log.warning("failed to update billing record for pyn_canary_prose_7b1d")
`

// TestPYN_GraphIndexWritesTheIndexWhole: rashomon graph index over a
// repository whose .rashomon/graph.jsonl an older reader wrote (its header
// names pylite/2, its records hold the five canary strings as that reader
// wrote them, and a sixth on a constant the source no longer declares)
// writes the index again whole, and the saved bytes hold none of the six:
// graphIndex builds from the configuration and the source files only, and
// SaveFile replaces the file by a temporary file renamed over it, so no byte
// of the older index is carried. The premises are that the older file held
// all six before the run and that the new index holds the constant the
// source declares. Break: carry any record or byte of the older file into
// the new one, or write any of the five from the source.
func TestPYN_GraphIndexWritesTheIndexWhole(t *testing.T) {
	dir := t.TempDir()
	for rel, text := range map[string]string{
		index.ConfigFile: `{"units":[{"path":"py","mod_root":"py","modules":["app"],"lang":"py"}]}`,
		"py/app.py":      pynSource,
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const (
		idToken   graph.ID = "sym:py:py:const:app.API_TOKEN"
		idGone    graph.ID = "sym:py:py:const:app.OLD_TOKEN"
		idConnect graph.ID = "sym:py:py:func:app.connect"
		idRun     graph.ID = "sym:py:py:func:app.run"
		idAudit   graph.ID = "sym:py:py:func:app.audit"
	)
	rev := graph.RevCommit("C0")
	older := graph.NewStore()
	addNode := func(id graph.ID, kind string, attrs map[string]string) {
		older.AssertNode(graph.Node{ID: id, Kind: kind, Attrs: attrs, Tier: graph.Algorithmic, Conf: 0.9, Rev: rev})
	}
	addEdge := func(src graph.ID, pred graph.Pred, dst graph.ID, attrs map[string]string) {
		older.AssertEdge(graph.Edge{Src: src, Pred: pred, Dst: dst, Attrs: attrs, Tier: graph.Algorithmic, Conf: 0.9, Rev: rev})
	}
	addNode(idToken, "sym", map[string]string{"symkind": "const", "value": `"pyn-canary-const-d41f"`, "line": "1"})
	addNode(idGone, "sym", map[string]string{"symkind": "const", "value": `"pyn-canary-oldfile-9c3e"`, "line": "2"})
	addNode(idConnect, "sym", map[string]string{"symkind": "func", "sig": `(host, secret="pyn-canary-default-82be", timeout: int = 5)`, "arity_min": "1", "arity_max": "3", "line": "4"})
	addNode(idRun, "sym", map[string]string{"symkind": "func", "sig": "()", "arity_min": "0", "arity_max": "0", "line": "8"})
	addNode(idAudit, "sym", map[string]string{"symkind": "func", "sig": "(db, log)", "arity_min": "2", "arity_max": "2", "line": "12"})
	addNode(graph.TableID("users"), "table", map[string]string{"name": "users"})
	addNode(graph.TableID("billing"), "table", map[string]string{"name": "billing"})
	addEdge(idRun, graph.Calls, idConnect, map[string]string{"argc": "2", "kw.secret": `"pyn-canary-kwarg-c7a9"`, "line": "9"})
	addEdge(idAudit, graph.Reads, graph.TableID("users"), map[string]string{"cols": "id,pyn_canary_sql_5e3a,token", "line": "13"})
	addEdge(idAudit, graph.Writes, graph.TableID("billing"), map[string]string{"cols": "failed,for,pyn_canary_prose_7b1d,record,to", "line": "14"})
	hdr := graph.Header{Repo: "r", Rev: graph.HeaderRev{Commit: "C0"}, Producers: map[string]string{pylite.Lang: "pylite/2"}}
	if err := graph.SaveFile(dir, older, hdr); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, filepath.FromSlash(graph.GraphFile))
	canaries := []struct{ where, text string }{
		{"the right-hand side of a constant", "pyn-canary-const-d41f"},
		{"the default of a parameter", "pyn-canary-default-82be"},
		{"the value of a keyword argument", "pyn-canary-kwarg-c7a9"},
		{"a quoted value in an SQL statement", "pyn_canary_sql_5e3a"},
		{"a word of a prose line holding update", "pyn_canary_prose_7b1d"},
		{"a constant only the older index declares", "pyn-canary-oldfile-9c3e"},
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range canaries {
		if !strings.Contains(string(before), c.text) {
			t.Fatalf("premise: the older index holds no %s, %s", c.where, c.text)
		}
	}
	var out strings.Builder
	if err := graphIndex([]string{dir}, &out); err != nil {
		t.Fatalf("graph index = %v; want the index written\n%s", err, out.String())
	}
	s, h, err := graph.LoadFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Node(idToken, graph.RevCommit(h.Rev.Commit)); !ok {
		t.Fatalf("premise: the index graph index wrote has no node %s", idToken)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range canaries {
		if strings.Contains(string(saved), c.text) {
			t.Errorf("the index graph index wrote holds %s, %s; want none of it", c.where, c.text)
		}
	}
}

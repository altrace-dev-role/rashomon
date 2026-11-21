package pylite_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// pynResolver reads the unit u, which holds the top-level module app.
var pynResolver = pylite.Resolver{Units: []pylite.Unit{{Path: "u", ModRoot: "u"}}, Modules: map[string]string{"app": "u"}}

// pynDB is the module the fixture imports from.
const pynDB = `def save(row, table="t"):
    return row
`

// pynMain writes every record the reader has: a module constant, functions
// and a method, an import and a from-import, and calls with a keyword
// argument; it also holds an SQL read and an SQL write, from which the
// reader writes no record.
const pynMain = `import app.db
from app.db import save

LIMIT = 10


def helper(x, flag=0):
    return x


class Store:
    def put(self, key, ttl=60):
        return save(key, table="sessions")


def load(key):
    q = "SELECT id FROM users WHERE name = key"
    w = "INSERT INTO audit (id) VALUES (1)"
    return helper(key, flag=1)
`

// TestPYN_TheReaderWritesOnlyTheseAttributeKeys: the attribute keys the
// reader writes, by record (a node by its kind and a sym by its symkind, an
// edge by its predicate), through Extract and through Overlay, are exactly
// this list: a hash, a module name, kind words, parameter names and numbers,
// and no key that holds source text; the fixture's two SQL strings give no
// record at all, so no table node, reads edge, writes edge or cols key is in
// it. Break: add a key to any record, as the reader of pylite/2 did with
// value on a constant, kw.NAME on a call and cols on a reads or writes edge,
// or write a record from a string literal.
func TestPYN_TheReaderWritesOnlyTheseAttributeKeys(t *testing.T) {
	root := t.TempDir()
	for rel, src := range map[string]string{"u/app/db.py": pynDB, "u/app/main.py": pynMain} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := pynResolver.Extract(root, []string{"u/app/db.py", "u/app/main.py"}, graph.RevCommit("c1"))
	if err != nil {
		t.Fatal(err)
	}
	ov, err := pynResolver.Overlay("u/app/main.py", []byte(pynMain), graph.RevCommit("c1"), 1)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]map[string]bool{}
	note := func(group string, attrs map[string]string) {
		if keys[group] == nil {
			keys[group] = map[string]bool{}
		}
		for k := range attrs {
			keys[group][k] = true
		}
	}
	for _, r := range append(recs, ov...) {
		switch {
		case r.Node != nil && r.Node.Kind == "sym":
			note("node sym "+r.Node.Attrs["symkind"], r.Node.Attrs)
		case r.Node != nil:
			note("node "+r.Node.Kind, r.Node.Attrs)
		case r.Edge != nil:
			note("edge "+string(r.Edge.Pred), r.Edge.Attrs)
		}
	}
	want := map[string]string{
		"node file":       "digest,module",
		"node sym const":  "line,symkind",
		"node sym func":   "arity_max,arity_min,line,sig,symkind",
		"node sym method": "arity_max,arity_min,line,sig,symkind",
		"edge contains":   "",
		"edge declares":   "",
		"edge imports":    "",
		"edge references": "",
		"edge calls":      "argc,line",
	}
	for group, w := range want {
		set, ok := keys[group]
		if !ok {
			t.Errorf("%s: the fixture wrote no such record; want one", group)
			continue
		}
		if got := pynKeyList(set); got != w {
			t.Errorf("%s: keys %q; want %q", group, got, w)
		}
	}
	for group, set := range keys {
		if _, ok := want[group]; !ok {
			t.Errorf("%s: a record the list does not name, keys %q", group, pynKeyList(set))
		}
	}
}

// pynKeyList is the keys of set, sorted and joined by commas.
func pynKeyList(set map[string]bool) string {
	var ks []string
	for k := range set {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

// pynSig puts a default and an annotation on a function's parameters and on
// a method's, and star parameters on the method.
const pynSig = `def connect(host, secret="x", timeout: int = 5):
    return host


class Store:
    def put(self, key: str, ttl=60, *args, **opts):
        return key
`

// TestPYN_ASignatureNamesItsParametersOnly: a def's sig names its
// parameters and nothing else, no default and no annotation, while its arity
// still counts a parameter with a default as optional and a star parameter
// as any number. Break: keep the parameter text as written in sig, or count
// the arity from the names alone (every parameter then required).
func TestPYN_ASignatureNamesItsParametersOnly(t *testing.T) {
	recs, err := pynResolver.Overlay("u/app/sig.py", []byte(pynSig), graph.RevCommit("c1"), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []struct {
		id               graph.ID
		sig, least, most string
	}{
		{"sym:py:u:func:app.sig.connect", "(host, secret, timeout)", "1", "3"},
		{"sym:py:u:method:app.sig.Store.put", "(self, key, ttl)", "1", "*"},
	} {
		n, ok := nodeOf(recs, w.id)
		if !ok {
			t.Errorf("no node %s; want the def", w.id)
			continue
		}
		if got := n.Attrs["sig"]; got != w.sig {
			t.Errorf("%s: sig %q; want %q", w.id, got, w.sig)
		}
		if n.Attrs["arity_min"] != w.least || n.Attrs["arity_max"] != w.most {
			t.Errorf("%s: arity %s to %s; want %s to %s", w.id, n.Attrs["arity_min"], n.Attrs["arity_max"], w.least, w.most)
		}
	}
}

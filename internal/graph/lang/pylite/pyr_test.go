package pylite_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// The reader of card PY-R, pylite/4: every func and method node carries
// "end", the line its body ends on, a number and no text; a lambda's own
// commas split nothing, so a lambda in a default or in a call is one
// parameter or one argument; and the reader is pylite version 4.

// pyrResolver reads the unit r, its modules under the package lib.
var pyrResolver = pylite.Resolver{Units: []pylite.Unit{{Path: "r", ModRoot: "r"}}, Modules: map[string]string{"lib": "r"}}

// pyrRead is the records of one read of a fixture, by Extract or by Overlay.
type pyrRead struct {
	how  string
	recs []graph.Record
}

// pyrBoth is the records of src read as the file rel twice: by Extract, from
// the file written under a new root, and by Overlay, as an edit's content.
func pyrBoth(t *testing.T, rel, src string) []pyrRead {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ex, err := pyrResolver.Extract(root, []string{rel}, graph.RevCommit("c1"))
	if err != nil {
		t.Fatal(err)
	}
	ov, err := pyrResolver.Overlay(rel, []byte(src), graph.RevCommit("c1"), 1)
	if err != nil {
		t.Fatal(err)
	}
	return []pyrRead{{"Extract", ex}, {"Overlay", ov}}
}

// lineOf is the 1-based line of src that holds needle, which one line only
// may hold.
func lineOf(t *testing.T, src, needle string) string {
	t.Helper()
	found := ""
	for i, l := range strings.Split(src, "\n") {
		if !strings.Contains(l, needle) {
			continue
		}
		if found != "" {
			t.Fatalf("%q is on more than one line of the fixture", needle)
		}
		found = strconv.Itoa(i + 1)
	}
	if found == "" {
		t.Fatalf("%q is on no line of the fixture", needle)
	}
	return found
}

// pyrSym is the id of the symbol name of kind symkind in the module lib.m.
func pyrSym(symkind, name string) graph.ID {
	return graph.SymID("py", "r", symkind, "lib.m."+name)
}

// pyrEndSrc has a body on its def line, parameters over two lines, a call
// over two lines, a comment inside a body and one after it, a def with no
// body line, a def inside a def, two methods, and a call that closes on the
// last line of the file.
const pyrEndSrc = `import lib.dep


def one(): return 1


def span(a,
         b):
    x = helper(a,
               b)
    # a comment in the body
    return x

    # a comment after the body, indented

def stub(a2,
         b2): ...


def outer():
    def inner():
        return 2
    return inner()


class Store:
    def get(self, key):
        return key

    def put(self, key,
            value):
        pass


def last():
    return helper(1,
                  last_arg)
`

// TestPYR_EveryDefCarriesTheLineItsBodyEndsOn (card PY-R): on Extract and
// on Overlay, each func and method node's "end" is the 1-based last line of
// its body: the last line a body line or a call in it reaches, a def nested
// in it being part of it, and no blank or comment line counting; with no
// body line it is the line its parameters close on. Break: leave end on the
// line the parameters close on, or write no end.
func TestPYR_EveryDefCarriesTheLineItsBodyEndsOn(t *testing.T) {
	for _, b := range pyrBoth(t, "r/lib/m.py", pyrEndSrc) {
		for _, w := range []struct{ kind, name, last string }{
			{"func", "one", "def one(): return 1"},
			{"func", "span", "return x"},
			{"func", "stub", "b2): ..."},
			{"func", "outer", "return inner()"},
			{"method", "Store.get", "return key"},
			{"method", "Store.put", "pass"},
			{"func", "last", "last_arg)"},
		} {
			id := pyrSym(w.kind, w.name)
			n, ok := nodeOf(b.recs, id)
			if !ok {
				t.Errorf("%s: no node %s; want the def", b.how, id)
				continue
			}
			if got, want := n.Attrs["end"], lineOf(t, pyrEndSrc, w.last); got != want {
				t.Errorf("%s: %s end %q; want %q, the line of %q", b.how, id, got, want, w.last)
			}
		}
	}
}

// pyrLambdaSrc has a lambda with two parameters as a default, a lambda with
// none, a lambda inside a lambda's parameters, and three calls each with a
// lambda among its arguments.
const pyrLambdaSrc = `def sort_by(items, key=lambda a, b: a, reverse=False):
    return items


def keyed(fn=lambda: 0, *, opts={1: 2}):
    return fn


def nested(f=lambda x, g=lambda y, z: y, w=1: x, h=2):
    return f


def use(xs):
    sort_by(xs, lambda p, q: p)
    sort_by(xs, key=lambda p, q=(1, 2): p, reverse=True)
    sort_by(lambda m: m, xs)
    return xs
`

// TestPYR_ALambdasCommasAreItsOwn (card PY-R): on Extract and on Overlay, a
// comma between the word lambda and its colon splits no parameter list and
// no argument list, a lambda inside a lambda's parameters closing at its own
// colon first, while a comma after a lambda's body splits as any other. So
// the sig names each parameter of the def once and the bounds and each
// call's argc count a lambda once. Break: split on every comma at depth
// zero, or let the first colon close every open lambda.
func TestPYR_ALambdasCommasAreItsOwn(t *testing.T) {
	for _, b := range pyrBoth(t, "r/lib/m.py", pyrLambdaSrc) {
		for _, w := range []struct{ name, sig, least, most string }{
			{"sort_by", "(items, key, reverse)", "1", "3"},
			{"keyed", "(fn, opts)", "0", "2"},
			{"nested", "(f, h)", "0", "2"},
		} {
			id := pyrSym("func", w.name)
			n, ok := nodeOf(b.recs, id)
			if !ok {
				t.Errorf("%s: no node %s; want the def", b.how, id)
				continue
			}
			if n.Attrs["sig"] != w.sig || n.Attrs["arity_min"] != w.least || n.Attrs["arity_max"] != w.most {
				t.Errorf("%s: %s sig %q arity %s to %s; want %q, %s to %s", b.how, id,
					n.Attrs["sig"], n.Attrs["arity_min"], n.Attrs["arity_max"], w.sig, w.least, w.most)
			}
		}
		got := map[string]string{}
		for _, r := range b.recs {
			if e := r.Edge; e != nil && e.Pred == graph.Calls && e.Src == pyrSym("func", "use") && e.Dst == pyrSym("func", "sort_by") {
				got[e.Attrs["line"]] = e.Attrs["argc"]
			}
		}
		want := map[string]string{
			lineOf(t, pyrLambdaSrc, "sort_by(xs, lambda p"):   "2",
			lineOf(t, pyrLambdaSrc, "sort_by(xs, key=lambda"): "3",
			lineOf(t, pyrLambdaSrc, "sort_by(lambda m"):       "2",
		}
		if len(got) != len(want) {
			t.Errorf("%s: calls from use to sort_by by line %v; want %v", b.how, got, want)
			continue
		}
		for line, argc := range want {
			if got[line] != argc {
				t.Errorf("%s: the call on line %s has argc %q; want %s", b.how, line, got[line], argc)
			}
		}
	}
}

// TestPYR_TheReaderIsPyliteFour (card PY-R): the reader states pylite/4 as
// the producer an index header names, and every record it writes on Extract
// and on Overlay has that provenance, so an index the third version wrote,
// whose sigs and counts split a lambda, is read as holding no py records
// until it is built again. Break: stay on version 3.
func TestPYR_TheReaderIsPyliteFour(t *testing.T) {
	if got := pyrResolver.Producer(); got != "pylite/4" {
		t.Errorf("Producer %q; want pylite/4", got)
	}
	for _, b := range pyrBoth(t, "r/lib/m.py", pyrEndSrc) {
		n := 0
		for _, r := range b.recs {
			var p graph.Prov
			switch {
			case r.Node != nil:
				p = r.Node.Prov
			case r.Edge != nil:
				p = r.Edge.Prov
			default:
				continue
			}
			n++
			if p.Extractor != "pylite" || p.Version != "4" {
				t.Errorf("%s: a record from %s/%s; want pylite/4", b.how, p.Extractor, p.Version)
				break
			}
		}
		if n == 0 {
			t.Errorf("%s: no records; want the file's", b.how)
		}
	}
}

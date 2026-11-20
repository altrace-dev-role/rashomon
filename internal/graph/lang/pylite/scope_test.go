package pylite_test

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// scopeUnit is the unit the scope tests read their files in.
const scopeUnit = "services/auth"

// scopeOverlay is the records of src read as the file rel of the scope unit.
func scopeOverlay(t *testing.T, rel, src string) []graph.Record {
	t.Helper()
	recs, err := graphtest.Resolver().Overlay(rel, []byte(src), graph.RevCommit(graphtest.Commit), 1)
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

// symID is the id of the symbol of kind symkind, qualified name, in the
// scope unit.
func symID(symkind, qualified string) graph.ID {
	return graph.SymID("py", scopeUnit, symkind, qualified)
}

// hasNode reports whether recs hold a node whose id is id.
func hasNode(recs []graph.Record, id graph.ID) bool {
	for _, r := range recs {
		if r.Node != nil && r.Node.ID == id {
			return true
		}
	}
	return false
}

// hasEdge reports whether recs hold the edge src pred dst.
func hasEdge(recs []graph.Record, src graph.ID, pred graph.Pred, dst graph.ID) bool {
	for _, r := range recs {
		if r.Edge != nil && r.Edge.Src == src && r.Edge.Pred == pred && r.Edge.Dst == dst {
			return true
		}
	}
	return false
}

// defBodySrc has a def inside a def, a call after it in the outer body, and
// an import indented in that body.
const defBodySrc = `def helper():
    return 1


def wrap(f):
    return f


def outer():
    def inner():
        helper()
    import orgs
    return wrap(inner)
`

// TestLANGREG1_ADefInsideADefIsItsBody: a def nested in a def's body is no
// symbol of the module. Its lines are the outer def's body, so the calls in
// them are the outer def's, and so is a call after it; an import indented in
// that body is not the module's import. Break: test for a def before the
// body rule (inner gets an id, helper is inner's call, the call of wrap is
// lost and the import of orgs is the module's).
func TestLANGREG1_ADefInsideADefIsItsBody(t *testing.T) {
	rel := "services/auth/auth/nest.py"
	recs := scopeOverlay(t, rel, defBodySrc)
	if id := symID("func", "auth.nest.inner"); hasNode(recs, id) {
		t.Errorf("the nested def has the symbol %s; want none", id)
	}
	outer := symID("func", "auth.nest.outer")
	for _, callee := range []string{"helper", "wrap"} {
		if !hasEdge(recs, outer, graph.Calls, symID("func", "auth.nest."+callee)) {
			t.Errorf("outer has no call of %s; want the call its body makes", callee)
		}
	}
	if hasEdge(recs, graph.FileID(rel), graph.Imports, graph.SymID("py", "services/orgs", "mod", "orgs")) {
		t.Errorf("the import indented in the body of outer is an import of the module; want none")
	}
}

// nodeOf is the node id in recs, and whether recs hold it.
func nodeOf(recs []graph.Record, id graph.ID) (graph.Node, bool) {
	for _, r := range recs {
		if r.Node != nil && r.Node.ID == id {
			return *r.Node, true
		}
	}
	return graph.Node{}, false
}

// callsFrom is the destination of each Calls edge from src in recs, in
// record order.
func callsFrom(recs []graph.Record, src graph.ID) []graph.ID {
	var out []graph.ID
	for _, r := range recs {
		if r.Edge != nil && r.Edge.Src == src && r.Edge.Pred == graph.Calls {
			out = append(out, r.Edge.Dst)
		}
	}
	return out
}

// classSrc has a class whose base the file imports, with two methods and a
// classmethod, and a class nested in it whose bases span lines, with a
// method of its own; then module defs that share a method's name (get) and
// a name the base may define (close).
const classSrc = `from auth.base import Base


class Store(Base):
    def get(self, key):
        get(key)
        self.close()
        return self.load(key)

    def load(self, key):
        return "SELECT id FROM accounts"

    @classmethod
    def make(cls):
        return cls.load(None)

    class Entry(
        Base,
    ):
        def size(self):
            return self.size()


def get(key):
    return load(key)


def close():
    return None
`

// TestLANGREG1_AMethodIsKeyedByItsClass: a def in a class body is a method,
// keyed by its class path: symkind method, qualified name the module, the
// classes and its own name, so it shares no id with a module def of its
// name. The file declares each method, and the SQL in a method's body is
// the method's read. A class is no node yet: no node of kind class is
// written, the limit a card of class nodes turns. Break: key a method as a
// module def (func ids for load, make and size, one id for the method get
// and the module get), or end a class at the close of bases that span
// lines.
func TestLANGREG1_AMethodIsKeyedByItsClass(t *testing.T) {
	rel := "services/auth/auth/store.py"
	recs := scopeOverlay(t, rel, classSrc)
	for _, w := range []struct{ symkind, qualified string }{
		{"method", "auth.store.Store.get"},
		{"method", "auth.store.Store.load"},
		{"method", "auth.store.Store.make"},
		{"method", "auth.store.Store.Entry.size"},
		{"func", "auth.store.get"},
		{"func", "auth.store.close"},
	} {
		id := symID(w.symkind, w.qualified)
		n, ok := nodeOf(recs, id)
		declared := hasEdge(recs, graph.FileID(rel), graph.Declares, id)
		if !ok || n.Attrs["symkind"] != w.symkind || !declared {
			t.Errorf("%s: node %v, symkind %q, declared %v; want a %s the file declares", id, ok, n.Attrs["symkind"], declared, w.symkind)
		}
	}
	for _, name := range []string{"load", "make", "size"} {
		if id := symID("func", "auth.store."+name); hasNode(recs, id) {
			t.Errorf("%s is a node; want none, %s is a method", id, name)
		}
	}
	for _, r := range recs {
		if r.Node != nil && r.Node.Attrs["symkind"] == "class" {
			t.Errorf("%s is a node of kind class; want none, a class is no node yet", r.Node.ID)
		}
	}
	if !hasEdge(recs, symID("method", "auth.store.Store.load"), graph.Reads, graph.TableID("accounts")) {
		t.Errorf("Store.load does not read accounts; want the read of the SQL in its body")
	}
}

// TestLANGREG1_ASelfCallResolvesInItsOwnClass: in a method, self.name() or
// cls.name() calls the method name of the caller's own class when that
// class defines it: Store.get calls Store.load, the classmethod Store.make
// calls it by cls, and Store.Entry.size calls itself. A name the class does
// not define, self.close, may be its base's: no edge, a named gap, and never
// the module's close. Break: leave self calls unresolved, look the name up
// in the outer class, or fall back to the module's defs.
func TestLANGREG1_ASelfCallResolvesInItsOwnClass(t *testing.T) {
	recs := scopeOverlay(t, "services/auth/auth/store.py", classSrc)
	for _, w := range []struct{ caller, callee string }{
		{"Store.get", "Store.load"},
		{"Store.make", "Store.load"},
		{"Store.Entry.size", "Store.Entry.size"},
	} {
		if !hasEdge(recs, symID("method", "auth.store."+w.caller), graph.Calls, symID("method", "auth.store."+w.callee)) {
			t.Errorf("%s has no call of %s; want the self call resolved in its own class", w.caller, w.callee)
		}
	}
	for _, dst := range callsFrom(recs, symID("method", "auth.store.Store.get")) {
		if strings.HasSuffix(string(dst), ".close") {
			t.Errorf("Store.get calls %s; want no edge for self.close, which the base may define", dst)
		}
	}
}

// TestLANGREG1_ABareNameIsAModuleDef: a bare name called in a method is
// looked up in the module, not in the class (a class body is no enclosing
// scope of its methods): the get(key) of Store.get calls the module's get.
// A bare name no module def has is no edge: the module's get calls load,
// which only Store defines. Break: put methods among the module's names,
// and the module's get calls the method load.
func TestLANGREG1_ABareNameIsAModuleDef(t *testing.T) {
	recs := scopeOverlay(t, "services/auth/auth/store.py", classSrc)
	get := symID("func", "auth.store.get")
	if !hasEdge(recs, symID("method", "auth.store.Store.get"), graph.Calls, get) {
		t.Errorf("Store.get has no call of the module's get; want its bare get resolved in the module")
	}
	if got := callsFrom(recs, get); len(got) > 0 {
		t.Errorf("the module's get calls %v; want none, no module def is named load", got)
	}
}

// capWordsSrc imports a class, a function and a constant from the orgs
// unit, and calls the class.
const capWordsSrc = `from orgs.models import OrgSettings
from orgs.settings import get_org_settings, TIMEOUT


def build():
    return OrgSettings()
`

// TestLANGREG1_ACapWordsNameGuessesAClass: a name imported from a module of
// a known unit is guessed by its case, as PEP 8 names things: CapWords a
// class, all upper case a constant, any other a function. The guess keys
// both the reference the import writes and a call of the name, which makes
// an instance. Break: guess a function for every name not all upper case,
// as the first reader did.
func TestLANGREG1_ACapWordsNameGuessesAClass(t *testing.T) {
	rel := "services/auth/auth/use.py"
	recs := scopeOverlay(t, rel, capWordsSrc)
	class := graph.SymID("py", "services/orgs", "class", "orgs.models.OrgSettings")
	for _, id := range []graph.ID{
		class,
		graph.SymID("py", "services/orgs", "func", "orgs.settings.get_org_settings"),
		graph.SymID("py", "services/orgs", "const", "orgs.settings.TIMEOUT"),
	} {
		if !hasEdge(recs, graph.FileID(rel), graph.References, id) {
			t.Errorf("the file has no reference of %s; want the guess its case gives", id)
		}
	}
	if !hasEdge(recs, symID("func", "auth.use.build"), graph.Calls, class) {
		t.Errorf("build has no call of %s; want the call of the class", class)
	}
}

// TestLANGREG1_TheReaderIsPyliteTwo: the reader that keys a method by its
// class and guesses a CapWords name a class is pylite version 2, the
// version an index names as the producer of its py records. An index an
// older reader made keeps its old ids until it is built again. Break:
// leave the version at 1.
func TestLANGREG1_TheReaderIsPyliteTwo(t *testing.T) {
	rel := "services/auth/auth/use.py"
	n, ok := nodeOf(scopeOverlay(t, rel, capWordsSrc), graph.FileID(rel))
	if !ok || n.Prov.Extractor != "pylite" || n.Prov.Version != "2" {
		t.Errorf("file node %v, reader %s/%s; want pylite/2", ok, n.Prov.Extractor, n.Prov.Version)
	}
}

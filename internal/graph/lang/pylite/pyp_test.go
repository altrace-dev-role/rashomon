package pylite_test

import (
	"slices"
	"sort"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The reader of card PY-P: a call whose name directly follows the open paren
// of another call is read. The pattern a call matches takes one byte before
// the name, and that byte may be the paren that ended the match before, so
// the search for the next call starts at that paren, not after it.

// pypSrc has a call right after the paren of another call in each shape the
// card names: the print line of main, three deep, a reserved word's paren
// (print, return), a method call as the outer and as the inner call; and two
// that read before the card, a paren with no name before it and a space
// after the paren; and a call on a continuation line inside another call,
// which stays unread in 1.0.
const pypSrc = `def add(a, b=0):
    return a + b


def total():
    return add(4, 5)


def put(x, y=None):
    return x


def main():
    print(add(2, 3), total())


def deep():
    put(add(total()))


def bare():
    x = (add(1) + 2)
    return x


def spaced():
    put( add(1))


def word():
    return(add(1, 2))


def cont():
    put(1,
        add(2))


class Store:
    def get(self, k):
        return k

    def keep(self, v):
        return v

    def outer(self):
        self.keep(add(1, 2))

    def inner(self):
        put(self.get(1), 2)
`

// TestPYP_ACallRightAfterAnOpenParenIsRead (card PY-P): on Extract and on
// Overlay, the Calls edges of pypSrc are these and no others, each with its
// argc and line written here by hand. main calls add with 2 arguments and
// total with none, print being a reserved word that is no edge; deep calls
// put, add and total; word calls add past the paren of return; Store.outer
// calls Store.keep and add; Store.inner calls put and Store.get. The outer
// call's argc is the same as before the card. bare and spaced read add
// before the card too. cont calls put with 2 arguments and add on the line
// after is not read: a call on a continuation line inside another call is
// the limit that stays in 1.0. Break: start the search for the next call
// after the paren that ended the match before, and every edge to a call
// right after that paren is lost (main to add, deep to add, word to add,
// Store.outer to add, Store.inner to Store.get).
func TestPYP_ACallRightAfterAnOpenParenIsRead(t *testing.T) {
	fn := func(name string) graph.ID { return pyrSym("func", name) }
	meth := func(name string) graph.ID { return pyrSym("method", name) }
	at := func(needle string) string { return lineOf(t, pypSrc, needle) }
	type call struct {
		src, dst   graph.ID
		argc, line string
	}
	want := []call{
		{fn("total"), fn("add"), "2", at("return add(4, 5)")},
		{fn("main"), fn("add"), "2", at("print(add(2, 3), total())")},
		{fn("main"), fn("total"), "0", at("print(add(2, 3), total())")},
		{fn("deep"), fn("put"), "1", at("put(add(total()))")},
		{fn("deep"), fn("add"), "1", at("put(add(total()))")},
		{fn("deep"), fn("total"), "0", at("put(add(total()))")},
		{fn("bare"), fn("add"), "1", at("x = (add(1) + 2)")},
		{fn("spaced"), fn("put"), "1", at("put( add(1))")},
		{fn("spaced"), fn("add"), "1", at("put( add(1))")},
		{fn("word"), fn("add"), "2", at("return(add(1, 2))")},
		{fn("cont"), fn("put"), "2", at("put(1,")},
		{meth("Store.outer"), meth("Store.keep"), "1", at("self.keep(add(1, 2))")},
		{meth("Store.outer"), fn("add"), "2", at("self.keep(add(1, 2))")},
		{meth("Store.inner"), fn("put"), "2", at("put(self.get(1), 2)")},
		{meth("Store.inner"), meth("Store.get"), "1", at("put(self.get(1), 2)")},
	}
	show := func(c call) string {
		return string(c.src) + " calls " + string(c.dst) + " argc " + c.argc + " line " + c.line
	}
	var wantRows []string
	for _, c := range want {
		wantRows = append(wantRows, show(c))
	}
	sort.Strings(wantRows)
	for _, b := range pyrBoth(t, "r/lib/m.py", pypSrc) {
		var got []string
		for _, r := range b.recs {
			if e := r.Edge; e != nil && e.Pred == graph.Calls {
				got = append(got, show(call{e.Src, e.Dst, e.Attrs["argc"], e.Attrs["line"]}))
			}
		}
		sort.Strings(got)
		if slices.Equal(got, wantRows) {
			continue
		}
		for _, w := range wantRows {
			if !slices.Contains(got, w) {
				t.Errorf("%s: no edge %s", b.how, w)
			}
		}
		for _, g := range got {
			if !slices.Contains(wantRows, g) {
				t.Errorf("%s: edge %s; want none such", b.how, g)
			}
		}
		// The two loops above miss a row that is read twice; this line fails
		// any difference between the sorted lists.
		t.Errorf("%s: %d Calls edges, want the %d above", b.how, len(got), len(wantRows))
	}
}

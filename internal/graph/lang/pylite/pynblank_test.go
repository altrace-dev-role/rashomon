package pylite_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The pairs below are Python sources that differ only inside string
// literals or comments, each side written by hand. The left holds plain
// words; the right holds what a line reader takes for code: a parameter
// split, a bracket, a constant, a def, an import, a call. Every pair sits
// between pynHead, which declares the helper the right sides call, and
// pynTail, which declares a last function, so a string or comment read as
// running on past its close loses the tail. The two sides of a pair have
// their line ends at the same places.
const (
	pynHead = "def helper(x):\n    return x\n\n\n"
	pynTail = "\n\ndef tail():\n    return 0\n"
	pynRel  = "u/app/s.py"
)

var pynPairs = []struct{ name, left, right string }{
	{"01 a string default holding commas with a bare word",
		`def f(a="alpha  zzcanary  omega", b=1):
    return helper(a)
`,
		`def f(a="alpha, zzcanary ,omega", b=1):
    return helper(a)
`},
	{"02 a string default holding an opening bracket",
		`def f(a="x", b=1):
    return helper(a)
`,
		`def f(a="(", b=1):
    return helper(a)
`},
	{"03 a module triple string holding a constant, a def, an import and an SQL line",
		`DOC = """
nothing here
nothing here
nothing here
nothing here
"""
`,
		`DOC = """
API_KEY = 1
def fake(x):
from app.db import save
SELECT id FROM users
"""
`},
	{"04 a body string holding a name and a bracket",
		`def g():
    log = "xxxxxxxxx"
    return log
`,
		`def g():
    log = "helper(1)"
    return log
`},
	{"05 a body comment holding a name and a bracket",
		`def g():
    x = 1  # xxxxxxxxx
    return x
`,
		`def g():
    x = 1  # helper(1)
    return x
`},
	{"06 a raw string ending in a backslash before its quote",
		`PATTERN = r"""xx"""
`,
		`PATTERN = r"""\\"""
`},
	{"07 an f-string holding a call",
		`def g(x):
    return f"{xxxxxxxxx}"
`,
		`def g(x):
    return f"{helper(x)}"
`},
	{"08 a string in one quote holding the other",
		`NAME = """itss
ok"""
`,
		`NAME = """it's
ok"""
`},
	{"09 a string holding a hash",
		`TEXT = """a x b"""
`,
		`TEXT = """a # b"""
`},
	{"10 a comment holding one quote with a def on the next line",
		`# itxs here
def after():
    return 0
`,
		`# it's here
def after():
    return 0
`},
	{"11 a bytes triple string holding a def line",
		`B = b"""
nothing here
"""
`,
		`B = b"""
def inner(x):
"""
`},
	{"12 a one-quote string carried by a backslash holding a constant line",
		`S = 'abc\
xxxxxxxxx'
`,
		`S = 'abc\
LIMIT = 5'
`},
	{"13 a string of the same quote nested in an f-string field",
		`def g(d):
    return f"{d["xxxxxxxxx"]}"
`,
		`def g(d):
    return f"{d["helper(1)"]}"
`},
	{"14 a quote as the fill of an f-string format spec",
		`W = f"""{1:x^10}"""
`,
		`W = f"""{1:'^10}"""
`},
}

// pynRead reads src as the one file of a unit, through Extract from a temp
// root and through Overlay, and returns both record lists with both
// refusals as text.
func pynRead(t *testing.T, src string) ([]graph.Record, []graph.Record, [2]string) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, pynRel, src)
	ext, extErr := pynResolver.Extract(root, []string{pynRel}, relRev)
	ov, ovErr := pynResolver.Overlay(pynRel, []byte(src), relRev, 1)
	return ext, ov, [2]string{fmt.Sprint(extErr), fmt.Sprint(ovErr)}
}

// pynText is recs as sorted text lines without the file node's digest, the
// one thing a change inside a string may move. Sorted, because the reader
// writes some records in map order.
func pynText(recs []graph.Record) []string {
	var out []string
	for _, r := range recs {
		switch {
		case r.Node != nil:
			n := *r.Node
			if n.Kind == "file" {
				n.Attrs = maps.Clone(n.Attrs)
				delete(n.Attrs, "digest")
			}
			out = append(out, fmt.Sprintf("node %+v", n))
		case r.Edge != nil:
			out = append(out, fmt.Sprintf("edge %+v", *r.Edge))
		}
	}
	slices.Sort(out)
	return out
}

// pynOnly is the lines of a that b does not hold.
func pynOnly(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

// TestPYN_StringsAndCommentsChangeNothing: two sources that differ only
// inside string literals or comments read the same, through Extract and
// through Overlay: the same records but the file node's digest, and the same
// refusal or none, and both sides declare the tail. Break: take the blanking
// pass out of parse.
func TestPYN_StringsAndCommentsChangeNothing(t *testing.T) {
	tail := graph.ID("sym:py:u:func:app.s.tail")
	for _, p := range pynPairs {
		t.Run(p.name, func(t *testing.T) {
			if strings.Count(p.left, "\n") != strings.Count(p.right, "\n") {
				t.Fatalf("the sides hold %d and %d line ends; a pair keeps them", strings.Count(p.left, "\n"), strings.Count(p.right, "\n"))
			}
			lx, lo, lerr := pynRead(t, pynHead+p.left+pynTail)
			rx, ro, rerr := pynRead(t, pynHead+p.right+pynTail)
			if lerr != rerr {
				t.Errorf("refusals differ: left %q, right %q", lerr, rerr)
			}
			for _, s := range []struct {
				via         string
				left, right []graph.Record
			}{{"Extract", lx, rx}, {"Overlay", lo, ro}} {
				l, r := pynText(s.left), pynText(s.right)
				if !slices.Equal(l, r) {
					t.Errorf("%s: %d records left, %d right\nleft only:\n%s\nright only:\n%s", s.via, len(l), len(r),
						strings.Join(pynOnly(l, r), "\n"), strings.Join(pynOnly(r, l), "\n"))
				}
				if !hasNode(s.left, tail) || !hasNode(s.right, tail) {
					t.Errorf("%s: tail declared left %v, right %v; want both", s.via, hasNode(s.left, tail), hasNode(s.right, tail))
				}
			}
		})
	}
}

// TestPYN_AStringNeverClosedEndsAtItsLineOrAtTheEnd pins the pass on a
// string that does not close, which Python refuses and the reader reads: a
// string in one quote ends at its line end, so the next line is read; a
// triple-quoted one runs to the end of the file, so nothing after it is.
// Neither is a refusal. Break: end a triple-quoted string at its line end
// (the def after it is then read), or run a one-quote string on to the end
// of the file (the def after it is then lost).
func TestPYN_AStringNeverClosedEndsAtItsLineOrAtTheEnd(t *testing.T) {
	x := graph.ID("sym:py:u:const:app.s.X")
	after := graph.ID("sym:py:u:func:app.s.after")
	for _, c := range []struct {
		name, body string
		after      bool
	}{
		{"one quote", "X = \"never closed\ndef after():\n    return 0\n", true},
		{"three quotes", "X = \"\"\"never closed\ndef after():\n    return 0\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ext, ov, errs := pynRead(t, pynHead+c.body)
			if errs != [2]string{"<nil>", "<nil>"} {
				t.Errorf("refused: %q; want no refusal", errs)
			}
			for _, s := range []struct {
				via  string
				recs []graph.Record
			}{{"Extract", ext}, {"Overlay", ov}} {
				if !hasNode(s.recs, x) {
					t.Errorf("%s: X not declared; want it", s.via)
				}
				if got := hasNode(s.recs, after); got != c.after {
					t.Errorf("%s: after declared %v; want %v", s.via, got, c.after)
				}
			}
		})
	}
}

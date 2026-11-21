package pylite_test

import (
	"errors"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// defHeaderRes reads one unit, src/pkg, so src/pkg/a.py is module pkg.a.
var defHeaderRes = pylite.Resolver{
	Units:   []pylite.Unit{{Path: "src/pkg", ModRoot: "src"}},
	Modules: map[string]string{"pkg": "src/pkg"},
}

// The file of every def header row: helper on lines 1 and 2, the row's
// header from line 4, its body, then a def after it that calls helper and a
// constant, so a row also shows the lines after the def read as their own.
const (
	defHeaderRel  = "src/pkg/a.py"
	defHeaderHead = "def helper(x):\n    return x\n\n"
	defHeaderTail = "\n\ndef after(b):\n    return helper(b)\n\nLIMIT = 3\n"
	defHeaderMod  = "sym:py:src/pkg:mod:pkg.a"
	defHeaderHelp = "sym:py:src/pkg:func:pkg.a.helper"
	defHeaderAft  = "sym:py:src/pkg:func:pkg.a.after"
	defHeaderLim  = "sym:py:src/pkg:const:pkg.a.LIMIT"
	defHeaderF    = "sym:py:src/pkg:func:pkg.a.f"
	defHeaderKM   = "sym:py:src/pkg:method:pkg.a.K.m"
	defHeaderBody = "    return helper(a)\n"
	defNoColon    = "a def with no colon after its parameters"
)

// defHeaderRead extracts src as src/pkg/a.py under a fresh root.
func defHeaderRead(t *testing.T, src string) ([]graph.Record, error) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, defHeaderRel, src)
	return defHeaderRes.Extract(root, []string{defHeaderRel}, relRev)
}

// defHeaderNode is the attributes of the node id in recs, or nil.
func defHeaderNode(recs []graph.Record, id string) map[string]string {
	for _, r := range recs {
		if r.Node != nil && string(r.Node.ID) == id {
			return r.Node.Attrs
		}
	}
	return nil
}

// defHeaderRow is one def header: form is the header as written over its
// lines and twin the same header on one line, which Python reads the same;
// a row with no twin is a header Python refuses.
type defHeaderRow struct {
	name, form, twin, body string
	sym, sig, min, max     string
	line                   int
	calls                  bool // the def's body calls helper
}

// fHeader is a row for def f(a, b=1) on line 4 with the shared body.
func fHeader(name, form, twin string) defHeaderRow {
	return defHeaderRow{name: name, form: form, twin: twin, body: defHeaderBody,
		sym: defHeaderF, sig: "(a, b)", min: "1", max: "2", line: 4, calls: true}
}

// TestDefHeader_AHeaderOpenPastItsParametersIsRead: Python joins a def
// header's physical lines into one logical line while a bracket is open or
// a backslash ends the code, so a return annotation may run over lines after
// the parameter list closes, and the colon that ends the header sits on a
// later line. Each such header is read beside its one-line twin: no refusal,
// the def with its parameters, its line and its end (the last line of its
// body, so the header's lines are no body lines), the calls of its body and
// of the def after it, and the overlay's declarations and calls equal to the
// extract's. A colon inside a bracket ends no header. A body on the header's
// own line is not read, as for a one-line header. The refused rows are
// headers Python refuses: no colon after the parameters, past a backslash,
// or after an annotation's bracket; each is refused at the def's line with
// the words a header with no colon has always had.
func TestDefHeader_AHeaderOpenPastItsParametersIsRead(t *testing.T) {
	inline := fHeader("a body on the colon's line",
		"def f(a, b=1) -> tuple[\n    int]: return helper(a)\n",
		"def f(a, b=1) -> tuple[int]: return helper(a)\n")
	inline.body, inline.calls = "", false
	rows := []defHeaderRow{
		fHeader("a subscript annotation over lines",
			"def f(a, b=1) -> tuple[\n    int,\n    str,\n]:\n",
			"def f(a, b=1) -> tuple[int, str]:\n"),
		fHeader("an async def with a subscript annotation over lines",
			"async def f(a, b=1) -> dict[\n    str, int\n]:\n",
			"async def f(a, b=1) -> dict[str, int]:\n"),
		fHeader("a parenthesized annotation over lines",
			"def f(a, b=1) -> (\n    int\n):\n",
			"def f(a, b=1) -> (int):\n"),
		fHeader("a dict display whose colon is inside its brace",
			"def f(a, b=1) -> {\n    1: int,\n}:\n",
			"def f(a, b=1) -> {1: int,}:\n"),
		fHeader("nested brackets over lines",
			"def f(a, b=1) -> dict[\n    str,\n    list[\n        int\n    ],\n]:\n",
			"def f(a, b=1) -> dict[str, list[int],]:\n"),
		fHeader("a blank line and a comment line inside the bracket",
			"def f(a, b=1) -> tuple[\n\n    # one: two\n    int,\n]:\n",
			"def f(a, b=1) -> tuple[int,]:\n"),
		fHeader("the close and the colon on a line of the annotation",
			"def f(a, b=1) -> tuple[\n        int]:\n",
			"def f(a, b=1) -> tuple[int]:\n"),
		fHeader("a backslash after the parameters",
			"def f(a, b=1) \\\n        -> int:\n",
			"def f(a, b=1) -> int:\n"),
		fHeader("a backslash after the arrow",
			"def f(a, b=1) -> \\\n        int:\n",
			"def f(a, b=1) -> int:\n"),
		fHeader("a backslash before the colon",
			"def f(a, b=1) -> int \\\n        :\n",
			"def f(a, b=1) -> int:\n"),
		fHeader("a backslash and then a bracket",
			"def f(a, b=1) \\\n        -> tuple[\n    int,\n]:\n",
			"def f(a, b=1) -> tuple[int,]:\n"),
		{name: "a method whose annotation runs over lines",
			form: "class K:\n    def m(self, a, b=1) -> tuple[\n        int,\n    ]:\n",
			twin: "class K:\n    def m(self, a, b=1) -> tuple[int]:\n",
			body: "        return helper(a)\n",
			sym:  defHeaderKM, sig: "(self, a, b)", min: "1", max: "2", line: 5, calls: true},
		inline,
		fHeader("pinned: parameters over lines, the colon on their close line",
			"def f(\n    a,\n    b=1,\n) -> int:\n",
			"def f(a, b=1,) -> int:\n"),
		fHeader("pinned: a colon in a bracket on the close line, the bracket open past it",
			"def f(a, b=1) -> Literal[{1: 2},\n    3]:\n",
			"def f(a, b=1) -> Literal[{1: 2}, 3]:\n"),
		fHeader("refused: a header with no colon", "def f(a, b=1)\n", ""),
		fHeader("refused: a backslash and then no colon", "def f(a, b=1) \\\n        -> int\n", ""),
		fHeader("refused: an annotation's bracket closes with no colon after it", "def f(a, b=1) -> tuple[\n    int,\n]\n", ""),
	}
	file := string(graph.FileID(defHeaderRel))
	for _, c := range rows {
		t.Run(c.name, func(t *testing.T) {
			if c.twin == "" {
				src := defHeaderHead + c.form + c.body + defHeaderTail
				_, err := defHeaderRead(t, src)
				var refused graph.Refusals
				want := graph.Refusals{{Path: defHeaderRel, Line: c.line, Reason: defNoColon}}
				if !errors.As(err, &refused) || !reflect.DeepEqual(refused, want) {
					t.Errorf("extract error %v; want %+v", err, want)
				}
				if _, err := defHeaderRes.Overlay(defHeaderRel, []byte(src), relRev, 1); err == nil || !strings.Contains(err.Error(), defNoColon) {
					t.Errorf("overlay error %v; want one holding %q", err, defNoColon)
				}
				return
			}
			declares := []string{defHeaderMod, defHeaderHelp, c.sym, defHeaderAft, defHeaderLim}
			for i, d := range declares {
				declares[i] = file + " -" + string(graph.Declares) + "-> " + d
			}
			sort.Strings(declares)
			calls := []string{defHeaderAft + " -" + string(graph.Calls) + "-> " + defHeaderHelp}
			if c.calls {
				calls = append(calls, c.sym+" -"+string(graph.Calls)+"-> "+defHeaderHelp)
			}
			sort.Strings(calls)
			for _, v := range []struct{ label, header string }{{"form", c.form}, {"twin", c.twin}} {
				src := defHeaderHead + v.header + c.body + defHeaderTail
				recs, err := defHeaderRead(t, src)
				if err != nil {
					t.Errorf("%s: refused: %v", v.label, err)
					continue
				}
				if got := edgesOf(recs, graph.Declares); !slices.Equal(got, declares) {
					t.Errorf("%s: declares %q; want %q", v.label, got, declares)
				}
				if got := edgesOf(recs, graph.Calls); !slices.Equal(got, calls) {
					t.Errorf("%s: calls %q; want %q", v.label, got, calls)
				}
				end := strconv.Itoa(3 + strings.Count(v.header+c.body, "\n"))
				want := map[string]string{"sig": c.sig, "arity_min": c.min, "arity_max": c.max, "line": strconv.Itoa(c.line), "end": end}
				attrs := defHeaderNode(recs, c.sym)
				for k, w := range want {
					if attrs[k] != w {
						t.Errorf("%s: %s %s %q; want %q", v.label, c.sym, k, attrs[k], w)
					}
				}
				ov, err := defHeaderRes.Overlay(defHeaderRel, []byte(src), relRev, 1)
				if err != nil {
					t.Errorf("%s: overlay refused: %v", v.label, err)
					continue
				}
				for _, p := range []graph.Pred{graph.Declares, graph.Calls} {
					if got, want := edgesOf(ov, p), edgesOf(recs, p); !slices.Equal(got, want) {
						t.Errorf("%s: overlay %s edges %q; extract %q", v.label, p, got, want)
					}
				}
			}
		})
	}
}

// TestDefHeader_AColonOnTheCloseLineEndsTheHeaderAsBefore: when the line the
// parameters close on holds a colon, the header ends on that line, as it
// always has, even when the colon is inside a bracket that stays open past
// it; the line after is read as a body line, so the call on it is the def's.
// Python would read that line as part of the annotation. The reader keeps
// the old reading so that every file it read before gives the same records
// under the same version; only a header it refused before is read anew.
func TestDefHeader_AColonOnTheCloseLineEndsTheHeaderAsBefore(t *testing.T) {
	src := defHeaderHead + "def f(a, b=1) -> Literal[{1: 2},\n    helper(3)]:\n" + defHeaderBody + defHeaderTail
	recs, err := defHeaderRead(t, src)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, r := range recs {
		if e := r.Edge; e != nil && e.Pred == graph.Calls && string(e.Src) == defHeaderF {
			lines = append(lines, e.Attrs["line"])
		}
	}
	sort.Strings(lines)
	if want := []string{"5", "6"}; !slices.Equal(lines, want) {
		t.Errorf("calls of f at lines %q; want %q", lines, want)
	}
	if got := defHeaderNode(recs, defHeaderF)["end"]; got != "6" {
		t.Errorf("f's end %q; want 6", got)
	}
}

package verify_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// fxUser is a user record of prompt id, a prompt or, with a tool id, the
// tool_result that answers it.
func fxUser(id, tool string, side bool) string {
	if tool == "" {
		return fmt.Sprintf(`{"type":"user","promptId":%q,"origin":{"kind":"human"},"message":{"content":"go"}}`, id)
	}
	return fmt.Sprintf(`{"type":"user","promptId":%q,"isSidechain":%t,"message":{"content":[{"type":"tool_result","tool_use_id":%q}]}}`, id, side, tool)
}

// fxCall is an assistant record calling tool name with id tool.
func fxCall(tool, name string, side bool) string {
	return fmt.Sprintf(`{"type":"assistant","isSidechain":%t,"message":{"content":[{"type":"tool_use","id":%q,"name":%q}]}}`, side, tool, name)
}

const fxStop = `{"type":"system","subtype":"stop_hook_summary"}`

// TestTurnReplay_OneTurnPerClass runs the replay over a synthetic session,
// one turn per class, each closed by a Stop and followed by the prompt its
// last-turn note is delivered at: an Edit after a Read (a file tool; the
// rule acts), a Bash call (Bash only; the rule does not act), a Read alone
// (no act, a call held; the rule acts on an execute-class call, a residual
// note), a sidechain's Edit (never the main agent's: no act, no call held)
// and an Edit whose Stop record names another prompt (counted apart, never
// scored). Each lands in its class and cell. Seqs and times are as the
// product writes them: a Stop's verify record takes the seq after its point,
// and the note, built at the next prompt, takes the point as its after_seq.
// The prompt that takes pB's note ends text-only, so its Stop shares the
// point and is written after the note. Break: count a sidechain's call, drop
// the tool_result join, read Bash as no act, score a mismatch, join the
// Stop by seq at or before after_seq, or drop the Stop's time bound.
func TestTurnReplay_OneTurnPerClass(t *testing.T) {
	type turn struct {
		id    string
		calls [][3]string // tool id, name, "side" for a sidechain's
		rule  string      // the prompt id the Stop record names
		class tClass
		call  bool
		quiet bool // the prompt that takes the note ends text-only
	}
	turns := []turn{
		{"pA", [][3]string{{"a1", "Read", ""}, {"a2", "Edit", ""}}, "pA", tFileTool, true, false},
		{"pB", [][3]string{{"b1", "Bash", ""}}, "pB", tBashOnly, true, true},
		{"pC", [][3]string{{"c1", "Read", ""}}, "pC", tNoAct, true, false},
		{"pD", [][3]string{{"d1", "Edit", "side"}}, "pD", tNoAct, false, false},
		{"pE", [][3]string{{"e1", "Edit", ""}}, "pZ", tFileTool, true, false},
	}
	var src []string
	var vs []rVerify
	var notes []rNote
	nexts := make([]string, len(turns))
	for i, tn := range turns {
		src = append(src, fxUser(tn.id, "", false))
		for _, c := range tn.calls {
			side := c[2] == "side"
			src = append(src, fxCall(c[0], c[1], side), fxUser(tn.id, c[0], side))
		}
		src = append(src, fxStop)
		nexts[i] = "q" + tn.id[1:]
		src = append(src, fxUser(nexts[i], "", false))
		point, at := int64(10*(i+1)+5), int64(1000*(i+1))
		vs = append(vs, rVerify{Type: "verify", Seq: point + 1, PromptID: &turns[i].rule, PointSeq: point, RecordedAt: at})
		notes = append(notes, rNote{Type: "steer", Pattern: "claim-vs-record", Key: "last-turn", PromptID: nexts[i], AfterSeq: point, AtMS: at + 500})
		if tn.quiet {
			src = append(src, fxStop)
			vs = append(vs, rVerify{Type: "verify", Seq: point + 2, PromptID: &nexts[i], PointSeq: point, RecordedAt: at + 700})
		}
	}
	ls, err := tRead(strings.NewReader(strings.Join(src, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	// The rule's record: pA's turn holds a record before may_write, pC's an
	// execute-class call in a project; pB, pD and pZ hold none.
	pA, pC, repo := "pA", "pC", "/repo"
	run := &store.Run{Declarations: []store.Declaration{
		{PromptID: &pA, Seq: 11, SchemaVersion: 2},
		{PromptID: &pC, Seq: 31, SchemaVersion: 5},
	}}
	run.Declarations[1].Project = &repo
	run.Declarations[1].Shape.VerbClass = shape.VerbExecute
	rows := rReplay(t, ls, run, vs, notes)
	if len(rows) != len(turns) {
		t.Fatalf("%d rows, want %d", len(rows), len(turns))
	}
	for i, tn := range turns {
		r := rows[i]
		if !r.placed || r.truth != tn.id || r.class != tn.class || r.called != tn.call {
			t.Errorf("turn %s: placed %t, truth %q, class %s, call held %t; want true, %q, %s, %t",
				tn.id, r.placed, r.truth, r.class, r.called, tn.id, tn.class, tn.call)
		}
	}
	tb := rTabulate(rows)
	want := [3][2]int{tNoAct: {1, 1}, tBashOnly: {0, 1}, tFileTool: {1, 0}}
	if tb.cells != want || tb.mismatch != 1 || tb.unplaced != 0 || tb.calledNoAct != 1 ||
		len(tb.residual) != 1 || tb.residual[shape.VerbExecute] != 1 {
		t.Errorf("table %+v; want cells %v, one mismatch, none unplaced, one no-act turn with a call, one residual counted by %s",
			tb, want, shape.VerbExecute)
	}
	rReport(t, len(rows), tb)
}

// tForeign lists what ties a Go source to code outside it and the standard
// library: an import whose path's first element holds a dot, and a call of
// a plain name the source does not resolve (a builtin aside).
func tForeign(t *testing.T, src []byte) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		if first, _, _ := strings.Cut(p, "/"); strings.Contains(first, ".") {
			bad = append(bad, "import "+p)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if id, ok := c.Fun.(*ast.Ident); ok && id.Obj == nil && types.Universe.Lookup(id.Name) == nil {
				bad = append(bad, "call "+id.Name)
			}
		}
		return true
	})
	return bad
}

// TestTurnTruth_SharesNoCode: the truth file imports the standard library
// only and calls only what it declares, so it cannot read the rule's code
// or table. Control: a source that imports a package of this module and
// calls the replay's own helper is caught twice.
func TestTurnTruth_SharesNoCode(t *testing.T) {
	src, err := os.ReadFile("instrument_truth_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if bad := tForeign(t, src); len(bad) != 0 {
		t.Errorf("the truth file reaches outside itself and the standard library: %v", bad)
	}
	ctl := "package x\nimport \"github.com/altrace-dev-role/rashomon/internal/shape\"\nfunc f() string { rDecidedBy(nil, \"\", 0); return shape.VerbRead }\n"
	if bad := tForeign(t, []byte(ctl)); len(bad) != 2 {
		t.Errorf("control: %v, want the import and the call", bad)
	}
}

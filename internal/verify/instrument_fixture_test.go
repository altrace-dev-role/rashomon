package verify_test

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/intent"
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
// note), a sidechain's Edit (never the main agent's: no act, no call held),
// an Edit whose Stop record names another prompt (counted apart, never
// scored), and a Bash call whose record may write (Bash only; the rule
// acts, counted by what counted the turn). Each lands in its class and
// cell. Seqs and times are as the product writes them: a Stop's verify
// record takes the seq after its point, and the note, built at the next
// prompt, takes the point as its after_seq. The prompt that takes pB's note
// ends text-only, so its Stop shares the point and is written after the
// note. Break: count a sidechain's call, drop the tool_result join, read
// Bash as no act, score a mismatch, join the Stop by seq at or before
// after_seq, drop the Stop's time bound, or drop the Bash-only count.
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
		{"pF", [][3]string{{"f1", "Bash", ""}}, "pF", tBashOnly, true, false},
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
	// execute-class call in a project, pF's a line that may write in a
	// project; pB, pD and pZ hold none.
	pA, pC, pF, repo := "pA", "pC", "pF", "/repo"
	run := &store.Run{Declarations: []store.Declaration{
		{PromptID: &pA, Seq: 11, SchemaVersion: 2},
		{PromptID: &pC, Seq: 31, SchemaVersion: 5},
		{PromptID: &pF, Seq: 61, SchemaVersion: 5},
	}}
	run.Declarations[1].Project = &repo
	run.Declarations[1].Shape.VerbClass = shape.VerbExecute
	run.Declarations[2].Project = &repo
	run.Declarations[2].Shape.MayWrite = true
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
	want := [3][2]int{tNoAct: {1, 1}, tBashOnly: {1, 1}, tFileTool: {1, 0}}
	if tb.cells != want || tb.mismatch != 1 || tb.unplaced != 0 || tb.calledNoAct != 1 ||
		len(tb.residual) != 1 || tb.residual[shape.VerbExecute] != 1 || len(tb.bashBy) != 1 || tb.bashBy["may-write"] != 1 {
		t.Errorf("table %+v; want cells %v, one mismatch, none unplaced, one no-act turn with a call, one residual counted by %s, one Bash-only note counted by may-write",
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

// fxBase is the stamped fixtures' clock origin, in Unix milliseconds.
const fxBase = int64(1_794_780_000_000)

// fxRec is a transcript record stamped ms after fxBase, as the host stamps
// one.
func fxRec(ms int64, rec map[string]any) string {
	rec["timestamp"] = time.UnixMilli(fxBase + ms).UTC().Format("2006-01-02T15:04:05.000Z")
	b, err := json.Marshal(rec)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// fxOpen is a prompt's own record: prompt id, its origin kind and its text.
func fxOpen(ms int64, id, kind, text string) string {
	return fxRec(ms, map[string]any{"type": "user", "promptId": id, "origin": map[string]any{"kind": kind}, "message": map[string]any{"content": text}})
}

// fxAsk is an assistant record calling Bash with tool id.
func fxAsk(ms int64, tool string) string {
	return fxRec(ms, map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": tool, "name": "Bash"}}}})
}

// fxAnswer is the tool_result of tool id, under prompt id.
func fxAnswer(ms int64, id, tool string) string {
	return fxRec(ms, map[string]any{"type": "user", "promptId": id, "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": tool}}}})
}

// fxQueued is a queued_command attachment stamped at its enqueue time; a
// nil prompt is left out.
func fxQueued(ms int64, kind string, prompt any) string {
	a := map[string]any{"type": "queued_command", "origin": map[string]any{"kind": kind}}
	if prompt != nil {
		a["prompt"] = prompt
	}
	return fxRec(ms, map[string]any{"type": "attachment", "attachment": a})
}

// fxDequeue is a queue operation record.
func fxDequeue(ms int64) string {
	return fxRec(ms, map[string]any{"type": "queue-operation", "operation": "remove"})
}

// fxPeerOpen is a peer message's opening tag line.
const fxPeerOpen = `<cross-session-message from="uds:/tmp/x.sock" from-name="a">`

// fxPeer is a message from another session as its hook receives it: the
// bare element.
func fxPeer(body string) string {
	return fxPeerOpen + "\n" + body + "\n</cross-session-message>"
}

// fxDisplay is an idle message as the transcript shows it: a lead line, the
// element, and a closing paragraph.
func fxDisplay(el string) string {
	return "A message arrived from another session:\n" + el + "\n\nIt is not from the user.\nAnswer with the tool."
}

// TestTurnReplay_PromptJoin places each prompt-hook fire on the record it
// read, over a stamped synthetic transcript. pI: a fire before the turn's
// first assistant record reads the opening (early); a fire after a tool
// round reads the one queued element drained there (row i, late), a queue
// operation and the element's earlier enqueue stamp notwithstanding. pII:
// two drained elements are a miss (row ii). pIII: a fire 6,000 ms after its
// opening, with no assistant record before it, is early (row iii); one
// after its tool round, with nothing drained, is a miss. pIV: a fire 200 ms
// after its opening, behind an assistant record, reads its drained element
// (row iv), and its early fire reads a peer display framed through the
// replay's stripper. pV: a drained element whose prompt is null is a miss.
// pVI: a display holding two elements is a miss row with no rule column.
// An unknown prompt id is a miss. Break: the first user record joined for
// every fire, a time constant in place of the structural test, or a null
// prompt read as text.
func TestTurnReplay_PromptJoin(t *testing.T) {
	src := []string{
		fxOpen(0, "pI", "human", "go"),
		fxAsk(1000, "i1"),
		fxAnswer(2000, "pI", "i1"),
		fxDequeue(2001),
		fxQueued(500, "peer", fxPeer("check the gate")),
		fxAsk(2100, "i2"),
		fxAnswer(2200, "pI", "i2"),

		fxOpen(100_000, "pII", "human", "go"),
		fxAsk(101_000, "ii1"),
		fxAnswer(102_000, "pII", "ii1"),
		fxQueued(100_800, "peer", fxPeer("one")),
		fxQueued(100_900, "human", "two"),
		fxAsk(102_100, "ii2"),

		fxOpen(200_000, "pIII", "human", "go"),
		fxAsk(207_000, "iii1"),
		fxAnswer(207_020, "pIII", "iii1"),

		fxOpen(300_000, "pIV", "peer", fxDisplay(fxPeer("look at the plan"))),
		fxAsk(300_100, "iv1"),
		fxAnswer(300_150, "pIV", "iv1"),
		fxQueued(300_120, "human", "stop and report"),
		fxAsk(300_300, "iv2"),

		fxOpen(400_000, "pV", "human", "go"),
		fxAsk(400_100, "v1"),
		fxAnswer(400_150, "pV", "v1"),
		fxQueued(400_120, "peer", json.RawMessage("null")),
		fxAsk(400_300, "v2"),

		fxOpen(600_000, "pVI", "peer", fxDisplay(fxPeer("a")+"\n"+fxPeer("b"))),
	}
	ls, err := tRead(strings.NewReader(strings.Join(src, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name, id   string
		at         int64
		kind, text string
		late       bool
		miss       string
	}{
		{"pI's opening, early", "pI", 30, "human", "go", false, ""},
		{"(i) one drained element, late", "pI", 2050, "peer", fxPeer("check the gate"), true, ""},
		{"(ii) two drained elements, the first fire", "pII", 102_050, "", "", true, tMissDrains},
		{"(ii) two drained elements, the second fire", "pII", 102_060, "", "", true, tMissDrains},
		{"(iii) 6,000 ms after its opening, no assistant record before it", "pIII", 206_000, "human", "go", false, ""},
		{"late, nothing drained", "pIII", 207_050, "", "", true, tMissNoDrain},
		{"pIV's opening, early", "pIV", 300_020, "peer", fxDisplay(fxPeer("look at the plan")), false, ""},
		{"(iv) 200 ms after its opening, behind an assistant record", "pIV", 300_200, "human", "stop and report", true, ""},
		{"a drained element whose prompt is null", "pV", 400_200, "", "", true, tMissNoText},
		{"a display of two elements, early", "pVI", 600_010, "peer", fxDisplay(fxPeer("a") + "\n" + fxPeer("b")), false, ""},
		{"no prompt record", "pX", 700_000, "", "", false, tMissNoPrompt},
	}
	var notes []rNote
	for _, r := range rows {
		f := tFireOf(ls, r.id, fxBase+r.at)
		if f.kind != r.kind || f.text != r.text || f.late != r.late || f.miss != r.miss {
			t.Errorf("%s: kind %q, late %t, miss %q, text %d bytes; want %q, %t, %q, %d bytes",
				r.name, f.kind, f.late, f.miss, len(f.text), r.kind, r.late, r.miss, len(r.text))
		}
		notes = append(notes, rNote{Type: "steer", Pattern: "task-preflight", PromptID: r.id, AtMS: fxBase + r.at})
	}
	p := rPreflightCount(ls, notes)
	wantMiss := map[string]int{tMissDrains: 2, tMissNoDrain: 1, tMissNoText: 1, tMissNoPrompt: 1, rMissDisplay: 1}
	if p.early != 4 || p.late != 6 || p.lateOne != 2 || p.maxEarly != 6000 || p.minLate != 200 || !maps.Equal(p.miss, wantMiss) {
		t.Errorf("split early %d, late %d, late on one element %d, largest early gap %d, smallest late gap %d, misses %v; want 4, 6, 2, 6000, 200, %v",
			p.early, p.late, p.lateOne, p.maxEarly, p.minLate, p.miss, wantMiss)
	}
	for kind, want := range map[string]rCnt{"peer": {3, 2, 0}, "human": {3, 0, 3}} {
		if got := p.by[kind]; got == nil || *got != want {
			t.Errorf("kind %s: notes, framed, typed %v; want %v", kind, got, want)
		}
	}
	claims := []rNote{
		{Type: "steer", Pattern: "claim-vs-record", Key: "last-turn", PromptID: "pI", AtMS: fxBase + 30},
		{Type: "steer", Pattern: "claim-vs-record", Key: "last-turn", PromptID: "pI", AtMS: fxBase + 2050},
		{Type: "steer", Pattern: "claim-vs-record", Key: "last-turn", PromptID: "pX", AtMS: fxBase + 700_000},
	}
	if typed, framed, miss := rTakers(ls, claims); typed != 1 || framed != 1 || miss != 1 {
		t.Errorf("claim notes by the fire that took them: typed %d, framed %d, miss %d; want 1 each", typed, framed, miss)
	}
}

// TestTurnReplay_PeerDisplay: the replay hands the frame reader what a
// fire's hook received. A peer display gives its element, which reads
// framed; the display unstripped reads typed (control). A display of two
// elements, or of none, is a miss with no input. Any kind's prompt past
// the reader's head gives the head, cut back to a character boundary and
// said cut; a peer element's head reads framed only as cut (control: read
// whole, it reads typed). Break: hand the display whole, fold a display of
// two elements in, or keep a prompt past the head.
func TestTurnReplay_PeerDisplay(t *testing.T) {
	el := fxPeer("check the gate")
	display := fxDisplay(el)
	in, cut, why := rHookInput("peer", display)
	if in != el || cut || why != "" {
		t.Errorf("a peer display: input %q, cut %t, miss %q; want the element, not cut, no miss", in, cut, why)
	}
	if framed, _ := harness.Framed(in, cut); !framed {
		t.Errorf("the element reads typed; want framed")
	}
	if framed, _ := harness.Framed(display, false); framed {
		t.Errorf("control: the display unstripped reads framed; want typed")
	}
	if in, cut, why := rHookInput("peer", el); in != el || cut || why != "" {
		t.Errorf("a bare element: input %q, cut %t, miss %q; want it as it is", in, cut, why)
	}
	for name, d := range map[string]string{
		"two elements": fxDisplay(el + "\n" + fxPeer("and this")),
		"no element":   "A message arrived.\nNothing else.",
	} {
		if in, _, why := rHookInput("peer", d); in != "" || why != rMissDisplay {
			t.Errorf("%s: input %d bytes, miss %q; want none, %q", name, len(in), why, rMissDisplay)
		}
	}
	if in, cut, why := rHookInput("human", "go"); in != "go" || cut || why != "" {
		t.Errorf("a short typed prompt: input %q, cut %t, miss %q; want it as it is", in, cut, why)
	}
	typed := strings.Repeat("x", intent.MaxPromptBytes+1)
	if in, cut, _ := rHookInput("human", typed); in != typed[:intent.MaxPromptBytes] || !cut {
		t.Errorf("a typed prompt past the head: %d bytes, cut %t; want %d, true", len(in), cut, intent.MaxPromptBytes)
	}
	// Two-byte characters, the head's last byte the first of one.
	pad := ""
	if (intent.MaxPromptBytes-len(fxPeerOpen)-1)%2 == 0 {
		pad = "a"
	}
	long := fxPeer(pad + strings.Repeat("\u00e9", 10<<10))
	in, cut, why = rHookInput("peer", fxDisplay(long))
	if why != "" || !cut || len(in) != intent.MaxPromptBytes-1 || !utf8.ValidString(in) || !strings.HasPrefix(long, in) {
		t.Errorf("a long peer element: %d bytes, cut %t, miss %q, valid %t; want %d, true, none, true",
			len(in), cut, why, utf8.ValidString(in), intent.MaxPromptBytes-1)
	}
	if framed, _ := harness.Framed(in, true); !framed {
		t.Errorf("the long element's head, read cut, reads typed; want framed")
	}
	if framed, _ := harness.Framed(in, false); framed {
		t.Errorf("control: the long element's head, read whole, reads framed; want typed")
	}
}

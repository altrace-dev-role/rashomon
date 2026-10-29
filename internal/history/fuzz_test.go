package history

import (
	"fmt"
	"strings"
	"testing"
)

// FuzzHistCalls holds Read to two kinds of oracle.
//
// Over the raw input, read as a transcript: no panic, Seq counts from 0 in
// emission order, no id is emitted twice, and a call with no outcome is
// Missing or Unsupported, never graded as if it ran.
//
// Over a transcript generated from the input (ops): every distinct tool_use
// id is emitted exactly once, in first-use order; a call with no result
// after it is Missing; and a call whose first result after it has no
// unreadable line between them gets the outcome that result encodes.
func FuzzHistCalls(f *testing.F) {
	f.Add([]byte(`{"message":{"content":[{"type":"tool_use","id":"a","name":"Bash","input":{}}]}}`), []byte{0, 1, 4, 3})
	f.Add([]byte("{\n\n[]\n{\"message\":{\"content\":\"x\"}}"), []byte{0, 0, 9, 2, 1, 10, 5, 6, 3, 4})
	f.Add([]byte{}, []byte{1, 0, 0, 8, 2, 8, 7, 3, 11, 0})
	f.Fuzz(func(t *testing.T, raw, ops []byte) {
		checkRaw(t, raw)
		checkOps(t, ops)
	})
}

func checkRaw(t *testing.T, raw []byte) {
	seen := map[string]bool{}
	seq := 0
	_, err := Read(strings.NewReader(string(raw)), DefaultLimits, func(c Call) {
		if c.Seq != seq {
			t.Fatalf("seq %d emitted at position %d", c.Seq, seq)
		}
		seq++
		if seen[c.ToolUseID] {
			t.Fatalf("id %q emitted twice", c.ToolUseID)
		}
		seen[c.ToolUseID] = true
		if c.Outcome == OutcomeNone && c.Grade != Missing && c.Grade != Unsupported {
			t.Fatalf("a call with no outcome graded %s", c.Grade)
		}
		if c.Grade == Missing && c.Outcome != OutcomeNone {
			t.Fatalf("a Missing call with outcome %s", c.Outcome)
		}
		if c.UsePos.IsZero() || c.UsePos.Line != c.Line {
			t.Fatalf("%q: use at %+v, line %d", c.ToolUseID, c.UsePos, c.Line)
		}
		if (c.Outcome == OutcomeNone) != c.ResultPos.IsZero() {
			t.Fatalf("%q: outcome %s with result at %+v", c.ToolUseID, c.Outcome, c.ResultPos)
		}
		if !c.ResultPos.IsZero() && !c.UsePos.Before(c.ResultPos) {
			t.Fatalf("%q: result at %+v is not after its use at %+v", c.ToolUseID, c.ResultPos, c.UsePos)
		}
	})
	if err != nil {
		t.Fatalf("a read from memory failed: %v", err)
	}
}

// variants are the results the generator writes, with the outcome each
// encodes for a Bash call.
var variants = []struct {
	isError bool
	text    string
	outcome Outcome
}{
	{false, "ok", OK},
	{true, "Exit code 1\nfail", Failed},
	{true, "Exit code 124", Timeout},
	{true, "Exit code 127", NotRun},
	{true, "Permission for this action has been denied.", Denied},
	{true, "something else", ToolError},
}

func checkOps(t *testing.T, ops []byte) {
	type idState struct {
		used, answered, skipped bool
		first                   Outcome
		useLine, resultLine     int // each line holds one block, so the block is 0
	}
	var lines []string
	state := map[string]*idState{}
	var order []string
	uses, results := 0, 0
	for i := 0; i+1 < len(ops); i += 2 {
		id := fmt.Sprintf("t%d", ops[i+1]%6)
		st := state[id]
		if st == nil {
			st = &idState{}
			state[id] = st
		}
		switch ops[i] % 4 {
		case 0, 1: // a use, or a repeated one
			uses++
			lines = append(lines, at(use(id, "Bash", bash("x"))).String())
			if !st.used {
				st.used, st.useLine = true, len(lines)
				order = append(order, id)
			}
		case 2: // a result
			v := variants[int(ops[i])/4%len(variants)]
			results++
			lines = append(lines, at(result(id, v.isError, v.text)).String())
			if st.used && !st.answered {
				st.answered, st.first, st.resultLine = true, v.outcome, len(lines)
			}
		case 3: // a line that cannot be read
			lines = append(lines, "{cut")
			for _, s := range state {
				if s.used && !s.answered {
					s.skipped = true
				}
			}
		}
	}
	var got []Call
	stats, err := Read(strings.NewReader(strings.Join(lines, "\n")), DefaultLimits, func(c Call) { got = append(got, c) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(order) {
		t.Fatalf("emitted %d calls, want %d distinct ids", len(got), len(order))
	}
	for i, c := range got {
		if c.ToolUseID != order[i] {
			t.Fatalf("call %d is %q, want %q (first-use order)", i, c.ToolUseID, order[i])
		}
		st := state[c.ToolUseID]
		if c.UsePos != (Pos{Line: st.useLine}) {
			t.Fatalf("%q used at %+v, want line %d", c.ToolUseID, c.UsePos, st.useLine)
		}
		if st.answered && c.ResultPos != (Pos{Line: st.resultLine}) || !st.answered && !c.ResultPos.IsZero() {
			t.Fatalf("%q answered at %+v, want line %d (answered %v)", c.ToolUseID, c.ResultPos, st.resultLine, st.answered)
		}
		switch {
		case !st.answered:
			if c.Grade != Missing || c.Reason != NoResult {
				t.Fatalf("%q has no result but is %s %s", c.ToolUseID, c.Grade, c.Reason)
			}
		case c.Outcome != st.first:
			t.Fatalf("%q = %s, its first result encodes %s", c.ToolUseID, c.Outcome, st.first)
		case st.skipped && c.Grade == Verified:
			t.Fatalf("%q is Verified across an unreadable line", c.ToolUseID)
		}
	}
	if stats.Uses != uses || stats.Results != results {
		t.Fatalf("stats uses %d results %d, want %d %d", stats.Uses, stats.Results, uses, results)
	}
}

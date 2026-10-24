package hook

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHookOrder_DeclarationTerminalThenExecution runs two calls' handlers
// against one store, interleaved as a harness may run them: A's PreToolUse,
// B's PreToolUse, then A's PostToolUse and B's. The seqs read back prove what
// the run builder in internal/verify assumes. A call's terminal is written by
// the PreToolUse handler's Close and precedes its execution; the ordered
// stream holds no coverage record (AppendCoverage writes a file of its own);
// and another call's PreToolUse pair may land between a call's terminal and
// its execution.
func TestHookOrder_DeclarationTerminalThenExecution(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	post := func(id string) string {
		return `{"session_id":"sess-caps","tool_use_id":"` + id + `","tool_name":"Bash","hook_event_name":"PostToolUse"}`
	}
	for _, id := range []string{"toolu_a", "toolu_b"} {
		h := New(st, time.Now)
		h.Close(&Signals{}, h.Capture(strings.NewReader(capPayload(t, "tool_use_id", id))))
	}
	for _, id := range []string{"toolu_a", "toolu_b"} {
		p := NewPost(st, time.Now)
		p.Close(&Signals{}, p.Capture(strings.NewReader(post(id))))
	}
	run, err := st.ReadRun("sess-caps")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 2 || len(run.Terminals) != 2 || len(run.Executions) != 2 || run.Skipped != 0 {
		t.Fatalf("read %d declarations, %d terminals and %d executions with Skipped %d; want 2, 2, 2 and 0",
			len(run.Declarations), len(run.Terminals), len(run.Executions), run.Skipped)
	}
	decl := map[string]int64{}
	for _, d := range run.Declarations {
		decl[d.ToolUseID] = d.Seq
	}
	term, exec := map[string]*int64{}, map[string]*int64{}
	for _, x := range run.Terminals {
		term[x.ToolUseID] = x.Seq
	}
	for _, x := range run.Executions {
		exec[x.ToolUseID] = x.Seq
	}
	text := func(p *int64) string {
		if p == nil {
			return "nil (spilled)"
		}
		return strconv.FormatInt(*p, 10)
	}
	// Written out, not computed: A's PreToolUse pair, B's, then the two
	// executions in PostToolUse order.
	for _, w := range []struct {
		id               string
		decl, term, exec int64
	}{
		{"toolu_a", 1, 2, 5},
		{"toolu_b", 3, 4, 6},
	} {
		if decl[w.id] != w.decl {
			t.Errorf("%s: declaration Seq %d, want %d", w.id, decl[w.id], w.decl)
		}
		if s := term[w.id]; s == nil || *s != w.term {
			t.Errorf("%s: terminal Seq %s, want %d", w.id, text(s), w.term)
		}
		if s := exec[w.id]; s == nil || *s != w.exec {
			t.Errorf("%s: execution Seq %s, want %d", w.id, text(s), w.exec)
		}
	}
}

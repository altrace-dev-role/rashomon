package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestHarnessArg: no flag is Claude Code, and anything the build does not
// know -- a value it has never heard of, or the flag with nothing after it --
// is refused rather than read as Claude Code, because a payload read under
// the wrong harness's rules records a wrong fact.
func TestHarnessArg(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want Harness
		ok   bool
	}{
		{nil, ClaudeCode, true},
		{[]string{"--install", "abc"}, ClaudeCode, true},
		{[]string{"--harness", "codex"}, Codex, true},
		{[]string{"start", "--harness", "codex", "--install", "abc"}, Codex, true},
		{[]string{"--harness", "cursor"}, "", false},
		{[]string{"--harness", "Codex"}, "", false},
		{[]string{"--install", "abc", "--harness"}, "", false},
	} {
		got, ok := HarnessArg(tc.args)
		if got != tc.want || ok != tc.ok {
			t.Errorf("HarnessArg(%q) = %q, %v; want %q, %v", tc.args, got, ok, tc.want, tc.ok)
		}
	}
}

// codexTurn records the main agent's turn_id as its prompt_id, gives a
// subagent's call the main agent's latest turn rather than its own, and keeps
// the subagent's own turn when there is no main-agent call to borrow from.
// Break: return the payload's turn_id for every call and the subagent case
// reads its own turn.
func TestCodexTurn(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw := func(turn string) []byte { return []byte(`{"turn_id":"` + turn + `"}`) }
	deref := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}

	if got := deref(codexTurn(st, "s", "agent-1", raw("sub"))); got != "sub" {
		t.Errorf("subagent with no main call before it: %s, want its own turn", got)
	}
	main := codexTurn(st, "s", "", raw("parent"))
	if deref(main) != "parent" {
		t.Fatalf("main agent: %s, want parent", deref(main))
	}
	if err := st.AppendDeclaration(store.Declaration{
		Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion, ToolUseID: "1", SessionID: "s", PromptID: main,
	}); err != nil {
		t.Fatal(err)
	}
	if got := deref(codexTurn(st, "s", "agent-1", raw("sub"))); got != "parent" {
		t.Errorf("subagent: %s, want parent", got)
	}
	if got := codexTurn(st, "s", "", []byte(`{}`)); got != nil {
		t.Errorf("no turn_id: %s, want nil", *got)
	}
}

package store

import (
	"strings"
	"testing"
)

// LatestMainPromptID reads back the prompt_id of the main agent's last
// declaration, passing over a subagent's after it, and finds nothing past
// the tail window rather than reading the whole file. Break: drop the
// agent_id check and the subagent's own turn comes back.
func TestLatestMainPromptID(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.LatestMainPromptID("s"); ok {
		t.Fatal("found a turn in a run that does not exist")
	}

	str := func(s string) *string { return &s }
	decl := func(id string, prompt, agent *string, pad int) {
		t.Helper()
		d := Declaration{
			Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: id, SessionID: "s",
			PromptID: prompt, AgentID: agent, TranscriptPath: strings.Repeat("p", pad),
		}
		if err := st.AppendDeclaration(d); err != nil {
			t.Fatal(err)
		}
	}
	decl("1", str("turn-a"), nil, 0)
	decl("2", str("turn-b"), nil, 0)
	decl("3", str("turn-sub"), str("agent-1"), 0)
	decl("4", nil, nil, 0)

	if got, ok := st.LatestMainPromptID("s"); !ok || got != "turn-b" {
		t.Errorf("got %q, %v; want turn-b: the subagent's turn and a prompt-less call are passed over", got, ok)
	}

	// Push turn-b out of the window with subagent declarations alone.
	for range 3 {
		decl("pad", str("turn-sub"), str("agent-1"), mainTurnWindow/2)
	}
	if got, ok := st.LatestMainPromptID("s"); ok {
		t.Errorf("got %q past the tail window; want none", got)
	}
}

package history

import (
	"strings"
	"testing"
)

// TestHIST15_TheReaderKeepsTheParentSideOfTheSubagentLink: an Agent call's
// result names the agent it ran (toolUseResult.agentId), a call a second line
// answers with another agent claims two, and a subagent's assistant line names
// the type it is attributed to (attributionAgent). The reader keeps these and
// reads no text for them. A result line without a toolUseResult is told from
// one with a null or a string.
func TestHIST15_TheReaderKeepsTheParentSideOfTheSubagentLink(t *testing.T) {
	line := func(extra string) string {
		return `{"sessionId":"s","version":"2.1.312","cwd":"/r","timestamp":"2026-01-01T00:00:00.000Z",` + extra + `}` + "\n"
	}
	use := func(id, name, extra string) string {
		return line(`"type":"assistant",` + extra + `"message":{"content":[{"type":"tool_use","id":"` + id + `","name":"` + name + `","input":{}}]}`)
	}
	res := func(id, tur string) string {
		return line(`"type":"user",` + tur + `"message":{"content":[{"type":"tool_result","tool_use_id":"` + id + `","content":"ok"}]}`)
	}
	text := use("blocker", "Bash", "") + // never answered: holds back the calls after it
		use("ag", "Agent", "") +
		res("ag", `"toolUseResult":{"agentId":"A1"},`) +
		res("ag", `"toolUseResult":{"agentId":"A2"},`) +
		use("sub", "Bash", `"isSidechain":true,"agentId":"A1","attributionAgent":"Explore",`) +
		res("sub", ``) +
		use("nul", "Bash", "") + res("nul", `"toolUseResult":null,`) +
		use("str", "Bash", "") + res("str", `"toolUseResult":"Error: x",`) +
		use("bad", "Agent", "") + res("bad", `"toolUseResult":{"agentId":7},`)
	got := map[string]Call{}
	if _, err := Read(strings.NewReader(text), DefaultLimits, func(c Call) { got[c.ToolUseID] = c }); err != nil {
		t.Fatal(err)
	}
	if c := got["ag"]; c.ResultAgentID != "A1" || !c.ResultAgentConflict {
		t.Errorf("an Agent call answered twice: agent %q conflict %v", c.ResultAgentID, c.ResultAgentConflict)
	}
	if c := got["sub"]; c.AttributionAgent != "Explore" || !c.Sidechain || c.AgentID != "A1" || !c.ResultDetailAbsent {
		t.Errorf("a subagent's call: attribution %q sidechain %v agent %q detail absent %v", c.AttributionAgent, c.Sidechain, c.AgentID, c.ResultDetailAbsent)
	}
	if c := got["ag"]; c.AttributionAgent != "" || c.ResultDetailAbsent {
		t.Errorf("the main agent's call: attribution %q, detail absent %v", c.AttributionAgent, c.ResultDetailAbsent)
	}
	for _, id := range []string{"nul", "str"} {
		if c := got[id]; c.ResultDetailAbsent || c.ResultObject || c.ResultAgentID != "" {
			t.Errorf("%s: detail absent %v object %v agent %q", id, c.ResultDetailAbsent, c.ResultObject, c.ResultAgentID)
		}
	}
	if c := got["bad"]; c.ResultAgentID != "" || !c.ResultObject {
		t.Errorf("an agentId that is not a string: %q, object %v", c.ResultAgentID, c.ResultObject)
	}
}

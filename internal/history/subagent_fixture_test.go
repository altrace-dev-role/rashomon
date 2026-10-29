package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// subagentFixture is a live session of Claude Code 2.1.312 (claude -p, model
// claude-haiku-5-5) that wrote one file and then dispatched two subagents of
// different built-in types through the Agent tool, one after the other: an
// Explore agent that made one Read call, and a general-purpose agent that made
// one Bash call (`ls`). It was run and scrubbed as 2.1.312 was (see the commit
// that added the directory), with SubagentStart and SubagentStop registered
// beside install.Events, which Rashomon does not install.
//
// transcript.jsonl is the parent. Each subagent has its own transcript beside
// it, at <session>/subagents/agent-<id>.jsonl, which is where Claude Code puts
// it and where SubagentStop's agent_transcript_path points. payloads.ndjson is
// every hook payload in the order it fired.
const subagentFixture = "../../test/fixtures/claude-code/2.1.312-subagents/"

func sfLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for i, l := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("%s line %d: %v", path, i+1, err)
		}
		out = append(out, m)
	}
	return out
}

func sfMap(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

// sfBlocks returns the content blocks of a transcript line, none for a line
// whose content is a string.
func sfBlocks(l map[string]any) []map[string]any {
	raw, _ := sfMap(l, "message")["content"].([]any)
	var out []map[string]any
	for _, b := range raw {
		if b, ok := b.(map[string]any); ok {
			out = append(out, b)
		}
	}
	return out
}

// TestHIST14_SubagentFixture characterises the one question the fixture was
// captured to answer: can a subagent's tool calls be given the agent_type the
// hook payloads carry, from the transcripts alone, by a link that is
// unambiguous? Result, on 2.1.312: yes, for these two built-in types, and it
// is one-to-one. The link is
//
//	subagent line's top-level agentId        (every line of a subagent transcript)
//	  == parent line's toolUseResult.agentId (the user line answering the call)
//	that line's message.content[].tool_use_id
//	  == the parent's Agent tool_use block id
//	that block's input.subagent_type         (the type, byte for byte the payload's agent_type)
//
// Nothing here reads a file name or message text: the subagent transcripts are
// found by walking the directory, and each is identified by the agentId on its
// own lines. A transcript line that does not carry the fields, or a parent
// call that does not have exactly one result naming exactly one agent, is
// outside what this fixture proves and stays unsupported.
func TestHIST14_SubagentFixture(t *testing.T) {
	// --- The payloads: what Claude Code told the hooks.
	type payload struct {
		event, tool, toolUseID, agentID, agentType string
		input                                      map[string]any
		response                                   map[string]any
		raw                                        map[string]any
	}
	var payloads []payload
	for _, m := range sfLines(t, subagentFixture+"payloads.ndjson") {
		payloads = append(payloads, payload{
			event: str(m, "hook_event_name"), tool: str(m, "tool_name"), toolUseID: str(m, "tool_use_id"),
			agentID: str(m, "agent_id"), agentType: str(m, "agent_type"),
			input: sfMap(m, "tool_input"), response: sfMap(m, "tool_response"), raw: m,
		})
	}
	var events []string
	for _, p := range payloads {
		events = append(events, p.event)
	}
	if got, want := strings.Join(events, " "), strings.Join([]string{
		"SessionStart", "UserPromptSubmit",
		"PreToolUse", "PostToolUse", // Write
		"PreToolUse", "SubagentStart", "PreToolUse", "PostToolUse", "SubagentStop", "PostToolUse", // Agent(Explore): Read inside
		"PreToolUse", "SubagentStart", "PreToolUse", "PostToolUse", "SubagentStop", "PostToolUse", // Agent(general-purpose): Bash inside
		"Stop", "SessionEnd",
	}, " "); got != want {
		t.Fatalf("payload events:\n got %s\nwant %s", got, want)
	}

	// Two subagents, two types, one tool call each, and every payload of a
	// subagent's call carries both fields. The main agent's own calls (Write
	// and the two Agent calls) carry neither.
	type agentCall struct{ tool, toolUseID string }
	wantCall := map[string]agentCall{} // agent_type -> its one call
	typeOfAgent := map[string]string{} // payload agent_id -> agent_type
	callAgent := map[string]string{}   // subagent tool_use_id -> payload agent_id
	callType := map[string]string{}    // subagent tool_use_id -> payload agent_type
	var mainCalls []string
	for _, p := range payloads {
		switch p.event {
		case "PreToolUse", "PostToolUse":
			if p.agentID == "" && p.agentType == "" {
				mainCalls = append(mainCalls, p.event+":"+p.tool)
				continue
			}
			if p.agentID == "" || p.agentType == "" {
				t.Fatalf("%s %s carries only one of agent_id %q and agent_type %q", p.event, p.tool, p.agentID, p.agentType)
			}
			if prev, ok := typeOfAgent[p.agentID]; ok && prev != p.agentType {
				t.Fatalf("agent %s is %q in one payload and %q in another", p.agentID, prev, p.agentType)
			}
			typeOfAgent[p.agentID] = p.agentType
			callAgent[p.toolUseID], callType[p.toolUseID] = p.agentID, p.agentType
			if prev, ok := wantCall[p.agentType]; ok && prev != (agentCall{p.tool, p.toolUseID}) {
				t.Fatalf("agent type %s made two calls: %v and %v", p.agentType, prev, p)
			}
			wantCall[p.agentType] = agentCall{p.tool, p.toolUseID}
		case "SubagentStart", "SubagentStop":
			// Both carry the pair, and neither carries a tool_use_id.
			if p.agentID == "" || p.agentType == "" || p.toolUseID != "" {
				t.Fatalf("%s: agent_id %q agent_type %q tool_use_id %q", p.event, p.agentID, p.agentType, p.toolUseID)
			}
			if prev, ok := typeOfAgent[p.agentID]; ok && prev != p.agentType {
				t.Fatalf("%s: agent %s was %q", p.event, p.agentID, prev)
			}
			typeOfAgent[p.agentID] = p.agentType
		default:
			if p.agentID != "" || p.agentType != "" {
				t.Fatalf("%s carries agent_id %q agent_type %q; only tool and subagent events do", p.event, p.agentID, p.agentType)
			}
		}
	}
	if got, want := strings.Join(mainCalls, " "), "PreToolUse:Write PostToolUse:Write PreToolUse:Agent PostToolUse:Agent PreToolUse:Agent PostToolUse:Agent"; got != want {
		t.Fatalf("main-agent calls (no agent fields): %s, want %s", got, want)
	}
	if len(typeOfAgent) != 2 || len(wantCall) != 2 {
		t.Fatalf("premise: %d agents, %d types with calls, want 2 and 2: %v", len(typeOfAgent), len(wantCall), typeOfAgent)
	}
	if c := wantCall["Explore"]; c.tool != "Read" || c.toolUseID == "" {
		t.Fatalf("Explore's call: %+v, want one Read", c)
	}
	if c := wantCall["general-purpose"]; c.tool != "Bash" || c.toolUseID == "" {
		t.Fatalf("general-purpose's call: %+v, want one Bash", c)
	}

	// --- The parent transcript: each Agent call, and the line that answers it.
	parent := sfLines(t, subagentFixture+"transcript.jsonl")
	agentCalls := map[string]string{} // Agent tool_use id -> input.subagent_type
	var agentCallOrder []string
	resultAgents := map[string][]string{} // Agent tool_use id -> every toolUseResult.agentId on lines answering it
	resultTypes := map[string][]string{}  // ... and toolUseResult.agentType
	for i, l := range parent {
		if _, ok := l["agentId"]; ok {
			t.Fatalf("parent line %d has a top-level agentId; only subagent lines do", i+1)
		}
		if l["isSidechain"] == true {
			t.Fatalf("parent line %d is a sidechain line", i+1)
		}
		for _, b := range sfBlocks(l) {
			switch str(b, "type") {
			case "tool_use":
				if str(b, "name") != "Agent" {
					continue
				}
				id := str(b, "id")
				st, _ := sfMap(b, "input")["subagent_type"].(string)
				if _, dup := agentCalls[id]; dup || id == "" || st == "" {
					t.Fatalf("parent line %d: Agent call %q with subagent_type %q", i+1, id, st)
				}
				agentCalls[id] = st
				agentCallOrder = append(agentCallOrder, id)
			case "tool_result":
				if tur := sfMap(l, "toolUseResult"); tur != nil {
					if a, ok := tur["agentId"].(string); ok {
						id := str(b, "tool_use_id")
						resultAgents[id] = append(resultAgents[id], a)
						resultTypes[id] = append(resultTypes[id], str(tur, "agentType"))
					}
				}
			}
		}
	}
	if len(agentCalls) != 2 {
		t.Fatalf("premise: %d Agent calls in the parent transcript, want 2", len(agentCalls))
	}
	// Each Agent call has exactly one result that names an agent; no agent id
	// is named by two calls; the parent call input names the same type the
	// payload for that call does.
	claimed := map[string]string{} // agentId -> the one Agent tool_use id that claims it
	parentType := map[string]string{}
	for _, id := range agentCallOrder {
		got := resultAgents[id]
		if len(got) != 1 || got[0] == "" {
			t.Fatalf("Agent call %s has %d results naming an agent (%v), want exactly 1", id, len(got), got)
		}
		if other, dup := claimed[got[0]]; dup {
			t.Fatalf("agent %s is claimed by both %s and %s", got[0], other, id)
		}
		claimed[got[0]] = id
		parentType[got[0]] = agentCalls[id]
		// The result repeats the type under toolUseResult.agentType.
		if resultTypes[id][0] != agentCalls[id] {
			t.Fatalf("Agent call %s: input.subagent_type %q, toolUseResult.agentType %q", id, agentCalls[id], resultTypes[id][0])
		}
	}
	for id := range resultAgents {
		if _, ok := agentCalls[id]; !ok {
			t.Fatalf("a toolUseResult.agentId answers %s, which is no Agent call", id)
		}
	}
	// The same call's hook payloads agree: the PreToolUse input names the
	// type, and the PostToolUse tool_response names both the agent and the type.
	for _, p := range payloads {
		if p.tool != "Agent" {
			continue
		}
		if st, _ := p.input["subagent_type"].(string); st != agentCalls[p.toolUseID] {
			t.Fatalf("%s Agent %s: payload subagent_type %q, transcript %q", p.event, p.toolUseID, st, agentCalls[p.toolUseID])
		}
		if p.event == "PostToolUse" {
			if str(p.response, "agentId") != resultAgents[p.toolUseID][0] || str(p.response, "agentType") != agentCalls[p.toolUseID] {
				t.Fatalf("PostToolUse Agent %s: tool_response agentId/agentType %q/%q, transcript %q/%q",
					p.toolUseID, str(p.response, "agentId"), str(p.response, "agentType"), resultAgents[p.toolUseID][0], agentCalls[p.toolUseID])
			}
		}
	}

	// --- The subagent transcripts: found by walking, identified by agentId.
	var files []string
	err := filepath.Walk(subagentFixture, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".jsonl") && filepath.Base(filepath.Dir(path)) == "subagents" {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) != 2 {
		t.Fatalf("premise: %d subagent transcripts, want 2: %v", len(files), files)
	}
	seenAgent := map[string]bool{}
	linked := map[string]string{} // subagent tool_use id -> type recovered through the transcript link
	for _, f := range files {
		lines := sfLines(t, f)
		// Every line carries the same agentId, marks itself a sidechain, and
		// belongs to this session; the file is identified by that field alone.
		agent := str(lines[0], "agentId")
		if agent == "" {
			t.Fatalf("%s: first line has no agentId", f)
		}
		if seenAgent[agent] {
			t.Fatalf("two subagent transcripts carry agentId %s", agent)
		}
		seenAgent[agent] = true
		for i, l := range lines {
			if str(l, "agentId") != agent || l["isSidechain"] != true || str(l, "sessionId") != str(parent[0], "sessionId") {
				t.Fatalf("%s line %d: agentId %q sidechain %v session %q", f, i+1, str(l, "agentId"), l["isSidechain"], str(l, "sessionId"))
			}
		}
		// The agent id names exactly one parent Agent call, whose input gives the type.
		callID, ok := claimed[agent]
		if !ok {
			t.Fatalf("%s: agentId %s is claimed by no parent Agent call", f, agent)
		}
		recovered := parentType[agent]
		// Its tool calls: every tool_use block in the file.
		n := 0
		for _, l := range lines {
			for _, b := range sfBlocks(l) {
				if str(b, "type") != "tool_use" {
					continue
				}
				n++
				id := str(b, "id")
				if _, dup := linked[id]; dup {
					t.Fatalf("tool call %s appears in two subagent transcripts", id)
				}
				linked[id] = recovered
				// The same line names the type in attributionAgent. That is a
				// second, in-line route to the same value, on assistant lines only.
				if str(l, "attributionAgent") != recovered {
					t.Fatalf("%s: attributionAgent %q on the line of %s, link says %q", f, str(l, "attributionAgent"), id, recovered)
				}
			}
		}
		if n != 1 {
			t.Fatalf("%s: %d tool calls, want 1 (agent via parent call %s)", f, n, callID)
		}
		// The transcript's agentId is the payload's agent_id, and the type the
		// link recovers is the payload's agent_type.
		if typeOfAgent[agent] != recovered {
			t.Fatalf("%s: payload agent_type %q, link recovers %q", f, typeOfAgent[agent], recovered)
		}
	}
	for agent := range claimed {
		if !seenAgent[agent] {
			t.Fatalf("parent claims agent %s, which has no subagent transcript", agent)
		}
	}

	// --- The characterisation: the link maps each subagent call to the
	// payloads' agent_type, exactly, for every subagent call and no other.
	if len(linked) != len(callType) {
		t.Fatalf("link covers %d subagent calls, payloads have %d: %v vs %v", len(linked), len(callType), linked, callType)
	}
	for id, want := range callType {
		got, ok := linked[id]
		if !ok {
			t.Fatalf("subagent call %s (agent_type %q) has no link through the transcripts", id, want)
		}
		// Byte for byte: Explore is "Explore" and general-purpose is
		// "general-purpose" in input.subagent_type, in toolUseResult.agentType
		// and in the payload. No mapping is needed for these two built-in
		// types; whether a custom or plugin-namespaced agent differs is not
		// shown here and nothing is invented for it.
		if got != want {
			t.Fatalf("subagent call %s: link recovers %q, payload agent_type is %q", id, got, want)
		}
		if callAgent[id] == "" || typeOfAgent[callAgent[id]] != want {
			t.Fatalf("subagent call %s: payload agent_id %q", id, callAgent[id])
		}
	}
	if linked[wantCall["Explore"].toolUseID] != "Explore" || linked[wantCall["general-purpose"].toolUseID] != "general-purpose" {
		t.Fatalf("link: Explore's call -> %q, general-purpose's call -> %q", linked[wantCall["Explore"].toolUseID], linked[wantCall["general-purpose"].toolUseID])
	}
	// The payload's agent_id is the transcript's agentId, too (the join key
	// is the same string in both).
	for agent := range typeOfAgent {
		if !seenAgent[agent] {
			t.Fatalf("payload agent_id %s is no transcript agentId", agent)
		}
	}
	// And the main agent's calls are not subagent calls: none of them is in
	// the link, so a reader cannot attribute them a type.
	for _, p := range payloads {
		if (p.tool == "Write" || p.tool == "Agent") && linked[p.toolUseID] != "" {
			t.Fatalf("main-agent call %s %s was linked to type %q", p.tool, p.toolUseID, linked[p.toolUseID])
		}
	}
}

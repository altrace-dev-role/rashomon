package acceptance

import "testing"

// H-89 -- subagent calls land in the parent turn and are counted separately.
//
// Break: drop the separate count and a turn that was half subagent work
// reads as if the main agent did all of it. A second break this item guards
// against: group by (transcript_path, prompt_id), the way chains.go groups
// for the causal view, instead of prompt_id alone -- a subagent's
// declarations carry the PARENT's prompt_id under the SUBAGENT's OWN
// transcript path, so that key silently splits them into a different bucket
// and the turn's total undercounts.
func TestH89_SubagentCallsLandInTheParentTurnAndAreCountedSeparately(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	main := defaultPayload()
	main.ToolUseID = "toolu_main1"
	e.mustHook(main.build(t))
	mainPost := defaultPost()
	mainPost.ToolUseID = "toolu_main1"
	e.mustPost(mainPost.build(t))

	subTranscript := "/tmp/transcripts/sess-1.jsonl/subagents/agent-7.jsonl"
	for i, id := range []string{"toolu_sub1", "toolu_sub2"} {
		sub := defaultPayload()
		sub.ToolUseID = id
		sub.AgentID = "agent-7"
		sub.AgentType = "explorer"
		// The parent's prompt_id ("prompt-1", defaultPayload's default),
		// under the SUBAGENT's own transcript path -- the exact shape a
		// (transcript, prompt) grouping mishandles.
		sub.TranscriptPath = subTranscript
		e.mustHook(sub.build(t))
		if i == 0 {
			post := defaultPost()
			post.ToolUseID = id
			post.TranscriptPath = subTranscript
			e.mustPost(post.build(t))
		}
	}

	d := e.digest("--session", testSession, "--prompt", "prompt-1")
	if d.Declarations.Recorded != 3 {
		t.Fatalf("recorded = %d, want 3 (1 main + 2 subagent, one turn). Break: group by "+
			"(transcript, prompt) instead of prompt_id alone and this drops to 1 -- the "+
			"subagent's calls fall into a different bucket under their own transcript path.",
			d.Declarations.Recorded)
	}
	if d.Subagents.Declarations != 2 {
		t.Errorf("subagents.declarations = %d, want 2", d.Subagents.Declarations)
	}
	if d.Subagents.Executions != 1 {
		t.Errorf("subagents.executions = %d, want 1", d.Subagents.Executions)
	}
	if d.Executions.Recorded != 2 {
		t.Errorf("executions.recorded = %d, want 2 (main + one subagent execution)", d.Executions.Recorded)
	}
}

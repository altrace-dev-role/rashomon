package report

import "testing"

// TestRubricNAMING1_BothReadersSkipAFinalSidechainLine holds the two final-
// message readers to one rule on a transcript whose last assistant line is a
// subagent's: a sidechain line is never the main agent's word. The plural
// skipped it while the singular -- the reader behind the report's account
// and its failure-word check -- returned the subagent's words as the
// session's final message.
func TestRubricNAMING1_BothReadersSkipAFinalSidechainLine(t *testing.T) {
	sidechain := turnLine(t, map[string]any{"type": "assistant", "isSidechain": true,
		"timestamp": "2026-09-29T10:00:03.000Z",
		"message": map[string]any{"id": "msg_sub", "role": "assistant",
			"content": []map[string]any{{"type": "text", "text": "Subagent: all tests failed."}}}})
	path := writeTurns(t,
		userTurnLine(t, "p1", "fix the build", false),
		said(t, "msg_main", "Fixed the build; tests pass.", "2026-09-29T10:00:02.000Z"),
		sidechain,
	)

	single, ok := FinalAssistantText(path)
	if !ok || single != "Fixed the build; tests pass." {
		t.Errorf("FinalAssistantText = %q, %v; want the main agent's last words", single, ok)
	}
	turns := FinalAssistantTexts(path, map[string]bool{"p1": true})
	if got := turns["p1"]; !got.Said || got.Text != single {
		t.Errorf("FinalAssistantTexts[p1] = %+v; want the same words FinalAssistantText returned (%q)", got, single)
	}
}

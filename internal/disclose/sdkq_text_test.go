package disclose

import (
	"strings"
	"testing"
)

// TestCSH1_TheBreadthParagraphSaysClaudePIsClaudeCode (SDK-GATE1, the
// manager's 11:2x condition): the full screen's breadth paragraph promises
// Agent SDK programs are recorded only after a y to the second question.
// Claude Code sets the sdk-cli entrypoint for claude -p and every other
// non-interactive run, and the gate records those as Claude Code, so a
// person who answers n must read which side claude -p is on: recorded
// whatever that answer is, and, like the SDK programs, given no notes.
// TestSDKG1's n-with-sdk-cli row is the test that the sentence is true.
// Break: drop the sentence, or say claude -p follows the answer, and this
// fails.
func TestCSH1_TheBreadthParagraphSaysClaudePIsClaudeCode(t *testing.T) {
	const want = "claude -p and other non-interactive Claude Code runs are Claude Code sessions, " +
		"not such programs: they are recorded whatever that answer is, and, like them, get no notes."
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var breadth []string
	for _, p := range inv.Paragraphs {
		if p.Owner == OwnerBreadth {
			breadth = append(breadth, p.Text)
		}
	}
	if len(breadth) == 0 {
		t.Fatal("premise: the full screen has no breadth paragraph")
	}
	for _, text := range breadth {
		if strings.Contains(text, "Agent SDK") && !strings.Contains(text, want) {
			t.Errorf("the breadth paragraph on Agent SDK programs does not say %q:\n%s", want, text)
		}
	}
}

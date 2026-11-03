package disclose

import (
	"strings"
	"testing"
)

// TestCSH1_TheBreadthParagraphSaysClaudePIsClaudeCode (SDK-GATE1, the
// manager's 11:2x condition; SDK-GATE2, the DE's 22:4x clause): the full
// screen's breadth paragraph promises Agent SDK programs are recorded only
// after a y to the second question. A person's own claude -p and every other
// non-interactive run Claude Code starts for a person are Claude Code and
// record whatever that answer is; a claude -p an Agent SDK program starts
// carries CLAUDE_AGENT_SDK_VERSION, is that program's, and follows the
// answer. A person who answers n must read both. TestSDKG1's n-with-sdk-cli
// row and TestSDKG2's rows are the tests that the sentences are true.
// Break: drop either, or say claude -p follows the answer without saying
// whose, and this fails.
func TestCSH1_TheBreadthParagraphSaysClaudePIsClaudeCode(t *testing.T) {
	wants := []string{
		"Your own claude -p and other non-interactive Claude Code runs are Claude Code sessions, " +
			"not such programs: they are recorded whatever that answer is, and, like them, get no notes",
		"a claude -p an Agent SDK program starts is that program's and follows your answer",
	}
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
		if !strings.Contains(text, "Agent SDK") {
			continue
		}
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("the breadth paragraph on Agent SDK programs does not say %q:\n%s", want, text)
			}
		}
	}
}

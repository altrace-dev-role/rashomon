package consent

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestCSH1_TheAgentSDKQuestionSaysClaudePIsRecordedEitherWay (SDK-GATE1,
// the DE's 11:2x addition; SDK-GATE2, the DE's 22:4x wording): the second
// question is the one text every person reads before answering it. A
// person's own claude -p is Claude Code and records whatever the answer; a
// claude -p an Agent SDK program starts is that program's (the gate reads
// CLAUDE_AGENT_SDK_VERSION), so the question says "your own". It is one line
// with room for the answer, so the screen it is asked under keeps its rows.
// Break: drop the clause, claim more ("always recorded"), or let the
// question wrap, and this fails.
func TestCSH1_TheAgentSDKQuestionSaysClaudePIsRecordedEitherWay(t *testing.T) {
	const want = "Record Agent SDK programs? Your own claude -p is recorded anyway. [y/N] "
	if QuestionSDK != want {
		t.Errorf("QuestionSDK = %q, want %q", QuestionSDK, want)
	}
	if strings.Contains(QuestionSDK, "\n") || utf8.RuneCountInString(QuestionSDK)+1 > Width {
		t.Errorf("QuestionSDK is not one line with room for the answer: %q", QuestionSDK)
	}
	if strings.Contains(QuestionSDK, "always") {
		t.Errorf("QuestionSDK claims claude -p is always recorded; it is recorded after the install's y: %q", QuestionSDK)
	}
}

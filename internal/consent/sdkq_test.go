package consent

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestCSH1_TheAgentSDKQuestionSaysClaudePIsRecordedEitherWay (SDK-GATE1,
// the DE's 11:2x addition): the second question is the one text every
// person reads before answering it, and Claude Code sets the sdk-cli
// entrypoint for claude -p, which the gate records as Claude Code. So the
// question says claude -p is recorded whichever way it is answered, on one
// line with room for the answer, so the screen it is asked under keeps its
// rows. Break: drop the clause, claim more ("always recorded"), or let the
// question wrap, and this fails.
func TestCSH1_TheAgentSDKQuestionSaysClaudePIsRecordedEitherWay(t *testing.T) {
	const want = "Also record Agent SDK programs? claude -p is recorded either way. [y/N] "
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

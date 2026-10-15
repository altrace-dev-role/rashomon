package report

import (
	"encoding/json"
	"strings"
	"testing"
)

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

	single, ok := FinalAssistantText(openT(t, path))
	if !ok || single != "Fixed the build; tests pass." {
		t.Errorf("FinalAssistantText = %q, %v; want the main agent's last words", single, ok)
	}
	turns := FinalAssistantTexts(openT(t, path), map[string]bool{"p1": true})
	if got := turns["p1"]; !got.Said || got.Text != single {
		t.Errorf("FinalAssistantTexts[p1] = %+v; want the same words FinalAssistantText returned (%q)", got, single)
	}
}

// TestRubricNAMING1_BothReadersUnsayAnUndecodableNonSidechainLine holds the
// two final-message readers to one rule on a transcript whose last
// assistant line fails to decode as either reader's header (a non-boolean
// isSidechain): neither reader may fall back to an earlier line's words,
// because the undecodable line itself may be the real final message. The
// plural reader already ends its tie on such a line (unsay); the singular
// reader used to skip only the failed decode and keep returning the
// assistant line before it.
func TestRubricNAMING1_BothReadersUnsayAnUndecodableNonSidechainLine(t *testing.T) {
	bad := turnLine(t, map[string]any{"type": "assistant", "isSidechain": "no",
		"timestamp": "2026-09-29T10:00:03.000Z",
		"message": map[string]any{"id": "msg_bad", "role": "assistant",
			"content": []map[string]any{{"type": "text", "text": "REAL FINAL"}}}})
	path := writeTurns(t,
		userTurnLine(t, "p1", "fix the build", false),
		said(t, "msg_main", "EARLIER", "2026-09-29T10:00:02.000Z"),
		bad,
	)

	single, ok := FinalAssistantText(openT(t, path))
	if ok {
		t.Errorf("FinalAssistantText = %q, %v; want no final message, not the line before the undecodable one", single, ok)
	}
	turns := FinalAssistantTexts(openT(t, path), map[string]bool{"p1": true})
	if got := turns["p1"]; got.Said {
		t.Errorf("FinalAssistantTexts[p1] = %+v; want Said false, matching FinalAssistantText", got)
	}
}

// FuzzFinalAssistantReadersAgree fuzzes the isSidechain value of a
// transcript's last line -- an otherwise ordinary assistant reply, "REAL
// FINAL", following one earlier reply, "EARLIER" -- against both
// final-message readers: whatever JSON value isSidechain holds, boolean,
// string, number, null, or a container, the two readers must reach the same
// answer, Said and Text alike. A string value ("no") found them disagreeing
// (TestRubricNAMING1_BothReadersUnsayAnUndecodableNonSidechainLine): the
// plural reader dropped the turn's words and the singular one kept
// reporting the reply before it.
func FuzzFinalAssistantReadersAgree(f *testing.F) {
	for _, v := range []string{`true`, `false`, `"no"`, `"yes"`, `null`, `0`, `1`, `[]`, `{}`, `"true"`} {
		f.Add([]byte(v))
	}
	f.Fuzz(func(t *testing.T, isSidechain []byte) {
		if !json.Valid(isSidechain) || strings.ContainsAny(string(isSidechain), "\n\r") {
			return
		}
		last := `{"type":"assistant","isSidechain":` + string(isSidechain) +
			`,"timestamp":"2026-09-29T10:00:03.000Z","message":{"id":"msg_last","role":"assistant",` +
			`"content":[{"type":"text","text":"REAL FINAL"}]}}`
		path := writeTurns(t,
			userTurnLine(t, "p1", "fix the build", false),
			said(t, "msg_main", "EARLIER", "2026-09-29T10:00:02.000Z"),
			last,
		)
		single, ok := FinalAssistantText(openT(t, path))
		got := FinalAssistantTexts(openT(t, path), map[string]bool{"p1": true})["p1"]
		if ok != got.Said || single != got.Text {
			t.Errorf("isSidechain %s: FinalAssistantText = %q, %v; FinalAssistantTexts = %q, %v", isSidechain, single, ok, got.Text, got.Said)
		}
	})
}

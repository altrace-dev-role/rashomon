package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// B1 -- a call the user DENIED is not a recording failure.
//
// Field-reported in `claude -p` mode: a denied call appeared BOTH as a
// declaration with no execution AND in executed_but_unrecorded, and added
// execution_mismatch to the coverage reasons. The second is a list whose whole
// purpose is to surface the PostToolUse recorder having failed to fire, so a
// denial was inflating a coverage-failure count -- the user exercising the
// permission prompt read as the tool being broken.
//
// The mechanism: a denial DOES produce a tool_result block, so its id landed in
// `results`, while no execution record exists because the call never ran.
//
// THE SIGNAL IS NOT STRUCTURED. There is no `denied` field. A denial and an
// ordinary tool failure both carry `is_error: true`; what separates them is the
// text Claude Code writes, and only at the START of it. Measured over 1,858 real
// transcripts on this machine: 14 denials, three variants, one invariant prefix.

// deniedPrefix is the text every denial begins with, as measured. The variants
// diverge after it, so only the prefix is matched.
const deniedPrefixFixture = "The user doesn't want to proceed with this tool use. " +
	"The tool use was rejected"

// The three variants observed in real transcripts, verbatim.
var denialVariants = []string{
	deniedPrefixFixture + " (eg. if it was a file edit, the new_string was NOT written to the " +
		"file). STOP what you are doing and wait for the user to tell you how to proceed.",
	deniedPrefixFixture + " (eg. if it was a file edit, the new_string was NOT written to the " +
		"file). STOP what you are doing and wait for the user to tell you how to proceed.\n\n" +
		"Note: The user's next message may contain a correction or preference. Pay close " +
		"attention -- if they explain what went wrong or how they'd prefer you to work, " +
		"consider saving that to memory for future sessions.",
	deniedPrefixFixture + " (eg. if it was a file edit, the new_string was NOT written to the " +
		"file). To tell you how to proceed, the user said:\nThe user wants to clarify.",
}

// blk is one content block in a transcript line.
type blk struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// writeTranscriptBlocks writes one transcript whose single user message carries
// the given blocks, plus an assistant tool_use for each id named.
func writeTranscriptBlocks(t *testing.T, blocks []blk) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.jsonl")

	var lines []map[string]any
	for _, b := range blocks {
		if b.ToolUseID != "" {
			lines = append(lines, map[string]any{"message": map[string]any{
				"role":    "assistant",
				"content": []blk{{Type: "tool_use", ID: b.ToolUseID}},
			}})
		}
	}
	lines = append(lines, map[string]any{"message": map[string]any{
		"role": "user", "content": blocks,
	}})

	var buf []byte
	for _, l := range lines {
		body, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		buf = append(buf, body...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestTranscript_EveryRealDenialVariantIsRecognised pins all three shapes seen
// in real transcripts. They share only the prefix, so matching the whole text
// would recognise one and miss two.
func TestTranscript_EveryRealDenialVariantIsRecognised(t *testing.T) {
	for i, text := range denialVariants {
		id := "toolu_denied_" + string(rune('a'+i))
		path := writeTranscriptBlocks(t, []blk{
			{Type: "tool_result", ToolUseID: id, IsError: true, Content: text},
		})

		_, results, denied, _, err := TranscriptIDs(path)
		if err != nil {
			t.Fatalf("variant %d: %v", i, err)
		}
		if !denied[id] {
			t.Errorf("variant %d was not recognised as a denial:\n  %.90s...", i, text)
		}
		if results[id] {
			t.Errorf("variant %d counted as a RESULT; it would then read as executed but "+
				"unrecorded, which is a recording failure and this is a user decision", i)
		}
	}
}

// TestTranscript_ADenialInABlockListIsRecognised. Transcript content is a plain
// string in some messages and an array of blocks in others; both shapes occur,
// and a denial written the second way must not slip through.
func TestTranscript_ADenialInABlockListIsRecognised(t *testing.T) {
	path := writeTranscriptBlocks(t, []blk{{
		Type: "tool_result", ToolUseID: "toolu_list", IsError: true,
		Content: []map[string]any{{"type": "text", "text": denialVariants[0]}},
	}})

	_, results, denied, _, err := TranscriptIDs(path)
	if err != nil {
		t.Fatal(err)
	}
	if !denied["toolu_list"] {
		t.Error("a denial whose content is a block list was not recognised")
	}
	if results["toolu_list"] {
		t.Error("it was counted as a result")
	}
}

// TestTranscript_AFailureQuotingTheDenialSentenceIsNotADenial is the negative
// fixture that matters most, and it is not hypothetical: writing this feature
// meant searching transcripts for that sentence, and the search's own output --
// which quoted it -- became a tool_result in this very session and matched.
//
// A command whose stderr happens to contain the sentence is a FAILURE. It ran.
// Counting it as a denial would hide a real execution behind a user decision,
// which is the same error as the bug this fixes, pointing the other way.
func TestTranscript_AFailureQuotingTheDenialSentenceIsNotADenial(t *testing.T) {
	cases := []struct {
		name    string
		isError bool
		content string
	}{
		{
			name:    "stderr quotes the sentence, prefix not at the start",
			isError: true,
			content: "Exit code 1\ngrep found: " + denialVariants[0],
		},
		{
			name:    "an analysis session prints the sentence, not an error at all",
			isError: false,
			content: denialVariants[0],
		},
		{
			name:    "the prefix appears mid-line in otherwise ordinary output",
			isError: true,
			content: "matched 14 lines: " + deniedPrefixFixture,
		},
		{
			name:    "an ordinary failure",
			isError: true,
			content: "Exit code 3",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTranscriptBlocks(t, []blk{
				{Type: "tool_result", ToolUseID: "toolu_x", IsError: tc.isError, Content: tc.content},
			})

			_, results, denied, _, err := TranscriptIDs(path)
			if err != nil {
				t.Fatal(err)
			}
			if denied["toolu_x"] {
				t.Errorf("counted as a denial. The call RAN; recording it as a user decision "+
					"hides a real execution:\n  %.110s", tc.content)
			}
			if !results["toolu_x"] {
				t.Error("it is a result and must still be counted as one")
			}
		})
	}
}

// TestTranscript_IsErrorAloneIsNotEnoughAndThePrefixAloneIsNotEnough states the
// conjunction directly, because each half on its own is a plausible shortcut
// and each is wrong in a different direction.
func TestTranscript_IsErrorAloneIsNotEnoughAndThePrefixAloneIsNotEnough(t *testing.T) {
	path := writeTranscriptBlocks(t, []blk{
		// is_error, denial text: the only combination that is a denial.
		{Type: "tool_result", ToolUseID: "toolu_both", IsError: true, Content: denialVariants[0]},
		// is_error, ordinary text.
		{Type: "tool_result", ToolUseID: "toolu_err", IsError: true, Content: "Exit code 1"},
		// no is_error, denial text.
		{Type: "tool_result", ToolUseID: "toolu_txt", IsError: false, Content: denialVariants[0]},
	})

	_, results, denied, _, err := TranscriptIDs(path)
	if err != nil {
		t.Fatal(err)
	}
	if !denied["toolu_both"] {
		t.Error("is_error AND the prefix is a denial, and was not recognised")
	}
	for _, id := range []string{"toolu_err", "toolu_txt"} {
		if denied[id] {
			t.Errorf("%s was called a denial on half the evidence", id)
		}
		if !results[id] {
			t.Errorf("%s is a result and must be counted as one", id)
		}
	}
}

// Refusals that never reach a person, verbatim from real transcripts on this
// machine. Recognising only the interactive prompt's wording made every one of
// these read as executed-but-unrecorded, so a healthy `-p` or auto-mode session
// turned unverified the moment anything was refused -- the same defect B1
// fixed for the prompt, in the modes B1's measurement never sampled.
var nonInteractiveRefusals = map[string]string{
	"-p approval": "This command requires approval",
	"auto mode classifier": "Permission for this action was denied by the Claude Code auto mode " +
		"classifier. Reason: Blocked a production deploy the user did not ask for.",
	"hook or policy":     "Permission for this action has been denied. Reason: terraform apply on production.",
	"settings deny rule": "Permission to use Bash with command ls .env* has been denied.",
	"classifier unreachable": "claude-sonnet-5[1m] is temporarily unavailable, so auto mode cannot " +
		"determine the safety of Bash right now. Wait briefly and then try this action again.",
}

func TestTranscript_RefusalsOutsideThePromptAreDenials(t *testing.T) {
	for name, text := range nonInteractiveRefusals {
		path := writeTranscriptBlocks(t, []blk{
			{Type: "tool_result", ToolUseID: "toolu_x", IsError: true, Content: text},
		})
		_, results, denied, _, err := TranscriptIDs(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !denied["toolu_x"] || results["toolu_x"] {
			t.Errorf("%s: denied=%v result=%v, want a denial and not a result:\n  %.90s",
				name, denied["toolu_x"], results["toolu_x"], text)
		}
	}
}

// TestTranscript_RefusalWordingInsideARealResultIsNotADenial holds the two
// guards the new wordings keep: is_error must be set, and the wording must open
// the result. A command that PRINTED one of these sentences ran.
func TestTranscript_RefusalWordingInsideARealResultIsNotADenial(t *testing.T) {
	for name, text := range nonInteractiveRefusals {
		for _, c := range []struct {
			why     string
			isError bool
			content string
		}{
			{"not an error", false, text},
			{"quoted mid-output", true, "grep matched: " + text},
		} {
			path := writeTranscriptBlocks(t, []blk{
				{Type: "tool_result", ToolUseID: "toolu_x", IsError: c.isError, Content: c.content},
			})
			_, _, denied, _, err := TranscriptIDs(path)
			if err != nil {
				t.Fatal(err)
			}
			if denied["toolu_x"] {
				t.Errorf("%s, %s: read as a denial, but the call ran", name, c.why)
			}
		}
	}
}

// TestTranscript_WorkflowSubagentTranscriptsAreRead: a workflow's subagents
// write <session>/subagents/workflows/<run>/agent-*.jsonl, two levels below the
// plain subagent files. A one-level glob read 35 of a real session's 407
// transcripts, and the 372 it skipped made every call they held read as
// recorded-but-not-in-the-transcript.
func TestTranscript_WorkflowSubagentTranscriptsAreRead(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "s.jsonl")
	write := func(path, id string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		line := `{"message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id + `"}]}}` + "\n"
		if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(main, "toolu_main")
	write(filepath.Join(dir, "s", "subagents", "agent-plain.jsonl"), "toolu_plain")
	write(filepath.Join(dir, "s", "subagents", "workflows", "run1", "agent-deep.jsonl"), "toolu_deep")

	ids, _, _, files, err := TranscriptIDs(main)
	if err != nil {
		t.Fatal(err)
	}
	if files != 3 {
		t.Errorf("read %d transcripts, want 3: the main one, a plain subagent, a workflow subagent", files)
	}
	for _, id := range []string{"toolu_main", "toolu_plain", "toolu_deep"} {
		if !ids[id] {
			t.Errorf("%s missing from the transcript ids", id)
		}
	}
}

package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
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

		_, results, denied, _, _, err := TranscriptIDs(path)
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

	_, results, denied, _, _, err := TranscriptIDs(path)
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

			_, results, denied, _, _, err := TranscriptIDs(path)
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

	_, results, denied, _, _, err := TranscriptIDs(path)
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
	// A Read, Edit or Write refused by a settings deny rule on its path. Unlike the
	// Bash rule above it is wrapped in tool_use_error tags and names neither
	// the tool nor the path. Verbatim from Claude Code 2.1.292, all five
	// settings-denied edits in a 100-run benchmark.
	"settings deny rule on a path": "<tool_use_error>File is in a directory that is denied by " +
		"your permission settings.</tool_use_error>",
}

func TestTranscript_RefusalsOutsideThePromptAreDenials(t *testing.T) {
	for name, text := range nonInteractiveRefusals {
		path := writeTranscriptBlocks(t, []blk{
			{Type: "tool_result", ToolUseID: "toolu_x", IsError: true, Content: text},
		})
		_, results, denied, _, _, err := TranscriptIDs(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !denied["toolu_x"] || results["toolu_x"] {
			t.Errorf("%s: denied=%v result=%v, want a denial and not a result:\n  %.90s",
				name, denied["toolu_x"], results["toolu_x"], text)
		}
	}
}

// TestTranscript_OnlyThePathRefusalIsUnhooked: a path-denied Edit is refused
// before PreToolUse, so the report must not ask the store for it. Every other
// refusal comes after PreToolUse and must stay out of the unhooked set, or an
// undeclared one would hide a declaration the recorder lost.
func TestTranscript_OnlyThePathRefusalIsUnhooked(t *testing.T) {
	for name, text := range nonInteractiveRefusals {
		path := writeTranscriptBlocks(t, []blk{
			{Type: "tool_result", ToolUseID: "toolu_x", IsError: true, Content: text},
		})
		_, _, _, unhooked, _, err := TranscriptIDs(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := name == "settings deny rule on a path"
		if unhooked["toolu_x"] != want {
			t.Errorf("%s: unhooked=%v, want %v", name, unhooked["toolu_x"], want)
		}
	}
}

// inputCheckRefusals are results Claude Code writes when it refuses a call
// while checking its input, verbatim from 2.1.280. No hook of any kind fired
// for any of them. Only the first is a rule refusing the call; the others are
// ordinary Edit and Write mistakes, and the check has many more messages that
// change between versions -- which is why the class is matched by the
// <tool_use_error> wrapper and not by a list of its sentences.
var inputCheckRefusals = map[string]string{
	"settings deny rule on a path": "<tool_use_error>File is in a directory that is denied by " +
		"your permission settings.</tool_use_error>",
	"old_string not in the file": "<tool_use_error>String to replace not found in file.\n" +
		"String: zzz</tool_use_error>",
	"file not read first": "<tool_use_error>File has not been read yet. Read it first before " +
		"writing to it.</tool_use_error>",
}

// TestTranscript_EveryInputCheckRefusalIsUnhooked: Claude Code refuses all of
// these before PreToolUse. Keying the set on one of their sentences left the
// others reading as executions the recorder missed.
func TestTranscript_EveryInputCheckRefusalIsUnhooked(t *testing.T) {
	for name, text := range inputCheckRefusals {
		path := writeTranscriptBlocks(t, []blk{
			{Type: "tool_result", ToolUseID: "toolu_x", IsError: true, Content: text},
		})
		_, _, _, unhooked, _, err := TranscriptIDs(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !unhooked["toolu_x"] {
			t.Errorf("%s: not kept as refused before any hook:\n  %.90s", name, text)
		}
	}
}

// TestTranscript_TheToolUseErrorTagMustOpenAnErrorResult holds the two guards
// the denials keep: is_error must be set, and the tag must open the result. A
// command that printed the tag ran.
func TestTranscript_TheToolUseErrorTagMustOpenAnErrorResult(t *testing.T) {
	for name, text := range inputCheckRefusals {
		for _, c := range []struct {
			why     string
			isError bool
			content string
		}{
			{"not an error", false, text},
			{"quoted mid-output", true, "Exit code 1\n" + text},
		} {
			path := writeTranscriptBlocks(t, []blk{
				{Type: "tool_result", ToolUseID: "toolu_x", IsError: c.isError, Content: c.content},
			})
			_, _, _, unhooked, _, err := TranscriptIDs(path)
			if err != nil {
				t.Fatal(err)
			}
			if unhooked["toolu_x"] {
				t.Errorf("%s, %s: kept as refused before any hook, but the call ran", name, c.why)
			}
		}
	}
}

// groupFor returns the accounting group for one transcript path.
func groupFor(t *testing.T, sess Session, path string) Transcript {
	t.Helper()
	for _, tr := range sess.Transcripts {
		if tr.Path == path {
			return tr
		}
	}
	t.Fatalf("no transcript group for %s in %d groups", path, len(sess.Transcripts))
	return Transcript{}
}

// TestAccounting_ARefusalNoHookSawIsNamedAndTheCountAddsUp: a call refused
// while its input was checked reached no hook, so it is neither missing from
// the store nor executed but unrecorded. It is named on its own list, so the
// transcript's ids reconcile with the store's again.
func TestAccounting_ARefusalNoHookSawIsNamedAndTheCountAddsUp(t *testing.T) {
	path := writeTranscriptBlocks(t, []blk{
		{Type: "tool_result", ToolUseID: "toolu_ran", Content: "ok"},
		{Type: "tool_result", ToolUseID: "toolu_path", IsError: true,
			Content: inputCheckRefusals["settings deny rule on a path"]},
		{Type: "tool_result", ToolUseID: "toolu_mistake", IsError: true,
			Content: inputCheckRefusals["old_string not in the file"]},
	})
	sess := build(&store.Run{
		Declarations: []store.Declaration{
			{ToolUseID: "toolu_ran", ToolName: "Bash", SessionID: "s", TranscriptPath: path},
		},
		Executions: []store.Execution{{ToolUseID: "toolu_ran", SessionID: "s"}},
	})
	tr := groupFor(t, sess, path)

	if want := []string{"toolu_mistake", "toolu_path"}; !slices.Equal(tr.RefusedBeforeHooks, want) {
		t.Errorf("refused_before_hooks = %v, want %v", tr.RefusedBeforeHooks, want)
	}
	if len(tr.MissingFromStore) != 0 || len(tr.ExecutedButUnrecorded) != 0 {
		t.Errorf("missing_from_store = %v, executed_but_unrecorded = %v: no hook fires for "+
			"either call, so neither absence is a recorder gap",
			tr.MissingFromStore, tr.ExecutedButUnrecorded)
	}
	if tr.IDsInTranscript == nil {
		t.Fatal("transcript not read")
	}
	if got := tr.IDsRecorded + len(tr.MissingFromStore) + len(tr.RefusedBeforeHooks) -
		len(tr.MissingFromTranscript); got != *tr.IDsInTranscript {
		t.Errorf("ids in transcript %d, but recorded + missing from store + refused - missing "+
			"from transcript = %d", *tr.IDsInTranscript, got)
	}
	for _, r := range sess.Coverage.Reasons {
		if r == ReasonTranscriptMismatch || r == ReasonExecutionMismatch {
			t.Errorf("coverage carries %s for calls no hook could see: %v", r, sess.Coverage.Reasons)
		}
	}
}

// TestAccounting_AnyHookRecordTakesTheExemptionAway: the wrapper alone is not
// proof, because Claude Code also wraps a few errors raised after PreToolUse.
// A call any hook left a record of -- a declaration under this path or
// another, a terminal that outlived a declaration lost to the lock, or an
// execution -- is held to the store exactly as before.
func TestAccounting_AnyHookRecordTakesTheExemptionAway(t *testing.T) {
	text := inputCheckRefusals["old_string not in the file"]
	path := writeTranscriptBlocks(t, []blk{
		{Type: "tool_result", ToolUseID: "toolu_declared", IsError: true, Content: text},
		{Type: "tool_result", ToolUseID: "toolu_elsewhere", IsError: true, Content: text},
		{Type: "tool_result", ToolUseID: "toolu_terminal", IsError: true, Content: text},
		{Type: "tool_result", ToolUseID: "toolu_executed", IsError: true, Content: text},
	})
	sess := build(&store.Run{
		Declarations: []store.Declaration{
			{ToolUseID: "toolu_declared", ToolName: "Edit", SessionID: "s", TranscriptPath: path},
			{ToolUseID: "toolu_elsewhere", ToolName: "Edit", SessionID: "s", TranscriptPath: path + ".other"},
		},
		Terminals:  []store.Terminal{{ToolUseID: "toolu_terminal", SessionID: "s"}},
		Executions: []store.Execution{{ToolUseID: "toolu_executed", SessionID: "s"}},
	})
	tr := groupFor(t, sess, path)

	if len(tr.RefusedBeforeHooks) != 0 {
		t.Errorf("refused_before_hooks = %v; a hook saw every one of these calls", tr.RefusedBeforeHooks)
	}
	if want := []string{"toolu_elsewhere", "toolu_executed", "toolu_terminal"}; !slices.Equal(tr.MissingFromStore, want) {
		t.Errorf("missing_from_store = %v, want %v", tr.MissingFromStore, want)
	}
	if want := []string{"toolu_declared", "toolu_elsewhere", "toolu_terminal"}; !slices.Equal(tr.ExecutedButUnrecorded, want) {
		t.Errorf("executed_but_unrecorded = %v, want %v", tr.ExecutedButUnrecorded, want)
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
			_, _, denied, _, _, err := TranscriptIDs(path)
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

	ids, _, _, _, files, err := TranscriptIDs(main)
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

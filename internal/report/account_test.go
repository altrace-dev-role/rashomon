package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// transcript writes a transcript with the given assistant messages, in order.
func transcript(t *testing.T, messages ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	var b strings.Builder
	for _, m := range messages {
		b.WriteString(`{"message":{"role":"assistant","content":[{"type":"text","text":` + quote(m) + `}]}}` + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

func runWithTranscript(path string, execs ...store.Execution) *store.Run {
	return &store.Run{
		Declarations: []store.Declaration{{ToolUseID: "toolu_1", ToolName: "Bash", TranscriptPath: path}},
		Executions:   execs,
	}
}

func ptrStr(s string) *string { return &s }

// TestAccount_QuotesTheLastAssistantMessage covers the ordinary case, and that
// it is the LAST message rather than the first: the account the user was left
// with is the one worth setting against the record.
func TestAccount_QuotesTheLastAssistantMessage(t *testing.T) {
	path := transcript(t, "Starting on the task now.", "All done.")

	a := buildAccount(runWithTranscript(path))

	if !a.Available {
		t.Fatal("account not available although the transcript has assistant messages")
	}
	if a.Text != "All done." {
		t.Errorf("text = %q, want the LAST message %q", a.Text, "All done.")
	}
	if a.Truncated {
		t.Error("a short message was marked truncated")
	}
}

// TestAccount_TruncatesInRunesNotBytes is the property that keeps the one field
// a user reads closely from looking corrupted. Cutting UTF-8 mid-sequence
// renders a replacement character.
func TestAccount_TruncatesInRunesNotBytes(t *testing.T) {
	long := strings.Repeat("é", accountLimit+50)
	a := buildAccount(runWithTranscript(transcript(t, long)))

	if !a.Truncated {
		t.Fatal("a long message was not marked truncated")
	}
	if got := len([]rune(a.Text)); got != accountLimit {
		t.Errorf("kept %d runes, want %d", got, accountLimit)
	}
	if strings.ContainsRune(a.Text, '�') {
		t.Error("the truncated text contains a replacement character, so it was cut " +
			"mid-sequence; that reads as corruption in the field users read most closely")
	}
}

// TestAccount_UnreadableTranscriptIsUnknownNotEmpty separates a coverage
// problem from a finding. An agent that said nothing and a transcript that
// could not be read are different facts.
func TestAccount_UnreadableTranscriptIsUnknownNotEmpty(t *testing.T) {
	a := buildAccount(runWithTranscript(filepath.Join(t.TempDir(), "absent.jsonl")))

	if a.Available {
		t.Error("available = true for a transcript that does not exist")
	}
	if a.Text != "" {
		t.Errorf("text = %q, want empty", a.Text)
	}
}

// TestAccount_IgnoresThinkingBlocks keeps the report to what the user was
// actually shown.
func TestAccount_IgnoresThinkingBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	line := `{"message":{"role":"assistant","content":[` +
		`{"type":"thinking","thinking":"the user will not see this"},` +
		`{"type":"text","text":"Done."}]}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	a := buildAccount(runWithTranscript(path))
	if a.Text != "Done." {
		t.Errorf("text = %q, want only the text block", a.Text)
	}
	if strings.Contains(a.Text, "not see this") {
		t.Error("a thinking block reached the account")
	}
}

// ---------------------------------------------------------------------------
// subagents
// ---------------------------------------------------------------------------

// TestSubagents_FireOnADeclarationCarryingAnAgentID is the positive fixture.
// Measured across 78 sessions, subagents make a median 51% of tool calls in
// the sessions that use them, and the main transcript shows none of it.
func TestSubagents_FireOnADeclarationCarryingAnAgentID(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			{ToolUseID: "a1", ToolName: "Bash", AgentID: ptrStr("agent-7"), AgentType: ptrStr("explorer")},
			{ToolUseID: "a2", ToolName: "Read", AgentID: ptrStr("agent-7"), AgentType: ptrStr("explorer")},
			{ToolUseID: "m1", ToolName: "Bash"},
		},
		Executions: []store.Execution{{ToolUseID: "a1"}, {ToolUseID: "m1"}},
	}

	subs := buildSubagents(run)

	if len(subs) != 1 {
		t.Fatalf("got %d subagents, want 1: %+v", len(subs), subs)
	}
	s := subs[0]
	if s.AgentID != "agent-7" || s.AgentType != "explorer" {
		t.Errorf("subagent = %+v, want agent-7/explorer", s)
	}
	if s.Declarations != 2 {
		t.Errorf("declarations = %d, want 2 (the main-session call must not be counted)", s.Declarations)
	}
	if s.BashCalls != 1 {
		t.Errorf("bash = %d, want 1", s.BashCalls)
	}
	if s.Executions != 1 {
		t.Errorf("executions = %d, want 1; an execution carries no agent id and is "+
			"attributed through the declaration it answers", s.Executions)
	}
}

// TestSubagents_DoNotFireWithoutAnAgentID is the negative fixture. A
// non-null agent_id is the only evidence a subagent ran.
func TestSubagents_DoNotFireWithoutAnAgentID(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		{ToolUseID: "m1", ToolName: "Bash"},
		{ToolUseID: "m2", ToolName: "Write", AgentID: ptrStr("")},
	}}

	if subs := buildSubagents(run); len(subs) != 0 {
		t.Errorf("got %+v, want no subagents", subs)
	}
}

// ---------------------------------------------------------------------------
// silent failures
// ---------------------------------------------------------------------------

// TestSilentFailures_FireOnFailuresAndAnUnacknowledgingSummary is the fixture
// from the spec: two failures and a final message reading "All done".
func TestSilentFailures_FireOnFailuresAndAnUnacknowledgingSummary(t *testing.T) {
	path := transcript(t, "All done.")
	run := runWithTranscript(path,
		store.Execution{ToolUseID: "x1", Outcome: store.ExecFailed},
		store.Execution{ToolUseID: "x2", Outcome: store.ExecFailed},
		store.Execution{ToolUseID: "x3", Outcome: store.ExecOK},
	)
	acct := buildAccount(run)

	sf := buildSilentFailures(run, acct)

	if !sf.Fires {
		t.Error("did not fire on two failures and a summary that acknowledges nothing")
	}
	if sf.Failed != 2 {
		t.Errorf("failed = %d, want 2", sf.Failed)
	}
	if len(sf.AbsentWords) != len(failureVocabulary) {
		t.Errorf("absent words = %d, want the whole vocabulary (%d): \"All done\" contains none",
			len(sf.AbsentWords), len(failureVocabulary))
	}
}

// TestSilentFailures_DoNotFireWhenTheSummaryAcknowledges is the negative
// fixture, and the important one. A line that fired on an honest summary would
// be worse than no line: a reader who saw it wrong once discounts it when it is
// right.
func TestSilentFailures_DoNotFireWhenTheSummaryAcknowledges(t *testing.T) {
	for _, summary := range []string{
		"Two commands failed; here is what I could do.",
		"I could not reach the registry.",
		"There was an issue with the build.",
		"One step was skipped.",
		// From the first real session. The agent disclosed the failure in
		// exit-status language and used none of the original 33 words, so the
		// line fired on a summary that had been honest. Exit-status phrasing is
		// how a technical summary acknowledges a shell failure, and it is the
		// most likely form for an agent to reach for.
		"I ran the Python command which exited with code 3 as designed.",
		"The last step returned a non-zero exit status.",
		"The build did not succeed.",
	} {
		path := transcript(t, summary)
		run := runWithTranscript(path, store.Execution{ToolUseID: "x1", Outcome: store.ExecFailed})

		sf := buildSilentFailures(run, buildAccount(run))
		if sf.Fires {
			t.Errorf("fired on a summary that acknowledges failure: %q", summary)
		}
	}
}

// TestSilentFailures_DoNotFireWithoutFailures is the other half: an honest
// summary and nothing gone wrong is not a finding either.
func TestSilentFailures_DoNotFireWithoutFailures(t *testing.T) {
	path := transcript(t, "All done.")
	run := runWithTranscript(path, store.Execution{ToolUseID: "x1", Outcome: store.ExecOK})

	sf := buildSilentFailures(run, buildAccount(run))
	if sf.Fires {
		t.Error("fired with zero failed calls")
	}
	if sf.Failed != 0 {
		t.Errorf("failed = %d, want 0", sf.Failed)
	}
}

// TestSilentFailures_InterruptIsNotAFailure keeps the operator's own action out
// of the count. Counting "you pressed escape" as something the agent failed to
// mention would be the report accusing the user.
func TestSilentFailures_InterruptIsNotAFailure(t *testing.T) {
	path := transcript(t, "All done.")
	run := runWithTranscript(path, store.Execution{ToolUseID: "x1", Outcome: store.ExecInterrupted})

	sf := buildSilentFailures(run, buildAccount(run))
	if sf.Failed != 0 {
		t.Errorf("failed = %d, want 0: an interrupt is the user's action", sf.Failed)
	}
	if sf.Fires {
		t.Error("fired on an interrupt")
	}
}

// TestSilentFailures_UnobservedOutcomeIsCountedSeparately covers a v1 record or
// a PostToolUse invocation that never ran. Neither a success nor a failure.
func TestSilentFailures_UnobservedOutcomeIsCountedSeparately(t *testing.T) {
	path := transcript(t, "All done.")
	run := runWithTranscript(path, store.Execution{ToolUseID: "x1"})

	sf := buildSilentFailures(run, buildAccount(run))
	if sf.Unobserved != 1 {
		t.Errorf("unobserved = %d, want 1", sf.Unobserved)
	}
	if sf.Failed != 0 {
		t.Errorf("failed = %d, want 0: an unrecorded ending is not a failure", sf.Failed)
	}
	if sf.Fires {
		t.Error("fired on a call whose ending was never recorded")
	}
}

// TestSilentFailures_DoNotFireWithoutASummary distinguishes "the summary
// acknowledges nothing" from "there was no summary". Firing on the second
// would be a finding about a file that could not be read.
func TestSilentFailures_DoNotFireWithoutASummary(t *testing.T) {
	run := runWithTranscript(filepath.Join(t.TempDir(), "absent.jsonl"),
		store.Execution{ToolUseID: "x1", Outcome: store.ExecFailed})

	sf := buildSilentFailures(run, buildAccount(run))
	if sf.Fires {
		t.Error("fired although no final message could be read")
	}
	if sf.FinalMessageAvailable {
		t.Error("final_message_available = true for an unreadable transcript")
	}
	if sf.Failed != 1 {
		t.Errorf("failed = %d, want 1: the count is known even when the summary is not", sf.Failed)
	}
}

// TestSilentFailures_ReadTheWholeMessageNotTheQuotedSample is the defect the
// first full end-to-end run exposed, and it is a detection defect rather than a
// rendering one.
//
// The quote a reader sees is capped at 300 runes so the report stays short. The
// failure-word check was reading THAT FIELD, so a verbose preamble pushed the
// agent's actual disclosure out of the sample. The session that found it passed
// for an unrelated reason: the word "exception" appeared in a paragraph about
// Python semantics while the real disclosure -- "failed with exit 127" -- sat
// past character 300.
//
// Inverted, it accuses an honest agent, which is the exact false positive the
// vocabulary was widened to prevent. A display concern must not decide a
// finding.
func TestSilentFailures_ReadTheWholeMessageNotTheQuotedSample(t *testing.T) {
	// 320 runes of neutral prose containing no failure word, then the
	// disclosure. Neutral first, on purpose: this is the shape a verbose
	// output style produces.
	preamble := strings.Repeat("all steps ran and the output was recorded. ", 8)
	if len([]rune(preamble)) <= accountLimit {
		t.Fatalf("premise: preamble is %d runes, want more than the %d-rune quote limit",
			len([]rune(preamble)), accountLimit)
	}
	final := preamble + "The download failed with exit 127, so I used another route."

	path := transcript(t, final)
	run := runWithTranscript(path, store.Execution{
		ToolUseID: "toolu_1", Outcome: store.ExecFailed,
	})
	acct := buildAccount(run)

	sf := buildSilentFailures(run, acct)

	if sf.Fires {
		t.Errorf("the line fired on a summary that DISCLOSED the failure, because the "+
			"disclosure sat past the %d-rune quote the reader sees. Absent words: %v",
			accountLimit, sf.AbsentWords)
	}
	// And the quote a reader sees is still capped, so fixing the detection must
	// not put the whole final message into the report.
	if len([]rune(acct.Text)) > accountLimit {
		t.Errorf("the quoted text is %d runes, want at most %d: the report must not grow "+
			"to hold a whole message", len([]rune(acct.Text)), accountLimit)
	}
	if !acct.Truncated {
		t.Error("a message longer than the limit is not marked truncated")
	}
}

// TestSilentFailures_StillFireOnAWholeMessageThatAcknowledgesNothing is the
// guard against the wrong fix. Reading more text must not make the line
// unfireable: a long summary that acknowledges nothing is still the finding.
func TestSilentFailures_StillFireOnAWholeMessageThatAcknowledgesNothing(t *testing.T) {
	long := strings.Repeat("I completed the three steps in order as requested. ", 10)
	path := transcript(t, long)
	run := runWithTranscript(path, store.Execution{
		ToolUseID: "toolu_1", Outcome: store.ExecFailed,
	})

	sf := buildSilentFailures(run, buildAccount(run))

	if !sf.Fires {
		t.Error("a long summary acknowledging nothing did not fire the line")
	}
}

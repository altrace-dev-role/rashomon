package report

import (
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// accountLimit is how much of the final message is rendered.
//
// 300 characters is enough to see what the agent claimed and short enough that
// the report is not a transcript viewer. The limit is applied in RUNES, not
// bytes, so a multi-byte character is never cut in half -- a truncated UTF-8
// sequence would render as a replacement character and look like corruption in
// the one field a user is most likely to read closely.
const accountLimit = 300

// Account is the agent's own summary, placed beside the record.
//
// The point is not to catch the model out. A summary and a set of records are
// two descriptions of one session, and only a reader can reconcile them; the
// report's job is to put them side by side rather than to reach a verdict.
//
// It is read from the transcript at RENDER time and never written to the store.
// The store holds identifiers and hostnames; this is content, it stays on the
// machine, and it exists only for as long as the process that printed it.
type Account struct {
	// Available is false when no assistant message could be read, which is a
	// different fact from an empty summary: an unreadable transcript is a
	// coverage problem and a silent agent is a finding.
	Available bool   `json:"available"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// SubagentSummary is what one subagent did that the main transcript never
// shows.
//
// Measured across 78 sessions: 22% use subagents, and in those the subagents
// make a median 51% of all tool calls. A report built from the main transcript
// alone therefore understates those sessions by about half, and nothing in the
// main transcript says so.
type SubagentSummary struct {
	AgentID      string `json:"agent_id"`
	AgentType    string `json:"agent_type"`
	Declarations int    `json:"declarations"`
	Executions   int    `json:"executions"`
	BashCalls    int    `json:"bash_calls"`
}

// SilentFailures is the count of failed calls set against whether the final
// message mentions failure at all.
//
// It is a fact about TEXT and never about intent. The rendered line says how
// many calls failed and which words are absent from the summary; it does not
// say the agent concealed anything, because this program cannot know that and
// a tool that guesses at it would be worth less than one that does not. A test
// greps this package for the words that would cross that line.
type SilentFailures struct {
	// Fires is true only when there were failures AND none of the vocabulary
	// appears. Both halves are required: failures with an honest summary are
	// not a finding, and an honest summary with no failures is not either.
	Fires bool `json:"fires"`
	// Failed counts executions whose outcome is failed. Interrupted is
	// deliberately excluded: a user pressing escape is not something the agent
	// failed to mention.
	Failed int `json:"failed"`
	// Unobserved counts executions with no outcome at all -- v1 records, or a
	// PostToolUse invocation that never ran. They cannot be counted as
	// successes or as failures, and saying so is the honest answer.
	Unobserved int `json:"outcome_unobserved"`
	// AbsentWords is the exact list of failure words missing from the final
	// message, which is what makes the line checkable by the reader rather
	// than a conclusion they have to accept.
	AbsentWords []string `json:"absent_words"`
	// FinalMessageAvailable distinguishes "the summary mentions no failure"
	// from "there was no summary to read".
	FinalMessageAvailable bool `json:"final_message_available"`
}

// failureVocabulary is the fixed list checked against the final message.
//
// Deliberately broad. A wide list makes the line CONSERVATIVE: the more words
// counted as acknowledgement, the harder it is for the line to fire, so a
// false positive is the expensive error and breadth is the defence against it.
// Measured on 78 sessions with a list of this shape, 14% of sessions ended with
// a final message containing none of these words -- a floor, not a headline.
var failureVocabulary = []string{
	"fail", "failed", "failing", "error", "errors", "couldn't", "could not",
	"unable", "not able", "didn't", "did not", "blocked", "denied", "timed out",
	"timeout", "broken", "broke", "issue", "issues", "problem", "problems",
	"bug", "crash", "missing", "cannot", "can't", "warning", "exception",
	"permission", "refused", "interrupted", "skipped", "partial",
}

// buildAccount reads the agent's summary for a run.
//
// The transcript path comes from the run's own declarations, not from a flag or
// from configuration: the path a session recorded is the path that session
// wrote, and asking anywhere else would risk rendering one session's summary
// against another's records.
func buildAccount(run *store.Run) Account {
	for _, d := range run.Declarations {
		if d.TranscriptPath == "" {
			continue
		}
		text, ok := FinalAssistantText(d.TranscriptPath)
		if !ok {
			continue
		}
		runes := []rune(text)
		if len(runes) > accountLimit {
			return Account{Available: true, Text: string(runes[:accountLimit]), Truncated: true}
		}
		return Account{Available: true, Text: text}
	}
	return Account{}
}

// buildSubagents groups declarations by the agent that made them.
//
// It fires only when a declaration carries a non-null agent_id, because that is
// the only evidence a subagent ran. An empty list on a session with no
// subagents is correct and says nothing; an empty list on a session WITH them
// would be the under-count this section exists to close.
func buildSubagents(run *store.Run) []SubagentSummary {
	type key struct{ id, typ string }
	agg := map[key]*SubagentSummary{}
	var order []key

	for _, d := range run.Declarations {
		if d.AgentID == nil || *d.AgentID == "" {
			continue
		}
		k := key{id: *d.AgentID}
		if d.AgentType != nil {
			k.typ = *d.AgentType
		}
		s, ok := agg[k]
		if !ok {
			s = &SubagentSummary{AgentID: k.id, AgentType: k.typ}
			agg[k] = s
			order = append(order, k)
		}
		s.Declarations++
		if d.ToolName == "Bash" {
			s.BashCalls++
		}
	}
	if len(agg) == 0 {
		return []SubagentSummary{}
	}

	// Executions carry no agent id, so they are attributed through the
	// declaration they answer. A count derived from the execution records
	// alone would be unattributable.
	owner := map[string]key{}
	for _, d := range run.Declarations {
		if d.AgentID == nil || *d.AgentID == "" {
			continue
		}
		k := key{id: *d.AgentID}
		if d.AgentType != nil {
			k.typ = *d.AgentType
		}
		owner[d.ToolUseID] = k
	}
	for _, x := range run.Executions {
		if k, ok := owner[x.ToolUseID]; ok {
			agg[k].Executions++
		}
	}

	sort.Slice(order, func(i, j int) bool {
		if order[i].id != order[j].id {
			return order[i].id < order[j].id
		}
		return order[i].typ < order[j].typ
	})
	out := make([]SubagentSummary, 0, len(order))
	for _, k := range order {
		out = append(out, *agg[k])
	}
	return out
}

// buildSilentFailures compares the failure count against the final message.
func buildSilentFailures(run *store.Run, acct Account) SilentFailures {
	sf := SilentFailures{
		AbsentWords:           []string{},
		FinalMessageAvailable: acct.Available,
	}
	for _, x := range run.Executions {
		switch x.Outcome {
		case store.ExecFailed:
			sf.Failed++
		case "":
			// A v1 record, or an invocation that recorded no ending. Neither a
			// success nor a failure, and counting it as either would invent a
			// fact.
			sf.Unobserved++
		}
	}
	if sf.Failed == 0 {
		return sf
	}

	lower := strings.ToLower(acct.Text)
	var present bool
	for _, w := range failureVocabulary {
		if acct.Available && strings.Contains(lower, w) {
			present = true
			continue
		}
		sf.AbsentWords = append(sf.AbsentWords, w)
	}
	// Fires only when the summary EXISTS and acknowledges nothing. With no
	// summary there is no claim to set the failures against, and firing would
	// be a finding about a file that could not be read.
	sf.Fires = acct.Available && !present
	return sf
}

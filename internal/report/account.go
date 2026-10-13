package report

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/altrace-dev-role/rashomon/internal/shape"
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

	// full is the whole final message, kept for the failure-word check and
	// never rendered, never persisted, and deliberately absent from the JSON.
	//
	// Two fields rather than one because the 300-rune cap is a DISPLAY decision
	// and the check is a DETECTION decision, and sharing the field let the first
	// silently constrain the second: a verbose preamble pushed an agent's
	// disclosure out of the sample, so the line fired on an honest summary.
	//
	// It stays unexported and out of the JSON on purpose. The account is prose
	// that may name anything -- which is why --redact drops it entirely -- so
	// widening what is ANALYSED must not widen what is written down. This field
	// lives for the duration of one render.
	full string
}

// analysed is the text the failure-word check reads: the whole message when the
// account came from a transcript, and the quote when an Account was built by
// hand. Production always goes through buildAccount, which sets full.
func (a Account) analysed() string {
	if a.full != "" {
		return a.full
	}
	return a.Text
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
// It is a fact about TEXT and never about intent. The end-of-turn line
// prints the turn's count less the lookups, and how many it set aside; the
// report prints "failed calls" with the lookups included and lists the absent
// words. Neither says the agent
// concealed anything, because this program cannot know that and a tool that
// guesses at it would be worth less than one that does not. A test greps this
// package for the words that would cross that line.
type SilentFailures struct {
	// Fires is true only when there were failures other than lookups AND
	// none of the vocabulary appears. Both halves are required: failures with
	// an honest summary are not a finding, and an honest summary with no
	// failures is not either.
	Fires bool `json:"fires"`
	// Failed counts executions whose outcome is failed, lookups included.
	// Interrupted is deliberately excluded: a user pressing escape is not
	// something the agent failed to mention.
	Failed int `json:"failed"`
	// FailedLookups counts the calls among Failed made by a lookup tool
	// (IsLookup: Read, Glob, Grep, NotebookRead). They stay in Failed and in
	// the report, and they never make the line fire by themselves, nor count
	// in the number it prints (Counted).
	//
	// The rule is by tool name, so it covers every error of those tools: a
	// Read of a directory or of a missing file, a Glob of a missing folder, a
	// Grep whose pattern ripgrep rejects. A Glob or Grep that matches nothing
	// succeeds and never reaches this count. Only EISDIR was measured: on a
	// 100-run benchmark, 5 of the 7 Haiku runs where the line fired on a true
	// report rested on one failed Read of a directory and nothing else, and one
	// run it "caught" was caught by such a Read rather than by the failure it
	// hid. The record holds the tool name and the outcome and nothing of the
	// error, so the rule cannot be narrowed to one error; whether to keep the
	// error is an open decision. An Edit whose old string was not found, a
	// Write, a shell command, a subagent, a fetch and an MCP tool all still
	// count, and so does a shell `cat` of a missing file.
	//
	// The cost: a turn whose only failures are lookups does not fire even when
	// it stopped at one and reported success -- a review of a file it never
	// read -- though the report still lists the call.
	FailedLookups int `json:"failed_lookups"`
	// Unobserved counts executions with no outcome at all -- v1 records, or a
	// PostToolUse invocation that never ran -- and executions moved to the
	// background (Backgrounded), whose PostToolUse fired before the command
	// ended. They cannot be counted as successes or as failures, and saying so
	// is the honest answer.
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

	// Added after the first end-to-end run against a real session, where the
	// line fired on a summary that HAD disclosed the failure: the agent wrote
	// "ran the Python command which exited with code 3 as designed", which
	// contains none of the words above. The line was literally true and
	// invited exactly the wrong inference.
	//
	// That is the failure mode this list exists to prevent, and it is why the
	// rendered line prints the absent words: a reader could see that "exited
	// with code 3" was not among them. Printing the list saved the line from
	// being wrong; widening it stops the situation arising.
	//
	// Exit-status language is how a technical summary acknowledges a failure
	// without using a failure word, and it is the single most likely form for
	// an agent reporting a shell command.
	"exit code", "exit status", "exited", "non-zero", "nonzero",
	"returned 1", "did not succeed", "unsuccessful", "aborted", "rejected",
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
		return finishAccount(text)
	}
	return Account{}
}

// finishAccount applies the one rule both accessors to a final message share:
// the field a person reads is capped at accountLimit runes, and the field the
// failure-word check reads is not.
func finishAccount(text string) Account {
	runes := []rune(text)
	if len(runes) > accountLimit {
		return Account{Available: true, Text: string(runes[:accountLimit]), Truncated: true, full: text}
	}
	return Account{Available: true, Text: text, full: text}
}

// messageCap bounds a caller-supplied final message before it is used at all.
//
// A digest never opens a transcript (see AccountFromMessage), so nothing
// here already bounded the size of what arrives -- the transcript reader's
// maxLine did that job for buildAccount. 64 KiB is far past any real final
// message and exists only to keep a pathological input from costing more than
// a bounded string comparison should.
const messageCap = 64 << 10

// AccountFromMessage builds an Account from a message the CALLER already has,
// rather than one read from a transcript.
//
// buildAccount's FinalAssistantText reads the transcript's LAST assistant
// message -- a whole-session, whole-file read. A turn digest must not do
// that: the message is session-wide while the digest is turn-scoped, so
// checking a turn's failures against it would set an early turn's findings
// against a later turn's words, and at Stop the file may not even hold the
// final line yet, which is a coverage problem wearing a content-shaped
// costume. The caller of `rashomon digest` -- a future Stop hook -- already
// has the message in the hook payload's own last_assistant_message field, and
// handing it here means this package never opens a second content path to get
// the same fact.
//
// The text is never rendered by the digest -- see Account.full's comment: it
// exists for the failure-word CHECK, not for display, and the digest carries
// only booleans and counts derived from it (SilentFailures), never the
// message itself. It is still sanitised before that comparison runs: the
// message is attacker-influenceable (a poisoned page or repo can end up
// quoted back by the model), so control and other non-graphic runes are
// stripped first. strings.Contains cannot be corrupted by a crafted byte
// sequence, but stripping keeps this function's OWN behaviour independent of
// bytes it did not choose to accept, which is the same posture the rest of
// this package takes toward content it is handed rather than content it
// generated.
func AccountFromMessage(msg string) Account {
	msg = sanitizeMessage(msg)
	if msg == "" {
		return Account{}
	}
	return finishAccount(msg)
}

// sanitizeMessage caps length and drops non-graphic runes other than
// whitespace. Applied before the message is compared against anything, not
// applied to anything this package renders -- the digest never renders this
// text at all.
func sanitizeMessage(msg string) string {
	runes := []rune(msg)
	if len(runes) > messageCap {
		runes = runes[:messageCap]
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsGraphic(r) || unicode.IsSpace(r) {
			return r
		}
		return -1
	}, string(runes))
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

// lookupToolNames is how the report names the tools IsLookup counts, read
// from the same list IsLookup is (shape.ReadTools), so the two cannot drift.
var lookupToolNames = orList(shape.ReadTools())

// LookupToolNames is how a reader is told which tools are lookups: "Read,
// Glob, Grep or NotebookRead". spend prints it beside a "none found".
func LookupToolNames() string { return lookupToolNames }

// orList joins names as "A, B or C".
func orList(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// IsLookup reports whether a failed call to toolName is a lookup: one of the
// tools shape classes read by name alone (Read, Glob, Grep, NotebookRead),
// whatever the error was. A shell tool is not one whatever it runs, nor is
// an MCP tool, whatever its name says: the record cannot tell what either
// touched. See SilentFailures.FailedLookups for why a lookup alone does not
// fire the line.
func IsLookup(toolName string) bool { return shape.ToolVerb(toolName) == shape.VerbRead }

// Counted is the number of failed calls the line is about: Failed less the
// lookups. It is what fires the line and what the end-of-turn line prints.
func (sf SilentFailures) Counted() int { return sf.Failed - sf.FailedLookups }

// BuildSilentFailures compares the failure count against the final message.
//
// Exported so digest can call it with a turn-scoped run: the Failed and
// Unobserved counts below only ever read run.Executions, so a run holding
// only one turn's executions counts only that turn -- the transcript-reading
// half of Account is a separate concern, upstream of this function, and
// AccountFromMessage's doc explains why a digest builds one without opening
// anything.
func BuildSilentFailures(run *store.Run, acct Account) SilentFailures {
	sf := SilentFailures{
		AbsentWords:           []string{},
		FinalMessageAvailable: acct.Available,
	}
	for _, x := range run.Executions {
		if x.Backgrounded {
			// Recorded ok when Claude Code moved it to the background, before
			// it ended: how it ended is not known, as the timeline says.
			sf.Unobserved++
			continue
		}
		switch x.Outcome {
		case store.ExecFailed:
			sf.Failed++
			if IsLookup(x.ToolName) {
				sf.FailedLookups++
			}
		case "":
			// A v1 record, or an invocation that recorded no ending. Neither a
			// success nor a failure, and counting it as either would invent a
			// fact.
			sf.Unobserved++
		}
	}
	if sf.Counted() == 0 {
		// Nothing the line is about failed, so the message is not compared:
		// a failed lookup alone leaves no word to look for.
		return sf
	}

	// The WHOLE message, not the quote a reader sees: see Account.full.
	lower := strings.ToLower(acct.analysed())
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

// MaskedRuns is the build and test runs whose exit status their line masked
// -- `make test 2>&1 | tail -40`, `go test ./... ; echo $?`, `npm test ||
// true` -- set against whether the final message claims a pass.
//
// Like SilentFailures, a fact about TEXT and about the record, never about
// intent: it says the record cannot back a pass, not that there was none.
// The runner may well have passed; the line's ok is all that was recorded.
type MaskedRuns struct {
	// Fires is true when Runs is not 0, the final message was read, it claims
	// a pass (claimsPass), and it holds none of failureVocabulary: a
	// summary that names a failure has said what a masked status could have
	// kept from the reader, and one that claims no pass rests on nothing the
	// masked status could have changed.
	Fires bool `json:"fires"`
	// Runs counts calls whose shape says a build or test runner's exit
	// status was masked by its line (status_masked test or build), that
	// recorded ok and were not moved to the background, and that no later
	// call followed up: one whose line returns the same runner's failure
	// (status_masked none, the same runner_digest), started in the same
	// directory (cwd digest), and recorded ok or failed, not in the
	// background. A line that hid two runners, or ran its runner after a cd
	// its cwd digest does not hold, carries no runner digest, and nothing
	// follows it up. A masked
	// call that recorded failed is not here: it is a failed call, counted by
	// SilentFailures.
	Runs int `json:"runs"`
	// PassClaimed: the final message claims a pass (claimsPass).
	PassClaimed bool `json:"pass_claimed"`
	// FinalMessageAvailable distinguishes "claims no pass" from "there was
	// no summary to read".
	FinalMessageAvailable bool `json:"final_message_available"`
}

// passVocabulary is the fixed list of words that read as a claim that a
// build or test passed. Narrow on purpose, the other way round from
// failureVocabulary: the line fires only when one is present, so each word
// added makes it fire more. Matched as whole words (claimsPass), not as
// substrings: "password", "bypass" and "greenfield" claim nothing.
var passVocabulary = []string{
	"pass", "passes", "passed", "passing", "green",
	"succeed", "succeeds", "succeeded", "success", "successful", "successfully",
	"builds",
}

// negations are the words that make a pass word after them, to the end of
// its clause, no claim: "the tests do not pass", "not all tests pass yet",
// "none of the tests pass", "I'm not sure the tests pass". A word ending in
// n't is one too: "don't pass", "isn't green".
var negations = map[string]bool{
	"not": true, "no": true, "never": true, "cannot": true,
	"none": true, "nothing": true, "neither": true, "nor": true, "without": true,
}

// clauseBreaks are the characters that end a clause, and with it a
// negation: "No regressions, all tests pass." claims a pass.
const clauseBreaks = ".,;:!?()\n"

// claimsPass reports a message holding a word of passVocabulary, as a whole
// word, that no negation before it in its clause negates. "and" and "but"
// end a negation too: "No API changes and all tests pass." claims a pass.
// Clauses split at clauseBreaks; words are runs of letters and apostrophes,
// lower-cased, with a typographic apostrophe read as '. A conditional ("If
// the tests pass, merge it.") still counts as a claim. Failure words are
// matched as they always were, as substrings.
func claimsPass(msg string) bool {
	msg = strings.ReplaceAll(strings.ToLower(msg), "\u2019", "'")
	for _, clause := range strings.FieldsFunc(msg, func(r rune) bool { return strings.ContainsRune(clauseBreaks, r) }) {
		negated := false
		for _, w := range strings.FieldsFunc(clause, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
			switch w = strings.Trim(w, "'"); {
			case w == "and" || w == "but":
				negated = false
			case negations[w] || strings.HasSuffix(w, "n't"):
				negated = true
			case !negated && slices.Contains(passVocabulary, w):
				return true
			}
		}
	}
	return false
}

// sessionMaskedRuns is the report's masked runs for a session: nil when the
// session never measured masking (measuresMasking). The turn digest calls
// BuildMaskedRuns itself: it reads only the current turn, which this binary
// wrote.
func sessionMaskedRuns(run *store.Run, acct Account) *MaskedRuns {
	if !measuresMasking(run) {
		return nil
	}
	m := BuildMaskedRuns(run, acct)
	return &m
}

// BuildMaskedRuns counts run's masked runs that no later plain run of the
// same runner followed up (MaskedRuns.Runs), and compares the count against
// the final message. Exported so digest can call it with a turn-scoped run,
// as it calls BuildSilentFailures: a follow-up in a later turn is that turn's.
//
// The comparison reads seqs and the shapes' fields: the digest of the
// runner's words is compared and never printed, as the shape digest is.
func BuildMaskedRuns(run *store.Run, acct Account) MaskedRuns {
	out := MaskedRuns{FinalMessageAvailable: acct.Available}
	if run == nil {
		return out
	}
	executed := executionsByID(run)
	// ended reports a call that ran in the foreground and recorded result.
	ended := func(d store.Declaration, result string) bool {
		rec := outcomeRecord(executed[d.ToolUseID])
		return rec != nil && !rec.Backgrounded && rec.Outcome == result
	}
	type runner struct{ digest, cwd string }
	// last is the seq of the last call whose status was its runner's and
	// which recorded a result, per runner and directory.
	last := map[runner]int64{}
	for _, d := range run.Declarations {
		m, r := d.Shape.StatusMasked, d.Shape.RunnerDigest
		if m == nil || *m != shape.MaskedNone || r == nil {
			continue
		}
		if !ended(d, store.ExecOK) && !ended(d, store.ExecFailed) {
			continue
		}
		k := runner{*r, d.CWDDigest}
		if s, ok := last[k]; !ok || d.Seq > s {
			last[k] = d.Seq
		}
	}
	for _, d := range run.Declarations {
		if !statusMasked(d) || !ended(d, store.ExecOK) {
			continue
		}
		if r := d.Shape.RunnerDigest; r != nil {
			if s, ok := last[runner{*r, d.CWDDigest}]; ok && s > d.Seq {
				continue
			}
		}
		out.Runs++
	}
	if out.Runs == 0 || !acct.Available {
		return out
	}
	lower := strings.ToLower(acct.analysed())
	out.PassClaimed = claimsPass(acct.analysed())
	failure := false
	for _, w := range failureVocabulary {
		if strings.Contains(lower, w) {
			failure = true
		}
	}
	out.Fires = out.PassClaimed && !failure
	return out
}

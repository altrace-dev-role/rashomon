package store

// The outcome_join kind is in the 1.2.0 table (record_v5.go's OutcomeJoin:
// a call's outcome learned after its execution record), and its first writer
// is a background command's end (card BG-OUTCOME phase 1). A Bash call
// launched with run_in_background is filed Backgrounded at its post; its end
// arrives later as a task notification, which Claude Code hands to
// UserPromptSubmit as the prompt, and the catch-up keeps the end's closed
// fields here, joined to the call by its id. The notification's description
// is the agent's text and its output file is command output: neither is
// kept, nor is the task's own id.

// SourceTaskNotification is the outcome_source of an end read from a task
// notification; ParserTaskNotification the version of that read.
const (
	SourceTaskNotification = "task_notification"
	ParserTaskNotification = 1
)

// NewOutcomeJoin is an outcome_join for session's call toolUseID, outcome
// ExecOK or ExecFailed, stamped at its kind's version with critical true.
func NewOutcomeJoin(sessionID, toolUseID, outcome string, exitCode int, source string, parser int, recordedAtMS int64) *OutcomeJoin {
	return &OutcomeJoin{
		EventHeader: EventHeader{Type: TypeOutcomeJoin, SchemaVersion: EventSchemaVersion, Critical: true,
			RecordedAtMS: recordedAtMS, SessionID: sessionID},
		ToolUseID: CapID(toolUseID), Outcome: outcome, ExitCode: &exitCode, OutcomeSource: source, ParserVersion: parser,
	}
}

// Kind is the pair the record carries, which AppendRecord checks.
func (e *OutcomeJoin) Kind() (string, int) { return e.Type, e.SchemaVersion }

// SetSeq is the seq the store allocated the record under the run's lock.
func (e *OutcomeJoin) SetSeq(seq *int64) { e.Seq = seq }

// readOutcomeJoin files an outcome_join line in run.
func readOutcomeJoin(run *Run, line []byte, kind string, version int) error {
	var rec OutcomeJoin
	if err := decodeAs(line, &rec, kind, version); err != nil {
		return err
	}
	run.OutcomeJoins = append(run.OutcomeJoins, rec)
	return nil
}

package store

// TypeConflictCheck is the conflict-check record (W-M2.4 hook seam, card
// m2-seam1 D4): the post's reading of a landed edit against the session's
// conflict base, the full set of conflicts the edit leaves in its file,
// possibly empty, kept beside unitbrief_miss, the third non-critical kind.
// It is written for a Go edit the overlay writer kept (a session keeping its
// content, the call attributed, an index over the file) when the session
// holds the prompt's conflict base at the index's commit and the file is in
// a rule's scope; an out-of-scope edit can raise nothing and writes none.
// It is independent of delivery: a note the cap or the once-a-session key
// withholds still counts as seen, and the Stop folds these records into
// its conflicts (conflictsSeen): a conflict is resolved when no file still
// shows it at its commit. Nothing an older reader concludes depends on it,
// so it is written critical false and a reader without the type counts the
// line Ignored (passOver).
const TypeConflictCheck = "conflict_check"

// ConflictCheckRow is one conflict a check found. Kind and RuleID are in
// the clear (an engine kind; a rule's id as its spec or knowledge row
// declares it). Key is the conflict's identity (the engine's Conflict.Key)
// as a keyed digest at every record scope. At record scope v5 Unit and
// Subject are global ids and UnitPath the unit's path in the repository, as
// the Stop's line prints it; at v4 Unit and Subject are keyed digests and
// UnitPath is empty, so no path is held.
type ConflictCheckRow struct {
	Kind     string `json:"kind"`
	RuleID   string `json:"rule_id"`
	Unit     string `json:"unit"`
	UnitPath string `json:"unit_path"`
	Subject  string `json:"subject"`
	Key      string `json:"key"`
}

// ConflictCheck holds no word of the prompt. LandedSeq is the seq of the
// edit's execution record; Commit the index commit the base was read at; File
// the edited file's id as a keyed digest at every record scope.
type ConflictCheck struct {
	EventHeader
	LandedSeq int64              `json:"landed_seq"`
	Commit    string             `json:"commit"`
	File      string             `json:"file"`
	Rows      []ConflictCheckRow `json:"rows"`
}

// NewConflictCheck is a conflict_check for session's edit whose execution
// landed at landedSeq, stamped at its kind's version with critical false. A
// nil rows is written as an empty list: the check found nothing.
func NewConflictCheck(sessionID string, landedSeq int64, commit, file string, rows []ConflictCheckRow, recordedAtMS int64) *ConflictCheck {
	if rows == nil {
		rows = []ConflictCheckRow{}
	}
	return &ConflictCheck{
		EventHeader: EventHeader{Type: TypeConflictCheck, SchemaVersion: EventSchemaVersion, Critical: false,
			RecordedAtMS: recordedAtMS, SessionID: sessionID},
		LandedSeq: landedSeq, Commit: commit, File: file, Rows: rows,
	}
}

// Kind is the pair the record carries, which AppendRecord checks.
func (e *ConflictCheck) Kind() (string, int) { return e.Type, e.SchemaVersion }

// SetSeq is the seq the store allocated the record under the run's lock.
func (e *ConflictCheck) SetSeq(seq *int64) { e.Seq = seq }

// readConflictCheck files a conflict_check line in run.
func readConflictCheck(run *Run, line []byte, kind string, version int) error {
	var rec ConflictCheck
	if err := decodeAs(line, &rec, kind, version); err != nil {
		return err
	}
	run.ConflictChecks = append(run.ConflictChecks, rec)
	return nil
}

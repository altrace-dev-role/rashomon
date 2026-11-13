package store

// TypeUnitBriefMiss is the unit-brief miss record (W-M2.3 hook half, card
// m2-unitbrief-hook1 D7): the post's fact that a landed edit's read of its
// unit's brief found none it could say, kept beside plan_check, the
// second non-critical kind. The edit is a Go edit the overlay writer would
// keep (a session keeping its content, the call attributed, an index over
// the file), its unit not yet given the brief; any other edit writes none,
// so a given unit, a store without the session-content marker, a path in
// no Go unit and an unattributed session are not misses. Like rule_hit and
// unlike plan_check it does not wait on the steer context: whether a brief
// could be said is a fact about the file, whoever the note could reach.
// Nothing a reader concludes about the session's rules depends on it, so
// it is written critical false and a reader without the type counts the
// line Ignored (passOver).
const TypeUnitBriefMiss = "unitbrief_miss"

// The reasons a unit's brief was not said.
const (
	// UnitBriefAbsent: the session holds no brief file for the unit.
	UnitBriefAbsent = "absent"
	// UnitBriefStale: the file's brief is at a commit other than the
	// index's.
	UnitBriefStale = "stale"
	// UnitBriefRefused: the file did not read back as a brief of the unit
	// (unreadable, over the cap, torn, another shape, another unit), or did
	// not render.
	UnitBriefRefused = "refused"
)

// UnitBriefMissReasons is every reason a unitbrief_miss carries.
func UnitBriefMissReasons() []string {
	return []string{UnitBriefAbsent, UnitBriefStale, UnitBriefRefused}
}

// UnitBriefMiss holds no path, no unit's name and no word of a brief.
// LandedSeq is the seq of the edit's execution record. Unit is the unit's
// global id as a keyed digest (intent.PathDigest), at every record scope.
type UnitBriefMiss struct {
	EventHeader
	LandedSeq int64  `json:"landed_seq"`
	Unit      string `json:"unit"`
	Reason    string `json:"reason"`
}

// NewUnitBriefMiss is a unitbrief_miss for session's edit whose execution
// landed at landedSeq, stamped at its kind's version with critical false.
func NewUnitBriefMiss(sessionID string, landedSeq int64, unit, reason string, recordedAtMS int64) *UnitBriefMiss {
	return &UnitBriefMiss{
		EventHeader: EventHeader{Type: TypeUnitBriefMiss, SchemaVersion: EventSchemaVersion, Critical: false,
			RecordedAtMS: recordedAtMS, SessionID: sessionID},
		LandedSeq: landedSeq, Unit: unit, Reason: reason,
	}
}

// Kind is the pair the record carries, which AppendRecord checks.
func (e *UnitBriefMiss) Kind() (string, int) { return e.Type, e.SchemaVersion }

// SetSeq is the seq the store allocated the record under the run's lock.
func (e *UnitBriefMiss) SetSeq(seq *int64) { e.Seq = seq }

// readUnitBriefMiss files a unitbrief_miss line in run.
func readUnitBriefMiss(run *Run, line []byte, kind string, version int) error {
	var rec UnitBriefMiss
	if err := decodeAs(line, &rec, kind, version); err != nil {
		return err
	}
	run.UnitBriefMisses = append(run.UnitBriefMisses, rec)
	return nil
}

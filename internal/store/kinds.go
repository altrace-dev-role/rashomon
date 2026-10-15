package store

// EventSchemaVersion is the version every record kind introduced at 1.2.0 is
// written at: session, prompt, turn_end, turn_cost, human, subagent, batch,
// task, compact, instructions and model (RECORD), the verify checkpoint
// (VERIFY), outcome_join and write_observed (BREADTH). Technical spec 1.2.2;
// Archive 10.2.1.
//
// Contract only (P-07): the kinds, the per-kind versions and the acceptance
// rule are frozen here; the writers that stamp them land with their PRs, and
// the call-record writer moves from SchemaVersion to KindVersion in P-11.
const EventSchemaVersion = 5

// CallSchemaVersion is the version the call records (declaration, execution,
// terminal) take at 1.2.0, when their v5 fields land (record_v5.go). Until
// P-11 the writer stamps SchemaVersion, 4, because a record at version 5 with
// none of the v5 keys filled would be a v5 record that lied about being one.
const CallSchemaVersion = 5

// The record kinds introduced at 1.2.0. There is no steer kind (ED-1):
// STEER's lines live in their own side file, runs/<dir>/steer.ndjson.
const (
	TypeSession       = "session"
	TypePrompt        = "prompt"
	TypeTurnEnd       = "turn_end"
	TypeTurnCost      = "turn_cost"
	TypeHuman         = "human"
	TypeSubagent      = "subagent"
	TypeBatch         = "batch"
	TypeTask          = "task"
	TypeCompact       = "compact"
	TypeInstructions  = "instructions"
	TypeModel         = "model"
	TypeVerify        = "verify"
	TypeOutcomeJoin   = "outcome_join"
	TypeWriteObserved = "write_observed"
)

// kindVersion is the version each writer stamps on its kind once 1.2.0's
// writers are in (REC-1): call kinds at 5, coverage and gaps unchanged at 3,
// every new kind at 5. It is unexported so that only KindVersion can read
// it: an importer that could write into an exported map could change what
// AcceptsKind accepts.
var kindVersion = map[string]int{
	TypeDeclaration:   CallSchemaVersion,
	TypeExecution:     CallSchemaVersion,
	TypeTerminal:      CallSchemaVersion,
	TypeCoverage:      SessionSchemaVersion,
	TypeGap:           SessionSchemaVersion,
	TypeSession:       EventSchemaVersion,
	TypePrompt:        EventSchemaVersion,
	TypeTurnEnd:       EventSchemaVersion,
	TypeTurnCost:      EventSchemaVersion,
	TypeHuman:         EventSchemaVersion,
	TypeSubagent:      EventSchemaVersion,
	TypeBatch:         EventSchemaVersion,
	TypeTask:          EventSchemaVersion,
	TypeCompact:       EventSchemaVersion,
	TypeInstructions:  EventSchemaVersion,
	TypeModel:         EventSchemaVersion,
	TypeVerify:        EventSchemaVersion,
	TypeOutcomeJoin:   EventSchemaVersion,
	TypeWriteObserved: EventSchemaVersion,
}

// KindVersion returns the version a writer stamps on kind, and whether kind
// is in the table at all.
func KindVersion(kind string) (int, bool) {
	v, ok := kindVersion[kind]
	return v, ok
}

// AcceptsKind reports whether a 1.2.0 reader parses a record of this kind at
// this version. It replaces Accepts, which knew only the call records'
// versions, once the reader is switched to it in P-11.
//
// Call kinds are accepted from 1 to 5, so a store that recorded for weeks
// before the upgrade still reads whole (the data-loss bug Accepts' comment
// records). Coverage and gaps are accepted from 1 to 3, their own history.
// A new kind is accepted at 5 only: it has no older version to read. An
// unknown kind is never accepted here; whether it is ignored or unverifies
// the session is the reader's decision from the record's `critical` key
// (1.2.2), not this function's.
func AcceptsKind(kind string, version int) bool {
	switch kind {
	case TypeDeclaration, TypeExecution, TypeTerminal:
		return version >= 1 && version <= CallSchemaVersion
	case TypeCoverage, TypeGap:
		return version >= 1 && version <= SessionSchemaVersion
	}
	v, known := kindVersion[kind]
	return known && version == v
}

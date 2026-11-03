package store

// EventSchemaVersion is the version every record kind introduced at 1.2.0 is
// written at: session, prompt, turn_end, turn_cost, human, subagent, batch,
// task, compact, instructions and model (RECORD), the verify checkpoint
// (VERIFY), outcome_join and write_observed (BREADTH). Technical spec 1.2.2;
// Archive 10.2.1. The kinds, the per-kind versions and the acceptance rule
// are frozen here; each kind's writer lands with its own change.
const EventSchemaVersion = 5

// CallSchemaVersion is the version the call records (declaration, execution,
// terminal) are written at: the v5 keys (record_v5.go), at 6 runner_kind on
// the declaration's shape and the execution, at 7 store_named on the
// declaration (D173b), and at 8 runner_name and masked_by on the execution
// (D229), and at 9 location_unread on the declaration (5.14b). A new key gets a version of
// its own and never joins an existing one, whose records are already on disk
// without it (D11). SchemaVersion is the same number under the name the
// writers use.
const CallSchemaVersion = 9

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

// TypeSpecEvent is the first kind added after the 1.2.0 freeze (card
// SPEC-RULES-STOP1, D-b): a fact about the spec's rules only the hook saw,
// one kind with a closed reason set (specevent.go, D-e), at
// EventSchemaVersion and, unlike the 1.2.0 kinds, critical.
const TypeSpecEvent = "spec_event"

// kindVersion is the version each writer stamps on its kind (REC-1): call
// kinds at 9, coverage and gaps unchanged at 3, every new kind at 5, the
// spec_event added after the freeze included. It is
// unexported so that only KindVersion can read it: an importer that could
// write into an exported map could change what AcceptsKind accepts.
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
	TypeSpecEvent:     EventSchemaVersion,
}

// KindVersion returns the version a writer stamps on kind, and whether kind
// is in the table at all.
func KindVersion(kind string) (int, bool) {
	v, ok := kindVersion[kind]
	return v, ok
}

// AcceptsKind reports whether this reader parses a record of this kind at
// this version. It replaced Accepts, which knew only the call records'
// versions and so could not tell a coverage record at 4 from a declaration
// at 4.
//
// It is a rule over (kind, version) pairs rather than an equality against
// the version a writer stamps, because the two are different questions and
// conflating them is a silent data-loss bug: the reader once compared
// `!= SchemaVersion`, so the moment the writer moved to v2 every v1 record
// on disk would have been skipped, and a store that had recorded for weeks
// would have rendered an empty report with nothing saying why.
//
// Call kinds are accepted from 1 to CallSchemaVersion, so a store that
// recorded before the upgrade still reads whole. Coverage and gaps are
// accepted from 1 to 3, their own history. A new kind is accepted at 5 only:
// it has no older version to read. An unknown kind is never accepted here;
// whether it is ignored or unverifies the session is the reader's decision
// from the record's `critical` key (1.2.2, classifyRunLine), not this
// function's.
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

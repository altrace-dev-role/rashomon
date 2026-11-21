package verify

// The predicates a requirement row is checked by (W-M3.4): a file exists, a
// symbol exists, a gate ran green, a test of that name exists.
const (
	PredFileExists   = "file_exists"
	PredSymbolExists = "symbol_exists"
	PredGateGreen    = "gate_green"
	PredTestNamed    = "test_named"
)

// Preds is the closed list of requirement predicates, in the order above.
var Preds = []string{PredFileExists, PredSymbolExists, PredGateGreen, PredTestNamed}

// SourceRequirement is the source of a requirement's row. The row is an
// expected_evidence row, as a task's is, told apart by its source (a task's is
// SourceTask) and keyed by ReqFamily.
const SourceRequirement = "requirement"

// ReqRow is one requirement the Stop's caller checked (W-M3.4): its id, its
// predicate, one of Preds, and the predicate's argument. Known false is a
// check that could not be made, a row unknown and never met (INV-2); Holds is
// read only where Known.
type ReqRow struct {
	ID, Pred, Arg string
	Holds, Known  bool
}

// ReqFamily is the family a requirement's row is keyed by: "req:" and the
// requirement's id. The row's id is ObligationID(scope, ExpectedEvidence,
// ReqFamily(id)); no task evidence kind starts "req:", so a requirement never
// takes a task row's id.
func ReqFamily(id string) string { return "req:" + id }

package verify

import "testing"

// The ledger's turn rows (W-M3.1): each test below is the card's, and runs
// once the rows are built.

// TestLedger_TurnRowIDsDifferAcrossTurns: two turns' rows for one gate never
// share an id.
func TestLedger_TurnRowIDsDifferAcrossTurns(t *testing.T) {
	t.Skip("W-M3.1 body: the ledger has no turn rows yet")
}

// TestLedger_TwoStopsInOneTurnShareIDs: two Stops in one turn give its rows
// the same ids.
func TestLedger_TwoStopsInOneTurnShareIDs(t *testing.T) {
	t.Skip("W-M3.1 body: the ledger has no turn rows yet")
}

// TestLedger_UnitRowPerUnitWritten: one turn row per unit the turn wrote,
// its Unit UnitOf's id.
func TestLedger_UnitRowPerUnitWritten(t *testing.T) {
	t.Skip("W-M3.1 body: the ledger has no turn rows yet")
}

// TestLedger_TurnWindowOwesOnlyItsWrites: a write before the turn's From owes
// no turn row.
func TestLedger_TurnWindowOwesOnlyItsWrites(t *testing.T) {
	t.Skip("W-M3.1 body: the ledger has no turn rows yet")
}

// TestLedger_UnitRowCapBindsAndCounts: with MaxTurnUnitRows units written the
// Stop keeps every unit row and UnitRowsCut is 0; with one unit more it keeps
// MaxTurnUnitRows, in gate order then unit id ascending, and UnitRowsCut is 1.
func TestLedger_UnitRowCapBindsAndCounts(t *testing.T) {
	t.Skip("W-M3.1 body: the ledger has no turn rows yet")
}

// TestLedger_SubtaskPointAddsNoTurnRows: a subtask or commit point's ledger
// is its session rows only.
func TestLedger_SubtaskPointAddsNoTurnRows(t *testing.T) {
	t.Skip("W-M3.1 body: the ledger has no turn rows yet")
}

// TestLedger_NilUnitOfAddsNoUnitRow: with UnitOf nil no turn row carries a
// Unit; with it set, the unit rows come (witness).
func TestLedger_NilUnitOfAddsNoUnitRow(t *testing.T) {
	t.Skip("W-M3.1 body: the ledger has no turn rows yet")
}

// TestLedger_Replay: each Stop's ledger, turn rows included, is the ledger
// over the record's prefix at its seq (INV-11).
func TestLedger_Replay(t *testing.T) {
	t.Skip("W-M3.1 body: the ledger has no turn rows yet")
}

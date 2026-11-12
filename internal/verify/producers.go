package verify

// ProducedRow is one row a producer outside this package derived from the
// graph (W-M3.3), which the Stop's caller converts and the ledger places
// without reading a graph: the consumers of a changed unit to verify as
// platform_impact, the tests covering it as proposed_check. Unit is the
// unit's global id, in UnitOf's form; Subject is what the row is about and
// Source its producer; Seqs are the record seqs it cites, ascending. Known
// false is a row its producer could not decide, a row unknown and never met
// (INV-2); Met, whether the session did what the row asks, is read only
// where Known, as ReqRow's Holds is.
type ProducedRow struct {
	Kind                  ObligationKind
	Unit, Source, Subject string
	Seqs                  []int64
	Met, Known            bool
}

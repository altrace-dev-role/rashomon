package hook

// ConsentHas is consent.Has, which cmd/rashomon sets before any hook runs,
// handed in rather than imported for ConsentRecordScope's reason (the import
// would be a cycle). Nil, in a test that builds a probe without it, grants
// nothing: unlike the record scope's sync, which a nil skips, this one gates
// a copy, and a gate that cannot ask withholds.
var ConsentHas func(root, unit string) (bool, error)

// bodiesConsented reports whether root's consent grants the bodies unit
// (card 4.2), the user's answer to `rashomon consent bodies`. An error reads
// as no: SessionEnd copies nothing it cannot show was agreed to.
func bodiesConsented(root string) bool {
	if ConsentHas == nil {
		return false
	}
	ok, err := ConsentHas(root, "bodies")
	return err == nil && ok
}

package verify

// For the turn replay (instrument_replay_test.go, package verify_test): the
// Stop's turn rule as the Stop calls it, and the parts that name the call
// which made a turn count.
var (
	TurnActed    = turnActed
	WritesOrRuns = writesOrRuns
	ProjectOf    = projectOf
)

// InNoProject reports a project the hook proved is no project (projectOf).
func InNoProject(p *string) bool { return p != nil && *p == noProject }

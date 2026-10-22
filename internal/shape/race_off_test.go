//go:build !race

package shape

// raceEnabled says whether this binary is instrumented by the race
// detector, which inflates CPU time (and so wall time) well past what the
// same work costs plain -- a cost this package's allocation-gated tests
// never see, because TotalAlloc is unchanged either way. A wall-clock bound
// can only be held to an uninstrumented build; under -race it is logged,
// not enforced.
const raceEnabled = false

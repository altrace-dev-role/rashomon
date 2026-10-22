//go:build race

package jsonscan

// raceEnabled reports a -race build, whose sync.Pool drops a Put value at
// random, so a pooled object can be reallocated on any call.
const raceEnabled = true

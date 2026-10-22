//go:build !race && !windows

// raceEnabled's only reader, res7_test.go, is itself !windows (peak RSS is
// read from the child's Unix rusage), so a windows build never needs this
// constant and would leave it unused.
package main

// raceEnabled: see race_on_test.go.
const raceEnabled = false

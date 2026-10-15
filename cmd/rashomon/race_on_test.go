//go:build race && !windows

// raceEnabled's only reader, res7_test.go, is itself !windows (peak RSS is
// read from the child's Unix rusage), so a windows build never needs this
// constant and would leave it unused.
package main

// raceEnabled: the race detector's shadow memory raises every child's floor,
// so absolute peak-RSS bounds hold only in an uninstrumented build.
const raceEnabled = true

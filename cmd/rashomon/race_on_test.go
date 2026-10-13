//go:build race

package main

// raceEnabled: the race detector's shadow memory raises every child's floor,
// so absolute peak-RSS bounds hold only in an uninstrumented build.
const raceEnabled = true

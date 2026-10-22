//go:build !unix

package peakrss

import (
	"os"
	"os/exec"
)

// NewReaper returns ErrUnavailable: this platform's process state carries no
// ru_maxrss.
func NewReaper(string) (*Reaper, error) { return nil, ErrUnavailable }

// Run returns ErrUnavailable and does not start cmd.
func (*Reaper) Run(*exec.Cmd) (float64, error) { return 0, ErrUnavailable }

// Reap reports that it cannot measure and returns the reaper's own failure
// status.
func Reap(_ []string, _ *os.File) int { return 126 }

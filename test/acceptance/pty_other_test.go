//go:build !darwin && !linux

package acceptance

import (
	"os/exec"
	"testing"
)

// The pseudo-terminal tests answer on /dev/tty, which windows does not
// have: rashomon there refuses to ask and needs --yes, which the
// refusal tests check without a terminal.

type ptySession struct {
	cmd *exec.Cmd
	out *ptyOutput
}

type ptyOutput struct{}

func (*ptyOutput) String() string { return "" }

func startOnPTY(t *testing.T, cmd *exec.Cmd) *ptySession {
	t.Skip("no /dev/tty on this platform: the questions are asked only on a Unix terminal")
	return nil
}

func (s *ptySession) waitFor(string)     {}
func (s *ptySession) countOf(string) int { return 0 }
func (s *ptySession) answer(string)      {}
func (s *ptySession) eof()               {}
func (s *ptySession) wait() result       { return result{} }

// noTerminal is a no-op: a windows process has no /dev/tty to open.
func noTerminal(*exec.Cmd) {}

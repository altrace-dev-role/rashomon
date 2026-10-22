//go:build darwin || linux

package acceptance

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// A pseudo-terminal for the tests that answer rashomon's questions the way a
// person does: on the controlling terminal, never on stdin. It is the
// smallest one that works with only the standard library, on the two
// platforms the suite runs a terminal on.

func ioctl(fd, req, arg uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg); errno != 0 {
		return errno
	}
	return nil
}

// ptyOutput collects everything the child writes to the terminal.
type ptyOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *ptyOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	// The terminal turns each line feed into CR LF on output.
	return strings.ReplaceAll(o.buf.String(), "\r\n", "\n")
}

// ptySession is one child process whose controlling terminal is a fresh
// pseudo-terminal.
type ptySession struct {
	t      *testing.T
	cmd    *exec.Cmd
	master *os.File
	out    *ptyOutput
	read   chan struct{}
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

// startOnPTY starts cmd in a new session whose controlling terminal is a new
// pseudo-terminal. The terminal is not the child's stdin, stdout or stderr:
// stdin is left as cmd sets it, and stdout and stderr are captured apart, so
// an answer that arrives is one read from /dev/tty.
func startOnPTY(t *testing.T, cmd *exec.Cmd) *ptySession {
	t.Helper()
	// Opened by syscall and wrapped, not by os.OpenFile: os.OpenFile puts
	// the descriptor under the runtime's poller, and on darwin kqueue never
	// reports a pty master readable, so the child's output would sit in the
	// terminal unread and its exit would block on draining it.
	fd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("no pseudo-terminal: %v", err)
	}
	master := os.NewFile(uintptr(fd), "/dev/ptmx")
	name, err := ptsName(master)
	if err != nil {
		master.Close()
		t.Fatalf("pseudo-terminal slave: %v", err)
	}
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Fatalf("opening %s: %v", name, err)
	}
	s := &ptySession{t: t, cmd: cmd, master: master, out: &ptyOutput{}, read: make(chan struct{}),
		stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	cmd.Stdout, cmd.Stderr = s.stdout, s.stderr
	// The slave is the child's fd 3, made its controlling terminal in a
	// session of its own; the parent's copy is closed once it has started.
	cmd.ExtraFiles = []*os.File{slave}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 3}
	if err := cmd.Start(); err != nil {
		slave.Close()
		master.Close()
		t.Fatalf("starting %v: %v", cmd.Args, err)
	}
	slave.Close()
	go func() {
		defer close(s.read)
		b := make([]byte, 4096)
		for {
			n, err := master.Read(b)
			if n > 0 {
				s.out.mu.Lock()
				s.out.buf.Write(b[:n])
				s.out.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		if s.cmd.ProcessState == nil {
			s.cmd.Process.Kill() //nolint:errcheck // the test has already failed
			s.cmd.Wait()         //nolint:errcheck // reaping only
		}
		master.Close()
	})
	return s
}

// waitFor waits until the terminal has shown want, and fails the test with
// what it did show if it does not within the bound.
func (s *ptySession) waitFor(want string) {
	s.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.out.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("the terminal never showed %q; it showed:\n%s\nstderr: %s", want, s.out.String(), s.stderr.String())
}

// countOf is how many times the terminal has shown want so far.
func (s *ptySession) countOf(want string) int { return strings.Count(s.out.String(), want) }

// answer types a line at the terminal.
func (s *ptySession) answer(line string) {
	s.t.Helper()
	if _, err := s.master.Write([]byte(line + "\n")); err != nil {
		s.t.Fatalf("typing %q: %v", line, err)
	}
}

// typeThenEOF types text and then Ctrl-D twice, never a line feed: the
// first hands text to the reader, the second is end of file.
func (s *ptySession) typeThenEOF(text string) {
	s.t.Helper()
	if _, err := s.master.Write(append([]byte(text), 4, 4)); err != nil {
		s.t.Fatalf("typing %q then Ctrl-D: %v", text, err)
	}
}

// eof types Ctrl-D at the start of a line: end of file to the reader.
func (s *ptySession) eof() {
	s.t.Helper()
	if _, err := s.master.Write([]byte{4}); err != nil {
		s.t.Fatalf("typing Ctrl-D: %v", err)
	}
}

// wait waits for the child to exit and returns its exit code.
func (s *ptySession) wait() result {
	s.t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		s.cmd.Process.Kill() //nolint:errcheck // the test fails below
		s.t.Fatalf("the child did not exit; the terminal showed:\n%s", s.out.String())
	}
	return result{exitCode: s.cmd.ProcessState.ExitCode(), stdout: s.stdout.String(), stderr: s.stderr.String()}
}

// noTerminal makes cmd start in a session of its own with no controlling
// terminal, the way a hook or a CI job runs it, so opening /dev/tty fails
// even when the suite itself runs in a terminal.
func noTerminal(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

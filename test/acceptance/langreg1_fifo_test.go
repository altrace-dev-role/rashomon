//go:build unix

package acceptance

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph/index"
)

// lrBound is how long a LANG-REG1 hook or post may run before the test
// kills it: a configuration that is a FIFO must be refused, not opened, and
// an open of one with no writer would hold the child until the test
// binary's -timeout.
const lrBound = 10 * time.Second

// lrWithin runs sub (hook or post) on payload as e.hook and e.post do, and
// kills it at lrBound; ok is false for a child killed there.
func (e *env) lrWithin(sub, payload string) (res result, ok bool) {
	e.t.Helper()
	cmd := e.command(payload, lrEnv, append([]string{sub}, e.installArgs()...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		e.t.Fatalf("starting %v: %v", cmd.Args, err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	timer := time.NewTimer(lrBound)
	defer timer.Stop()
	select {
	case <-done:
		return result{exitCode: cmd.ProcessState.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}, true
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		return result{exitCode: -1, stdout: stdout.String(), stderr: stderr.String()}, false
	}
}

// lrSaidWithin is lrSaid with each child bounded by lrBound: what the hook
// and the post printed, and a failure for each that was killed or failed.
func (e *env) lrSaidWithin(dir, id, file, old string) string {
	e.t.Helper()
	pre, post := e.lrCall(dir, id, file, old)
	var said strings.Builder
	for _, c := range []struct{ sub, payload string }{{"hook", pre}, {"post", post}} {
		res, ok := e.lrWithin(c.sub, c.payload)
		switch {
		case !ok:
			e.t.Errorf("%s %s of %s ran past %s and was killed", c.sub, id, file, lrBound)
		case res.exitCode != 0:
			e.t.Errorf("%s %s: exit %d, stdout %q, stderr %q", c.sub, id, res.exitCode, res.stdout, res.stderr)
		}
		said.WriteString(res.stdout)
	}
	return said.String()
}

// lrFIFO replaces dir's file with a FIFO that no process writes.
func lrFIFO(t *testing.T, dir, file string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(file))
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLANGREG1_AFIFOConfigSaysWhy (s6, FIFO): a configuration that turns
// into a FIFO after the index is refused, not opened, and an edit says it
// was not read and why, once in the session for the file: the py unit's
// .rashomon/graph.json under two .py edits, and go.mod under a .go edit.
// Every hook and post is bounded by lrBound. Base: no line, and the Go
// edit's hook opens the go.mod FIFO and is killed at the bound. Break: open
// the configuration by os.Open, and the child is killed.
func TestLANGREG1_AFIFOConfigSaysWhy(t *testing.T) {
	for _, c := range []struct {
		name, file, ext string
		edits           [][3]string
	}{
		{"graph.json", index.ConfigFile, ".py", [][3]string{{"toolu_f1", "inv/core/calc.py", "sum"}, {"toolu_f2", "inv/app/main.py", "run"}}},
		{"go.mod", "go.mod", ".go", [][3]string{{"toolu_f3", "svc/svc.go", "package"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, _ := e.lrProject()
			lrFIFO(t, dir, c.file)
			var said strings.Builder
			for _, ed := range c.edits {
				said.WriteString(e.lrSaidWithin(dir, ed[0], ed[1], ed[2]))
			}
			lrReason(t, said.String(), lrUnread(c.file, c.ext))
		})
	}
}

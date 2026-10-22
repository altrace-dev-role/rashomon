package acceptance

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// H-104 -- digest never sniffs stdin to decide whether to read it.
//
// A live terminal is not the only stdin that never sends EOF: an inherited
// pipe that stays open does the same thing, and it is the ordinary case for
// a command invoked from a script or a parent process rather than typed at
// a shell prompt -- which is exactly how this was found, running a timing
// harness that inherited stdin without redirecting it. The hook path itself
// is safe, because Claude Code writes the payload and closes the pipe, but a
// general-purpose command must not wedge a caller that has no payload to
// send and no reason to expect it would be asked for one.
//
// Break: read stdin whenever it happens to be something other than a
// terminal (the sniffing this item replaces) and this hangs, because a pipe
// is not a character device either.
func TestH104_DigestDoesNotHangOnAnOpenStdinPipe(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close() //nolint:errcheck // closed at test end either way; the pipe outliving digest is the point

	cmd := exec.Command(rashomonBin, "digest", "--session", testSession, "--prompt", p.PromptID)
	cmd.Stdin = r
	cmd.Env = e.environ()
	cmd.Dir = e.cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting digest: %v", err)
	}
	r.Close() //nolint:errcheck // the child has its own copy of the read end; only the write end must outlive it

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("digest exited %v, stderr %q", err, stderr.String())
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("digest did not return within 2s with stdin an open pipe that never sends EOF -- " +
			"it is reading stdin without being asked to (--stdin was not given)")
	}

	if !strings.Contains(stdout.String(), `"schema_version"`) {
		t.Errorf("digest did not produce its usual JSON output:\n%s", stdout.String())
	}
}

// TestH104_StdinFlagReadsTheFinalMessage is the positive case: given
// --stdin, the SAME payload shape a Stop hook would send is read and used,
// and the caller is the one promising to close the pipe -- which a real
// write-then-close, as here, does.
func TestH104_StdinFlagReadsTheFinalMessage(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(failurePayload(t, p.ToolUseID, "Exit code 1", false, 5))

	res := e.run(`{"last_assistant_message":"The command failed as expected."}`, nil,
		"digest", "--session", testSession, "--prompt", p.PromptID, "--stdin")
	if res.exitCode != 0 {
		t.Fatalf("digest --stdin: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, `"final_message_available":true`) {
		t.Errorf("digest --stdin did not pick up the piped final message:\n%s", res.stdout)
	}
	if strings.Contains(res.stdout, `"fires":true`) {
		t.Errorf("silent_failures fired although the piped message acknowledges the failure:\n%s", res.stdout)
	}
}

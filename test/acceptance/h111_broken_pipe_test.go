package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// H-111 -- a hook whose reader has gone away still exits 0.
//
// Claude Code reads a hook's stdout and stderr through pipes, and it can stop
// reading before the hook writes: the turn is interrupted, the entry's timeout
// fires, Claude Code itself exits. The write then lands on a pipe with no read
// end. Go turns a failed write to fd 1 or fd 2 into death by SIGPIPE unless
// the program handles or ignores that signal (os/signal's package doc), so
// the process ends with status 141 and no deferred function or recover runs:
// safe.Guard cannot catch a signal. That is a non-zero exit from a hook path,
// which is the one outcome every entry point here is written to rule out.
//
// Each case runs twice from the same arrangement. With a reader present it
// must write, which proves the broken-pipe run had something to write and the
// write was attempted; an entry that wrote nothing would pass the second half
// trivially. With the reader gone it must exit 0, not die by a signal, and put
// nothing on the stream that is still open in place of what it could not say.
//
// Break: drop the SIGPIPE disposition from an entry point, or restore the
// default one, and that entry dies by SIGPIPE on its first write.
func TestH111_AnEntryWhoseReaderIsGoneStillExitsZero(t *testing.T) {
	cases := []struct {
		name string
		// gone is the stream whose reader has gone away; the other one is
		// captured, and is where anything said instead would show.
		gone string
		// arrange builds the store the entry runs against and returns its
		// stdin and its arguments.
		arrange func(e *env) (stdin string, args []string)
	}{
		{
			// A failed call makes recap speak, and its one line is the only
			// thing any entry writes to stdout.
			name: "recap",
			gone: "stdout",
			arrange: func(e *env) (string, []string) {
				e.watched(testSession)
				e.mustHook(defaultPayload().build(e.t))
				e.mustPost(failurePayload(e.t, testToolUseID, "Exit code 1", false, 30))
				return stopPayload(testSession, "Ran the command as requested.", false),
					append([]string{"recap"}, e.installArgs()...)
			},
		},
		{
			// An entry from another install says so in one line on stderr,
			// which is the only thing the recording entries ever write.
			name: "hook",
			gone: "stderr",
			arrange: func(e *env) (string, []string) {
				e.watched(testSession)
				return defaultPayload().build(e.t), []string{"hook", "--install", otherInstall}
			},
		},
		{
			name: "post",
			gone: "stderr",
			arrange: func(e *env) (string, []string) {
				e.watched(testSession)
				return defaultPost().build(e.t), []string{"post", "--install", otherInstall}
			},
		},
		{
			// probe writes its complaint about a phase it does not know before
			// it looks at a store, so this needs none.
			name: "probe",
			gone: "stderr",
			arrange: func(e *env) (string, []string) {
				return e.sessionPayload("start", testSession), []string{"probe", "sideways"}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			live := newEnv(t)
			stdin, args := c.arrange(live)
			res := live.run(stdin, nil, args...)
			if res.exitCode != 0 {
				t.Fatalf("with a reader: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			said := res.stderr
			if c.gone == "stdout" {
				said = res.stdout
				var out recapOutput
				if err := json.Unmarshal([]byte(res.stdout), &out); err != nil || out.SystemMessage == "" {
					t.Fatalf("with a reader, stdout is not one JSON object carrying a line "+
						"(%v): %q -- the arrangement no longer makes this entry speak", err, res.stdout)
				}
			}
			if said == "" {
				t.Fatalf("with a reader, %s is empty -- the arrangement no longer makes this "+
					"entry write, so the broken-pipe run below would prove nothing", c.gone)
			}

			e := newEnv(t)
			stdin, args = c.arrange(e)
			cmd := e.command(stdin, nil, args...)
			var open bytes.Buffer
			w := brokenPipe(t)
			if c.gone == "stdout" {
				cmd.Stdout, cmd.Stderr = w, &open
			} else {
				cmd.Stdout, cmd.Stderr = &open, w
			}
			if err := cmd.Run(); err != nil && cmd.ProcessState == nil {
				t.Fatalf("starting %v: %v", cmd.Args, err)
			}
			if code := cmd.ProcessState.ExitCode(); code != 0 {
				t.Fatalf("with its %s reader gone: %s (exit code %d), want exit status 0 -- "+
					"a write to a pipe nobody reads killed a hook entry", c.gone, cmd.ProcessState, code)
			}
			if open.Len() != 0 {
				t.Errorf("with its %s reader gone, the entry wrote %q to the stream still open, "+
					"want nothing", c.gone, open.String())
			}
			for _, marker := range []string{"panic:", "goroutine ", "broken pipe"} {
				if strings.Contains(open.String(), marker) {
					t.Errorf("the open stream carries %q: %q", marker, open.String())
				}
			}
		})
	}
}

// TestH111_ACommandAPersonTypesKeepsTheDefault is the other side of the
// same line. An ignored SIGPIPE survives exec, and a person who pipes
// `rashomon report` into head expects it to stop the way any command does
// there, killed by the signal and silent, not with an error on stderr. So the
// hook entry points set the disposition and the process as a whole does not.
// A fix applied to every command would pass the test above and fail this one.
func TestH111_ACommandAPersonTypesKeepsTheDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no SIGPIPE: a write to a pipe nobody reads is an error there for every command")
	}
	e := newEnv(t)
	cmd := e.command("", nil, "report")
	var open bytes.Buffer
	cmd.Stdout, cmd.Stderr = brokenPipe(t), &open
	if err := cmd.Run(); err != nil && cmd.ProcessState == nil {
		t.Fatalf("starting %v: %v", cmd.Args, err)
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGPIPE {
		t.Fatalf("report with its stdout reader gone: %s, stderr %q; want it ended by SIGPIPE "+
			"-- the disposition the hook entries set has reached a command a person types",
			cmd.ProcessState, open.String())
	}
}

// brokenPipe returns the write end of a pipe whose read end is already
// closed, which is the state a hook's stream is in once Claude Code has
// stopped reading it. Nothing else holds a read end: os.Pipe's descriptors
// are close-on-exec, and exec.Cmd hands a child only the three it is given.
func brokenPipe(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

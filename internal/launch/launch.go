// Package launch runs the user's own command with the proxy variables set.
//
// It is a SEPARATE PACKAGE because of what it has to import. os/exec is
// forbidden in the recorder's dependency graph -- code that runs inside an
// agent's tool calls, thousands of times a session, must not be able to spawn a
// process, because a process is a path to the network. Keeping the launcher
// here means the forbidden import lands in a package the recorder does not
// reach, and a test asserts that internal/hook never imports this one, so the
// recorder cannot acquire an exec or a signal handler later by accident.
package launch

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/altrace-dev-role/rashomon/internal/safe"
)

// NoProxyValue is what NO_PROXY is set to, and it matches `rashomon env` for a
// reason: two spellings of the same list would eventually differ, and the one
// that differed would be the one somebody debugged for an hour.
const NoProxyValue = "localhost,127.0.0.1,::1,0.0.0.0,*.local"

// Env is the variables to export, or nothing.
//
// HTTPS only. Plain HTTP is not observed in this release, so exporting
// HTTP_PROXY would route traffic through a proxy that does not record it and
// then report nothing: silence read as zero. The lowercase form is not a
// duplicate -- curl reads only lowercase https_proxy.
func Env(listenAddr string) []string {
	addr := "http://" + listenAddr
	return []string{
		"HTTPS_PROXY=" + addr,
		"https_proxy=" + addr,
		"NO_PROXY=" + NoProxyValue,
	}
}

// Run spawns argv with the given extra environment, forwards signals to it, and
// returns its exit code.
//
// SPAWN AND WAIT, not exec-replace. Replacing this process with the child would
// be simpler and is the usual shape for a wrapper, but it would make the
// post-exit report impossible: there would be no process left to render it.
// Waiting costs one live process for the duration of the session and buys the
// only moment at which the report can be produced automatically.
//
// SIGNALS ARE FORWARDED rather than left to the terminal. The child is in this
// process's group, so an interactive Ctrl-C would reach it anyway -- but a
// SIGTERM sent to this process specifically (a supervisor, a timeout, a kill)
// would not, and the child would be orphaned and keep running against a
// recorder that had stopped. Forwarding makes one signal mean one thing.
//
// The exit code is the CHILD's. A wrapper that returned its own status would
// break every script that checks the exit code of the command it thought it was
// running.
func Run(argv []string, extraEnv []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(argv) == 0 {
		return 1, fmt.Errorf("launch: no command given")
	}

	path, err := exec.LookPath(argv[0])
	if err != nil {
		return 1, fmt.Errorf("launch: %s: %w", argv[0], err)
	}

	cmd := exec.Command(path, argv[1:]...) //nolint:gosec // the command is the user's own argv
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("launch: starting %s: %w", argv[0], err)
	}

	// Relay every signal a supervisor might send, for as long as the child
	// runs. The channel is buffered because signal.Notify drops on a full
	// channel, and a dropped SIGTERM is a child that outlives its wrapper.
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	stop := make(chan struct{})
	// safe.Go rather than a bare `go`: internal/safe documents itself as the
	// only sanctioned way to start a goroutine here, because a panic raised in
	// one cannot be recovered by the starter and the runtime would terminate
	// the process with exit 2. The relay's cancellation is the stop channel,
	// closed before this function returns.
	relayed := safe.Go(func() {
		for {
			select {
			case s := <-sigs:
				if cmd.Process != nil {
					// Best effort: the child may have exited between the
					// signal arriving and this line, which is not an error and
					// there is nothing to do about it either way.
					cmd.Process.Signal(s) //nolint:errcheck // racing the child's own exit
				}
			case <-stop:
				return
			}
		}
	}, nil)

	waitErr := cmd.Wait()
	// Stop the relay and WAIT for it, so the goroutine cannot outlive this
	// call and the handler is uninstalled before whatever runs next in this
	// process -- the report, in the only caller.
	close(stop)
	signal.Stop(sigs)
	<-relayed

	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), nil
	}
	if waitErr != nil {
		return 1, waitErr
	}
	return 0, nil
}

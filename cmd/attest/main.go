// Command attest records what a Claude Code session asked to run.
//
// This program never exits with status 2. Exit code 2 from a PreToolUse hook
// blocks the tool call, and Claude Code documents that a JSON permissionDecision
// of allow cannot override it. A recorder that blocks the thing it is recording
// has stopped being a recorder, so 2 is not a status this binary is allowed to
// produce -- not for a usage error, not for a missing store, not for a panic.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/altrace-dev-role/altrace-attest/internal/hook"
	"github.com/altrace-dev-role/altrace-attest/internal/safe"
	"github.com/altrace-dev-role/altrace-attest/internal/store"
)

// version is set at build time.
var version = "dev"

// The only two statuses this program produces. Notably absent: 2.
const (
	exitOK   = 0
	exitFail = 1
)

func main() {
	// os.Exit skips deferred functions, so every path that needs one has to
	// have returned before this line is reached.
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitFail
	}

	switch args[0] {
	case "hook":
		return cmdHook(stdin, stderr)

	case "version":
		fmt.Fprintln(stdout, version)
		return exitOK

	case "watch", "detach", "report", "forget":
		fmt.Fprintf(stderr, "attest: %s is not implemented yet\n", args[0])
		return exitFail

	default:
		usage(stderr)
		return exitFail
	}
}

// cmdHook handles one PreToolUse invocation and always succeeds.
//
// Every failure below is swallowed into exit 0 on purpose. There is no failure
// of this program that is worth blocking a user's tool call over, and a
// recorder that cannot record has no standing to veto anything. What it can do
// honestly is decline to claim coverage it does not have, which is what the
// coverage record is for.
//
// Nothing is written to stdout on this path. Claude Code parses hook stdout as
// control output, so anything printed there is a second way to affect a
// decision this program has no business affecting.
func cmdHook(stdin io.Reader, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, err := store.DefaultRoot()
	if err != nil {
		fmt.Fprintln(stderr, "attest: store location unresolved")
		return exitOK
	}

	st, err := store.Open(root)
	if err != nil {
		// Nowhere to record that recording failed. The absence of any record
		// for this session is the signal, and report reads it as one.
		fmt.Fprintln(stderr, "attest: store unavailable")
		return exitOK
	}

	h := hook.New(st, time.Now)
	captureErr := safe.Guard(func() error { return h.Capture(stdin) })
	_ = safe.Guard(func() error { h.Close(ctx, captureErr); return nil })

	return exitOK
}

func usage(w io.Writer) {
	fmt.Fprint(w, `attest -- record what a Claude Code session asked to run

usage:
  attest watch     install the PreToolUse hook entry
  attest detach    remove it
  attest hook      handle one PreToolUse invocation (called by Claude Code)
  attest report    render a run's declarations and coverage
  attest forget    evict records, leaving a coverage gap behind
  attest version   print the version
`)
}

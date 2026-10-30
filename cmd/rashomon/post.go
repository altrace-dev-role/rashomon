package main

import (
	"fmt"
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// cmdPost handles one PostToolUse invocation, under the same rule as hook: it
// is invoked by Claude Code and it always succeeds. Once the record is
// durable it runs the steer step and prints what the step returns, the one
// line it writes to stdout; a record that may be lost is not cited, so after
// a failed Flush the step does not run (card 2.3 R1).
//
// A declaration is a request. This is what closes it: the record that the call
// the agent asked for went on to run.
func cmdPost(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Guarded whole, for the reason given in cmdHook.
	return guardedHook(stderr, func() error {
		hook.IgnoreBrokenPipe()
		sig := hook.WatchSignals()
		defer sig.Stop()

		payload, in, standDown, id := claudeHostGuard(stdin)
		if standDown {
			return nil
		}

		if checkPaused(args, in(), store.PhasePost, stderr) {
			return nil
		}

		st := openForHook(args, stderr)
		if st == nil {
			return nil
		}
		if standsDown(args, st, stderr) {
			return nil
		}

		p := hook.NewPost(st, time.Now)
		// Group commit, as in cmdHook.
		st.DeferDurability()
		// See cmdHook's own comment on payload != nil.
		captureErr := safe.Guard(func() error {
			if payload != nil {
				return p.CapturePayload(payload)
			}
			return p.Capture(in())
		})
		// As in cmdHook: Guard is for Close's panic, and a coverage record
		// Close did not write reads as its absence (H-10).
		_ = safe.Guard(func() error { p.Close(sig, captureErr); return nil })
		if err := safe.Guard(p.Flush); err != nil {
			fmt.Fprintln(stderr, flushLostLine)
			return nil
		}
		if b := p.Steer(steerEnv(id, time.Now(), st.Root())); len(b) > 0 {
			fmt.Fprintln(stdout, string(b))
		}
		return nil
	})
}

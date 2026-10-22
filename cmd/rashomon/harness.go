package main

import (
	"fmt"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/hook"
)

// harnessOrStandDown reads --harness off a hook command line. On a harness
// this build does not know it prints the fixed stand-down line and returns
// false, and the caller writes nothing at all -- not even a coverage record,
// whose fields would be resolved under rules that do not apply.
//
// It lives here rather than in main.go so the entry points carry one call
// each and nothing more.
func harnessOrStandDown(args []string, stderr io.Writer) (hook.Harness, bool) {
	h, ok := hook.HarnessArg(args)
	if !ok {
		fmt.Fprintln(stderr, hook.UnknownHarnessLine)
	}
	return h, ok
}

// recapSpeaksFor reports whether recap may print for this command line, and
// under which harness.
//
// Under Codex it speaks only at Stop. Codex shows a Stop hook's JSON
// systemMessage to the user as a warning; only a decision to block, or exit
// 2, would make the agent continue, and recap writes neither (Codex's Stop
// output schema and its hooks page). The pending catch-up runs on
// UserPromptSubmit, where Codex adds a hook's plain stdout to the model's
// context: one malformed line there would reach the agent, which this program
// must never do, so under Codex it prints nothing, installed or not. An
// unknown harness stands down, as the recorders do.
func recapSpeaksFor(args []string, stderr io.Writer) (hook.Harness, bool) {
	h, ok := harnessOrStandDown(args, stderr)
	if !ok || (h == hook.Codex && recapPending(args)) {
		return h, false
	}
	return h, true
}

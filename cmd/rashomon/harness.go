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

// recapSpeaksFor reports whether recap may print for this command line.
//
// Under Codex it never does. recap's line is a JSON systemMessage on a Stop
// hook's stdout, which is how Claude Code shows a hook's words to a person;
// how Codex reads a Stop hook's stdout has not been measured, and an output
// it took for an instruction would change what the agent does next -- the one
// effect this program must never have. An unknown harness stands down, as
// the recorders do.
func recapSpeaksFor(args []string, stderr io.Writer) bool {
	h, ok := harnessOrStandDown(args, stderr)
	return ok && h == hook.ClaudeCode
}

package hook

// Harness names the agent whose hook system invoked this process.
//
// It is passed on the installed command line, never inferred from the
// payload. Two agents can send payloads with the same keys and different
// meanings -- Claude Code's PostToolUse means the call succeeded, Codex's
// says only that it ended -- so guessing from the shape would be guessing at
// the one thing the record has to get right.
type Harness string

const (
	// ClaudeCode is the harness of a command line that names none: every entry
	// installed before the flag existed, and every one watch installs.
	ClaudeCode Harness = ""
	// Codex is the Codex CLI.
	Codex Harness = "codex"
)

// HarnessFlag introduces the harness on a hook command line.
const HarnessFlag = "--harness"

// UnknownHarnessLine is the whole of what an entry naming a harness this build
// does not know says. Hook stderr reaches the agent's debug log, so the value
// that was passed is not repeated.
const UnknownHarnessLine = "rashomon: unknown harness; standing down"

// HarnessArg returns the harness a hook command line names, and false when it
// names one this build does not know, or gives the flag no value.
//
// An unknown harness stands down rather than falling back to Claude Code: a
// payload read under the wrong harness's rules records a fact that is wrong,
// and writing nothing is the smaller error.
func HarnessArg(args []string) (Harness, bool) {
	for i, a := range args {
		if a != HarnessFlag {
			continue
		}
		if i+1 < len(args) && Harness(args[i+1]) == Codex {
			return Codex, true
		}
		return "", false
	}
	return ClaudeCode, true
}

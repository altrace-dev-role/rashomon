// Package selfguard tells a command typed by a person from one run inside an
// agent session, so an expansion -- watch, consent -- is never accepted on
// the agent's behalf (Technical spec 7.2.4).
//
// The commands that refuse an agent session: watch and consent (an
// expansion, and consent <unit> off), spec trust and untrust, steer on and
// off, summary on and off, bodies on and off, purge, detach and pause. Each
// reads AgentSignal before it opens or writes anything, and returns Refusal.
//
// It detects and announces; it does not prevent. A process running as the
// user can still edit rashomon's files directly.
//
// This build checks one of the specification's three agent signals, the
// CLAUDECODE variable Claude Code sets in every process it starts. The
// other two -- a recorded declaration in the last 120 s naming the verb or
// binary, and a claude (or listed harness) ancestor process -- need the
// record reader and a process-table walk, and are added with the rest of
// the guard.
package selfguard

import "fmt"

// EnvClaudeCode is the variable Claude Code sets in the environment of every
// process it runs.
const EnvClaudeCode = "CLAUDECODE"

// AgentSignal returns the name of the agent signal present, or "" when none
// is. getenv is os.Getenv in production.
func AgentSignal(getenv func(string) string) string {
	if getenv(EnvClaudeCode) != "" {
		return EnvClaudeCode
	}
	return ""
}

// Place is where a person runs a command an agent session may not. A `!`
// line typed into Claude Code carries CLAUDECODE as the agent's own commands
// do (2.1.280, measured), so "your own terminal" would read as the place the
// person already is.
const Place = "a terminal outside Claude Code"

// Refusal is the error a guarded command returns when signal is set (#177
// review D1, D2): why the command is the person's alone, then the command and
// where to run it, then what was left as it was, and the signal last, for the
// person who asks how the session was told apart. why is a clause, "purge
// deletes your record"; command is what follows "rashomon "; nothing is the
// participle for what was not done, "written" or "removed". The last
// sentence speaks of CLAUDECODE, the one signal this build reads.
func Refusal(signal, why, command, nothing string) error {
	return fmt.Errorf("%s, so only you can run it, in %s: `rashomon %s`. Nothing was %s. (%s is set here: Claude Code sets it for every command it runs, a `!` line included.)",
		why, Place, command, nothing, signal)
}

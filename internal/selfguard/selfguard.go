// Package selfguard tells a command typed by a person from one run inside an
// agent session, so an expansion -- watch, consent -- is never accepted on
// the agent's behalf (Technical spec 7.2.4).
//
// The commands that refuse an agent session: watch and consent (an
// expansion, and consent <unit> off), spec trust and untrust, steer on and
// off, summary on and off, bodies on and off, and purge. Each reads
// AgentSignal before it opens or writes anything.
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

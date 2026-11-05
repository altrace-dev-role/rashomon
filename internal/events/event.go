// Package events is the canonical event every hook-driven path consumes
// (AD9, AD19): one agent-free type and a closed vocabulary. An agent
// adapter (internal/hookio for Claude Code) is the only producer: it maps
// its hook names, sources and payload keys onto these kinds, so a core
// package that reads an agent's key names is a defect
// (graphtest.TestCore_NoAgentKeys). M1 fixes the type and the vocabulary;
// the paths that consume it are M4's (boundaries) and M7's (adapters), and
// the legacy paths migrate in M7.
package events

import (
	"encoding/json"
	"slices"
)

// Kind is what happened, in the vocabulary of Part B6.
type Kind string

// The kinds. A session start carries the agent's source word as its Form
// (startup for a fresh session); a boundary carries one of Boundaries; the
// plan kind is the plan-mode exit (W-M2.6); an act is a tool call before
// it runs (declared), after it ran (landed) or after it failed.
const (
	SessionStart Kind = "session_start"
	Prompt       Kind = "prompt"
	Plan         Kind = "plan"
	ActDeclared  Kind = "act_declared"
	ActLanded    Kind = "act_landed"
	ActFailed    Kind = "act_failed"
	Stop         Kind = "stop"
	StopFailed   Kind = "stop_failed"
	SessionEnd   Kind = "session_end"
	Boundary     Kind = "boundary"
)

// The boundary forms: the moments session state is lost or handed over
// (C7, AD7), each a Form on a Boundary event.
const (
	Compact       = "compact"
	Resume        = "resume"
	Clear         = "clear"
	SubagentStart = "subagent_start"
	SubagentStop  = "subagent_stop"
)

// Kinds is the closed list, a fresh slice each call.
func Kinds() []Kind {
	return []Kind{SessionStart, Prompt, Plan, ActDeclared, ActLanded, ActFailed, Stop, StopFailed, SessionEnd, Boundary}
}

// Boundaries is the closed list of boundary forms, a fresh slice each call.
func Boundaries() []string {
	return []string{Compact, Resume, Clear, SubagentStart, SubagentStop}
}

// Known reports whether kind and form are in the vocabulary: a boundary
// needs a boundary form, a session start takes the agent's source word or
// none, and every other listed kind takes no form.
func Known(k Kind, form string) bool {
	switch k {
	case Boundary:
		return slices.Contains(Boundaries(), form)
	case SessionStart:
		return true
	}
	return form == "" && slices.Contains(Kinds(), k)
}

// Agent names the producer: the agent's name and version as the adapter
// reads them.
type Agent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Event is one canonical event: its kind and form, the record seq it was
// recorded at (0 before it is), the session, the agent, the time in Unix
// milliseconds, the repository id (Part B2) and the adapter's payload in
// the canonical field names, left to the consumer to decode.
type Event struct {
	Kind    Kind            `json:"kind"`
	Form    string          `json:"form,omitempty"`
	Seq     int64           `json:"seq"`
	Session string          `json:"session"`
	Agent   Agent           `json:"agent"`
	At      int64           `json:"at"`
	Repo    string          `json:"repo"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

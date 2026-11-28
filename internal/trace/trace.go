// Package trace is the agent-neutral model of a coding session as a sequence
// of steps: what the agent said and called, what each call answered, and what
// arrived in the person's slot of the conversation. Readers that learn from
// sessions (the audit fold, the episode detectors) consume Steps and never an
// agent's file format; each agent's format is read by its own adapter package
// under this one (trace/claudecode), the only place that agent's words appear.
//
// A Step carries the agent's own payload for a call (Call.Tool, Call.Input) as
// data, for packages that speak that agent's tool vocabulary (internal/shape).
// Code that is meant to work for any agent reasons on ActClass, Origin, Signal,
// Denial and ToolError, never on those two fields.
package trace

import (
	"encoding/json"
	"time"
)

// Kind is what a step is.
type Kind uint8

const (
	// Entry: the source began a record. Its stamp, session, working
	// directory and sidechain flag hold for the steps that follow it until
	// the next Entry, and are repeated on each of those steps.
	Entry Kind = iota + 1
	// Message: the agent began a message.
	Message
	// Text: words in the agent's message.
	Text
	// Call: the agent called a tool (an act declared).
	Call
	// Result: a tool's answer to a call (an act landed, failed or refused).
	Result
	// Said: a line in the person's slot of the conversation. Who wrote it
	// is Origin, and a marker the agent's client writes there is Signal.
	Said
)

// Step is one step of a session, in the order the source recorded it.
type Step struct {
	Kind Kind
	// Session is the session the step belongs to, "" when the source names
	// none.
	Session string
	// At is when the source stamped the step's record; zero when it carries
	// no stamp or one that does not parse.
	At time.Time
	// Cwd is the working directory the source recorded with the step, ""
	// when none.
	Cwd string
	// Sidechain is true for a step a subagent wrote into the main record.
	Sidechain bool

	// Text is the words of a Text or Said step, as recorded (untrimmed).
	Text string
	// Origin says who wrote a Said step's Text.
	Origin Origin
	// Signal is the marker a Said step stands for, SignalNone for words.
	Signal Signal

	Call   *CallStep   // set on a Call step
	Result *ResultStep // set on a Result step
}

// ActClass is what kind of act a call is, in words no agent owns. The adapter
// maps its agent's tool names onto these.
type ActClass uint8

const (
	ActOther       ActClass = iota // a tool the adapter does not class
	ActExec                        // runs a command
	ActExecControl                 // reads or stops a command already running
	ActEdit                        // changes part of a file
	ActWrite                       // writes a whole file
	ActRead                        // reads a file
	ActSearch                      // searches files or names
	ActFetch                       // fetches or searches the web
	ActDelegate                    // hands work to a subagent
	ActPlan                        // presents a plan
	ActAsk                         // asks the person a question
	ActExternal                    // a tool an external server provides
)

// CallStep is a call the agent made.
type CallStep struct {
	// ID links the call to its Result.
	ID string
	// Tool is the agent's own name for the tool, kept as data.
	Tool  string
	Class ActClass
	// Input is the agent's own payload for the call, kept as data.
	Input json.RawMessage

	// The fields below are filled when the adapter is asked for content.

	// Command is the command an ActExec call runs, as the agent wrote it.
	Command string
	// Paths are the files or directories the call names, as written.
	Paths []string
	// Changes are what an ActEdit or ActWrite call puts in place of what.
	Changes []Change
}

// Change is one replacement an edit makes in a file: Old is "" for a whole
// file written.
type Change struct {
	Path, Old, New string
}

// ResultStep is a tool's answer to a call.
type ResultStep struct {
	// CallID is the ID of the call answered; it may name no call the
	// reader has seen.
	CallID string
	// Failed: the call did not succeed (an error, a refusal or a denial).
	Failed bool
	// Denied says who refused the call, DeniedNone when nobody did.
	Denied Denial
	// Error is the kind of tool error a failed, undenied call hit,
	// ErrorNone when it is not one the adapter knows. A command's own
	// failure is not a tool error.
	Error ToolError
	// Text is the answer's text, kept for a failed call only.
	Text string
	// Output is the answer's text for every call, filled when the adapter
	// is asked for content.
	Output string
}

// Origin is who wrote a Said step's words.
type Origin uint8

const (
	// OriginUnknown: the source does not say.
	OriginUnknown Origin = iota
	// OriginPerson: the source marks the words as typed by the person.
	OriginPerson
	// OriginPeer: the source marks the words as another session's.
	OriginPeer
	// OriginMachine: the agent's client, a hook or a command wrote the
	// words in the person's slot (markup, a command's output).
	OriginMachine
)

// Signal is a marker the agent's client writes in the person's slot.
type Signal uint8

const (
	SignalNone Signal = iota
	// SignalInterrupt: the person stopped the agent mid-turn.
	SignalInterrupt
	// SignalInterruptForTool: the stop that follows the person refusing a
	// call; the refusal, not the marker, is the person's act.
	SignalInterruptForTool
)

// Denial is who refused a call.
type Denial uint8

const (
	DeniedNone Denial = iota
	// DeniedByPerson: the person said no to the call.
	DeniedByPerson
	// DeniedByRule: a hook or a settings rule refused it.
	DeniedByRule
	// DeniedOther: refused, by something other than the person.
	DeniedOther
	// DeniedUnrecognised: refused, with a reason the adapter does not know.
	DeniedUnrecognised
)

// ToolError is a kind of tool error, an agent's tool refusing or failing a
// call before it acted, in words no agent owns.
type ToolError uint8

const (
	ErrorNone ToolError = iota
	// ErrorEditNotFound: the text an edit replaces is not in the file.
	ErrorEditNotFound
	// ErrorEditAmbiguous: the text an edit replaces is there more than once.
	ErrorEditAmbiguous
	// ErrorEditNoChange: the edit would change nothing.
	ErrorEditNoChange
	// ErrorStaleWrite: the file changed since the agent read it.
	ErrorStaleWrite
	// ErrorUnreadWrite: the agent wrote a file it had not read.
	ErrorUnreadWrite
	// ErrorTooLarge: the file is too large to read whole.
	ErrorTooLarge
	// ErrorNoSuchFile: the file the call names does not exist.
	ErrorNoSuchFile
	// ErrorBadInput: the tool refused the call's input as malformed.
	ErrorBadInput
)

// Package steer is STEER's output and line-record contract: the closed
// vocabularies a Finding is built from, the Output JSON Marshal
// serialises to hookSpecificOutput, and the steer.ndjson Line every
// delivered, suppressed or deferred line writes. Technical spec 4.2.3
// (data model) and 4.2.4 (the router that is the only path to Output).
//
// Contract only (P-07): the types, the closed lists, and Output.Marshal's
// key allowlist, all of which are pure data shape and so are implemented
// for real rather than stubbed. Route itself is the one stub in this
// package -- see route.go -- because deciding which Finding wins needs
// the record and the rules a later wave reads, not just the shape of what
// it returns.
//
// None of internal/steer, internal/steer/render or
// internal/steer/render/slot imports net, net/http, crypto/tls, os/exec,
// internal/hook, internal/report (or report/transcript), or
// internal/store: steer answers "what should be said," never "what
// happened" or "where does it go."
package steer

// Level is how hard a delivered finding pushes: a flag is visible but
// inert, a nudge adds context the model may act on, and a stop can hold or
// deny the call outright. The technical spec's Vocabularies list (Contracts
// and names) orders them off < flag < nudge < stop, and the constants
// below are assigned in that order on purpose -- comparing two Levels with
// <, <=, and so on already gives the right answer, which is why this
// package adds no separate Less/Compare method.
type Level uint8

const (
	LevelOff Level = iota
	LevelFlag
	LevelNudge
	LevelStop
)

// levelNames is a table, not a switch, for the reason budget.Phase's
// reasons table is one (internal/budget/budget.go): the word is the
// contract, and a table is harder to reword by accident than a case arm
// is.
var levelNames = [...]string{
	LevelOff:   "off",
	LevelFlag:  "flag",
	LevelNudge: "nudge",
	LevelStop:  "stop",
}

// String renders Level the way it appears on a steer.ndjson line's
// "level" key (Technical spec 4.2.3).
func (l Level) String() string {
	if int(l) < len(levelNames) {
		return levelNames[l]
	}
	return "unknown"
}

// AllLevels returns the four levels in ascending severity order.
func AllLevels() []Level {
	return []Level{LevelOff, LevelFlag, LevelNudge, LevelStop}
}

// Channel is the hook phase a Finding was produced for, spelled the way
// the steer.ndjson "channel" key spells it -- lower_snake_case, distinct
// from the PascalCase event name Output.Marshal's event argument and the
// ndjson "event" key use. Both exist on a Line side by side in tech 4.2.3's
// example; this package freezes both spellings rather than deriving one
// from the other, so a typo in either is caught by its own test instead of
// by a conversion function silently doing the wrong thing.
type Channel string

const (
	ChannelPreToolUse       Channel = "pre_tool_use"
	ChannelPostToolUse      Channel = "post_tool_use"
	ChannelPostToolBatch    Channel = "post_tool_batch"
	ChannelUserPromptSubmit Channel = "user_prompt_submit"
	ChannelSessionStart     Channel = "session_start"
	ChannelSubagentStart    Channel = "subagent_start"
	ChannelStop             Channel = "stop"
	ChannelPostModelSwitch  Channel = "post_model_switch"
)

// AllChannels returns the eight channels in the order tech 4.2.3 lists
// them.
func AllChannels() []Channel {
	return []Channel{
		ChannelPreToolUse, ChannelPostToolUse, ChannelPostToolBatch,
		ChannelUserPromptSubmit, ChannelSessionStart, ChannelSubagentStart,
		ChannelStop, ChannelPostModelSwitch,
	}
}

// Event is a hook event STEER may print on, spelled the way Claude Code's
// own hookEventName does -- PascalCase -- because Output.Marshal's event
// parameter and the ndjson "event" key are wire values this program does
// not control the spelling of. There are nine: the eight moments Channel
// names, plus PostToolUseFailure, which has no channel of its own in tech
// 4.2.3's list; `post` serves it beside PostToolUse (tech 4.2.1). See
// Channel's comment for why the two vocabularies are kept separate.
type Event string

const (
	EventPreToolUse         Event = "PreToolUse"
	EventPostToolUse        Event = "PostToolUse"
	EventPostToolUseFailure Event = "PostToolUseFailure"
	EventPostToolBatch      Event = "PostToolBatch"
	EventUserPromptSubmit   Event = "UserPromptSubmit"
	EventSessionStart       Event = "SessionStart"
	EventSubagentStart      Event = "SubagentStart"
	EventStop               Event = "Stop"
	EventPostModelSwitch    Event = "PostModelSwitch"
)

// AllEvents returns the nine events: AllChannels' order, with
// PostToolUseFailure after PostToolUse. The two lists do not zip.
func AllEvents() []Event {
	return []Event{
		EventPreToolUse, EventPostToolUse, EventPostToolUseFailure,
		EventPostToolBatch, EventUserPromptSubmit, EventSessionStart,
		EventSubagentStart, EventStop, EventPostModelSwitch,
	}
}

// Attendance is whether a human was at the keyboard when a Finding fired
// -- the signal the "outcome":"ask_unattended" case reacts to when a
// Stop-level finding needed to ask with nobody there to answer (Technical
// spec 4.2.3).
type Attendance string

const (
	AttendanceAttended   Attendance = "attended"
	AttendanceUnattended Attendance = "unattended"
	AttendanceLooping    Attendance = "looping"
)

// AllAttendance returns the three attendance values.
func AllAttendance() []Attendance {
	return []Attendance{AttendanceAttended, AttendanceUnattended, AttendanceLooping}
}

// Label is the eventual truth value a claim-vs-record (or similar)
// pattern's finding resolves to, once whatever confirms or refutes it has
// run (the Vocabularies list, Contracts and names: "Labels: `confirmed |
// refuted | unresolved`").
type Label string

const (
	LabelConfirmed  Label = "confirmed"
	LabelRefuted    Label = "refuted"
	LabelUnresolved Label = "unresolved"
)

// AllLabels returns the three label values.
func AllLabels() []Label {
	return []Label{LabelConfirmed, LabelRefuted, LabelUnresolved}
}

// StopForm is the shape a Finding wants its Level to take if that Level
// reaches Stop: say nothing special, deny outright, ask the human, or
// hold the turn open with a continuation message. Route (P-30) is what
// turns a winning Finding's StopForm into the right Output shape --
// Decision{Ask: false} for deny, Decision{Ask: true} for ask,
// StopContinue for stop_continue (Technical spec 4.2.3's Finding
// listing).
type StopForm string

const (
	StopFormNone         StopForm = "none"
	StopFormDeny         StopForm = "deny"
	StopFormAsk          StopForm = "ask"
	StopFormStopContinue StopForm = "stop_continue"
)

// AllStopForms returns the four stop forms.
func AllStopForms() []StopForm {
	return []StopForm{StopFormNone, StopFormDeny, StopFormAsk, StopFormStopContinue}
}

// PatternState is whether a pattern is actively steering, visible but
// downgraded to flag-only, or switched off by the user (the Vocabularies
// list, Contracts and names: "`steering | flag_only | off_by_user`, each
// with a `cause`").
type PatternState string

const (
	PatternStateSteering  PatternState = "steering"
	PatternStateFlagOnly  PatternState = "flag_only"
	PatternStateOffByUser PatternState = "off_by_user"
)

// AllPatternStates returns the three pattern states.
func AllPatternStates() []PatternState {
	return []PatternState{PatternStateSteering, PatternStateFlagOnly, PatternStateOffByUser}
}

// Cause is why a PatternState holds (archive 50.2.3): the eight closed
// values below. STEER's "demoted" is the flag_only state, and "user_set"
// in the spec's prose means any user_* cause.
type Cause string

const (
	CauseInitial        Cause = "initial"
	CauseAutoDemoted    Cause = "auto_demoted"
	CauseAutoRepromoted Cause = "auto_repromoted"
	CauseFlapHold       Cause = "flap_hold"
	CauseUserOn         Cause = "user_on"
	CauseUserOff        Cause = "user_off"
	CauseUserSetLevel   Cause = "user_set_level"
	CauseVersionChanged Cause = "version_changed"
)

// AllCauses returns the eight causes in the archive Name registry's order.
func AllCauses() []Cause {
	return []Cause{
		CauseInitial, CauseAutoDemoted, CauseAutoRepromoted, CauseFlapHold,
		CauseUserOn, CauseUserOff, CauseUserSetLevel, CauseVersionChanged,
	}
}

// LineState is the lifecycle state a steer.ndjson line's "state" key
// carries (Technical spec 4.2.3): candidate before a decision, delivered
// or suppressed once Route has one, superseded/stale/deferred/expired for
// the ways a candidate can stop being current without ever being
// delivered, and unconfirmed for a delivered line whose invocation was
// killed or dropped before anything could confirm it reached its
// destination.
type LineState string

const (
	LineStateCandidate  LineState = "candidate"
	LineStateDelivered  LineState = "delivered"
	LineStateSuppressed LineState = "suppressed"
	LineStateSuperseded LineState = "superseded"
	LineStateStale      LineState = "stale"
	LineStateDeferred   LineState = "deferred"
	LineStateExpired    LineState = "expired"
	// LineStateUnconfirmed carries a null delivery and counts in no value
	// line, label or budget (tech 4.2.3: "`unconfirmed` carries null and
	// counts in no value line, label or budget"): a line reaches this state
	// only after its invocation is already gone, so nothing downstream may
	// treat it as evidence of anything.
	LineStateUnconfirmed LineState = "unconfirmed"
)

// AllLineStates returns the eight states Technical spec 4.2.3 lists, in
// the order given there.
func AllLineStates() []LineState {
	return []LineState{
		LineStateCandidate, LineStateDelivered, LineStateSuppressed,
		LineStateSuperseded, LineStateStale, LineStateDeferred,
		LineStateExpired, LineStateUnconfirmed,
	}
}

// SuppressedBy is the closed list of reasons a candidate line did not
// reach delivered or flag_only (Technical spec 4.2.3) -- exactly 22
// values, pinned by TestSuppressedBy_ExactlyTwentyTwoValues so that
// adding a 23rd without updating this comment and that test is
// impossible to do quietly.
type SuppressedBy string

const (
	SuppressedByBudgetPrompt      SuppressedBy = "budget_prompt"
	SuppressedByBudgetPattern     SuppressedBy = "budget_pattern"
	SuppressedByBudgetBatch       SuppressedBy = "budget_batch"
	SuppressedByStopHookActive    SuppressedBy = "stop_hook_active"
	SuppressedByBackgroundRunning SuppressedBy = "background_running"
	SuppressedByHostForeign       SuppressedBy = "host_foreign"
	SuppressedBySteerOff          SuppressedBy = "steer_off"
	SuppressedByConsentMissing    SuppressedBy = "consent_missing"
	SuppressedByArmOff            SuppressedBy = "arm_off"
	SuppressedByDemoted           SuppressedBy = "demoted"
	SuppressedByLockTimeout       SuppressedBy = "lock_timeout"
	SuppressedByRenderRefused     SuppressedBy = "render_refused"
	SuppressedByDeadline          SuppressedBy = "deadline"
	SuppressedByDenyCap           SuppressedBy = "deny_cap"
	SuppressedBySlotRejected      SuppressedBy = "slot_rejected"
	SuppressedByOutputGate        SuppressedBy = "output_gate"
	SuppressedByUnverifiedChannel SuppressedBy = "unverified_channel"
	SuppressedByNoChannel         SuppressedBy = "no_channel"
	SuppressedByNoMirror          SuppressedBy = "no_mirror"
	SuppressedByHarnessOff        SuppressedBy = "harness_off"
	SuppressedBySurfaceOff        SuppressedBy = "surface_off"
	SuppressedByLate              SuppressedBy = "late"
)

// AllSuppressedBy returns the 22 reasons in the order Technical spec
// 4.2.3 lists them.
func AllSuppressedBy() []SuppressedBy {
	return []SuppressedBy{
		SuppressedByBudgetPrompt, SuppressedByBudgetPattern, SuppressedByBudgetBatch,
		SuppressedByStopHookActive, SuppressedByBackgroundRunning, SuppressedByHostForeign,
		SuppressedBySteerOff, SuppressedByConsentMissing, SuppressedByArmOff,
		SuppressedByDemoted, SuppressedByLockTimeout, SuppressedByRenderRefused,
		SuppressedByDeadline, SuppressedByDenyCap, SuppressedBySlotRejected,
		SuppressedByOutputGate, SuppressedByUnverifiedChannel, SuppressedByNoChannel,
		SuppressedByNoMirror, SuppressedByHarnessOff, SuppressedBySurfaceOff,
		SuppressedByLate,
	}
}

// Delivery is what a Line's state and suppression reason, taken
// together, resolve to for reporting: delivered at nudge or stop,
// flag_only at flag level, holdout or shadow for the two experiment arms
// that withhold delivery on purpose, and suppressed_budget specifically
// for a budget_* SuppressedBy (Technical spec 4.2.3). It is not derivable
// from LineState alone -- that derivation is Route's job (P-30) -- so
// this package only freezes the five values, not the function that picks
// one.
type Delivery string

const (
	DeliveryDelivered        Delivery = "delivered"
	DeliveryFlagOnly         Delivery = "flag_only"
	DeliveryHoldout          Delivery = "holdout"
	DeliverySuppressedBudget Delivery = "suppressed_budget"
	DeliveryShadow           Delivery = "shadow"
)

// AllDelivery returns the five delivery values.
func AllDelivery() []Delivery {
	return []Delivery{
		DeliveryDelivered, DeliveryFlagOnly, DeliveryHoldout,
		DeliverySuppressedBudget, DeliveryShadow,
	}
}

// Outcome is a closed, mostly-null annotation on a steer.ndjson line:
// null on every line except the one case Technical spec 4.2.3 names,
// ask_unattended, recorded when a Stop-level ask had nobody attended to
// answer it.
type Outcome string

// OutcomeAskUnattended is the one non-null Outcome value (Technical spec
// 4.2.3).
const OutcomeAskUnattended Outcome = "ask_unattended"

// AllOutcomes returns the one defined outcome value.
func AllOutcomes() []Outcome {
	return []Outcome{OutcomeAskUnattended}
}

// Arm names the experiment arm a Line was rendered under (archive 40.3.10;
// tech 4.2.9): live on any non-lab install and on a lab install with
// RASHOMON_STEER_ARM unset; steer or off when a lab install reads the
// variable. There is no placebo arm: E-9 moved it into the harness overlay.
type Arm string

const (
	ArmLive  Arm = "live"
	ArmSteer Arm = "steer"
	ArmOff   Arm = "off"
)

// AllArms returns the three arms.
func AllArms() []Arm {
	return []Arm{ArmLive, ArmSteer, ArmOff}
}

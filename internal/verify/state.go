// Package verify folds the record into gate state per point and compares the
// agent's account with it (tech spec 2.2.4 to 2.2.11). P-07 freezes the
// contract: the state and reason vocabularies, the one order over states
// (VER 11, decided (a) 2026-10-12), the Snapshot and checkpoint, the Finding
// and its facts. Evaluate and TurnSummary are stubs that fail closed until
// P-41 and P-45 build them.
package verify

// State is a gate's state at a point (2.2.4).
type State string

const (
	Green              State = "green"
	Red                State = "red"
	MaskedInconclusive State = "masked_inconclusive"
	Stale              State = "stale"
	Absent             State = "absent"
	Backgrounded       State = "backgrounded"
	Unknown            State = "unknown"
)

// States is the closed list in the spec's declaration order (2.2.4).
var States = []State{Green, Red, MaskedInconclusive, Stale, Absent, Backgrounded, Unknown}

// Order is the one ranking of states, worst first. The claim comparator's
// verdict (2.2.5 step 7) and TurnSummary's row (2.2.11) both read it and
// nothing else. A gate's own unknown (spilled record, order_unknown,
// outcome_unknown) ranks below every non-green state and above green: VER 11,
// decided (a) 2026-10-12, so an unknown never hides a red and never reads as
// a pass.
var Order = []State{Red, MaskedInconclusive, Backgrounded, Stale, Absent, Unknown, Green}

// Rank is a state's position in Order; 0 is the worst. A string outside the
// vocabulary ranks before red (-1): a state the code does not know is a
// defect, and a defect must surface as the verdict, never sink to green.
func Rank(s State) int {
	for i, o := range Order {
		if s == o {
			return i
		}
	}
	return -1
}

// Worse reports whether a ranks before b in Order.
func Worse(a, b State) bool { return Rank(a) < Rank(b) }

// Worst is the earliest state of Order present in states; ok is false with
// none, which the comparator reads as no_gate_known.
func Worst(states []State) (worst State, ok bool) {
	for i, s := range states {
		if i == 0 || Worse(s, worst) {
			worst = s
		}
	}
	return worst, len(states) > 0
}

// Verdict is the claim comparator's result (2.2.5 step 7): a State, or
// NoGateKnown when no gate of the claim's kind exists.
type Verdict string

// NoGateKnown is the verdict of a claim no gate can speak to.
const NoGateKnown Verdict = "no_gate_known"

// VerdictOf reduces the states of the gates a claim maps to by Order (2.2.5
// step 7). Incomplete recording silences the claim before this is reached
// (2.2.5, Silence); it is not a verdict.
func VerdictOf(states []State) Verdict {
	w, ok := Worst(states)
	if !ok {
		return NoGateKnown
	}
	return Verdict(w)
}

// Reason is the closed explanation beside a gate's state (2.2.4).
type Reason string

const (
	FailedExit            Reason = "failed_exit"
	FailedOutput          Reason = "failed_output"
	MaskedWithSuccessLine Reason = "masked_with_success_line"
	MaskedNoSuccessLine   Reason = "masked_no_success_line"
	MaskedUnattributable  Reason = "masked_unattributable"
	EmptyRun              Reason = "empty_run"
	PartialAfterRed       Reason = "partial_after_red"
	PartialOnly           Reason = "partial_only"
	RedThenWrites         Reason = "red_then_writes"
	WritesAfterGreen      Reason = "writes_after_green"
	WriteDuringRun        Reason = "write_during_run"
	StaleForeign          Reason = "stale_foreign" // the green run's tree moved with no write of the session's (P5A)
	NeverRunAfterWrite    Reason = "never_run_after_write"
	BackgroundLaunch      Reason = "background_launch"
	OrderUnknown          Reason = "order_unknown"
	OutcomeUnknown        Reason = "outcome_unknown"
	RecordingIncomplete   Reason = "recording_incomplete"
	Budget                Reason = "budget"
	OK                    Reason = "ok"
)

// Reasons is the closed list in the spec's order (2.2.4); DEL-22 pins it.
var Reasons = []Reason{
	FailedExit, FailedOutput,
	MaskedWithSuccessLine, MaskedNoSuccessLine, MaskedUnattributable,
	EmptyRun, PartialAfterRed, PartialOnly,
	RedThenWrites, WritesAfterGreen, WriteDuringRun, StaleForeign, NeverRunAfterWrite,
	BackgroundLaunch, OrderUnknown, OutcomeUnknown, RecordingIncomplete,
	Budget, OK,
}

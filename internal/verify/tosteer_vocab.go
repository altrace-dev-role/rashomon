package verify

import (
	"slices"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// vocabValid checks the closed vocabularies a fact carries, returning the
// reason or "" (P07N-13). idsValid checks the charsets; these values decide
// a candidate's visibility (a rule's trust), its key (a claim's class) and
// whether the fact contradicts anything at all (a gate's state), so an
// unknown value is refused rather than read as any member: an unknown trust
// mapped as trusted would reach the model (2.2.7: "untrusted (default):
// user flags only"). A gate's reason must explain its state (reasonState),
// so a fact never says red for a reason that means green or stale.
func vocabValid(f Finding) string {
	if r := f.Rule; r != nil && !slices.Contains(spec.Trusts, r.Trust) {
		return "rule trust outside its vocabulary"
	}
	if c := f.Claim; c != nil {
		switch {
		case !slices.Contains(ClaimClasses, c.Class):
			return "claim class outside its lexicon"
		case !sourceFits(f.Pattern, c.Source):
			return "claim source outside the pattern's reads"
		case c.Verdict != NoGateKnown && !slices.Contains(States, State(c.Verdict)):
			return "claim verdict outside its vocabulary"
		}
	}
	if g := f.Gate; g != nil {
		if g.Reason != "" && !slices.Contains(Reasons, g.Reason) {
			return "gate reason outside its vocabulary"
		}
		if g.Reason != "" && reasonState()[g.Reason] != g.State {
			return "a gate reason that explains another state"
		}
		if !gateFits(f) {
			return "a gate state that contradicts nothing for the pattern"
		}
	}
	return ""
}

// sourceFits reports a claim read where its pattern reads (2.2.5 input):
// claim-vs-record the final message at Stop; subagent-claim a subagent's
// account at SubagentStop or its handback.
func sourceFits(id pattern.ID, s ClaimSource) bool {
	if id == pattern.ClaimVsRecord {
		return s == SourceFinalMessage
	}
	return s == SourceSubagentStop || s == SourceHandback
}

// subtaskStates are the gate states subtask-gate fires on: no gate run
// exited 0 after the writes (40.4.1 trigger; 20.2.10 "a required gate not
// green"). Green contradicts nothing, unknown never fires (VER-17), and the
// card is silent while a same-family run is backgrounded and unobserved,
// which a PostToolUse or PreToolUse decision cannot tell from finished.
var subtaskStates = []State{Red, MaskedInconclusive, Stale, Absent}

// gateFits reports a gate state that contradicts something for the
// finding's pattern: subtaskStates for subtask-gate; for
// regression-location a recorded red run with an earlier green before it
// (40.4.2: "a gate run of family F ... is red ... F had an earlier green
// run"; a masked run with a failure line is red, 2.2.4 step 5); the card
// cites "this red" run, so a red state with no run seq is refused rather
// than cited by its green alone; beside a claim, the gate
// whose state set the verdict (Finding's comment). factsFit has already
// paired the gate with one of these patterns.
func gateFits(f Finding) bool {
	g := f.Gate
	switch f.Pattern {
	case pattern.SubtaskGate:
		return slices.Contains(subtaskStates, g.State)
	case pattern.RegressionLocation:
		return g.State == Red && g.LastGreenSeq != nil && g.LastRunSeq != nil && *g.LastGreenSeq < *g.LastRunSeq
	}
	return f.Claim != nil && g.State == State(f.Claim.Verdict)
}

// reasonState is the one state each closed reason explains (tech 2.2.4;
// archive 20.2.4, per gate g at point s): the algorithm names a reason only
// beside its state, so a fact pairing a reason with another state
// contradicts itself and is refused (P07N-13). Neither spec names the state
// of `budget` as a gate reason; VER-22's stand-down issues no verdict, so it
// is read as unknown, the state that asserts nothing. `ok` is green's.
func reasonState() map[Reason]State {
	reasonStateOnce.Do(func() {
		reasonStateOnce.v = map[Reason]State{
			FailedExit:            Red,
			FailedOutput:          Red,
			MaskedWithSuccessLine: Green,
			MaskedNoSuccessLine:   MaskedInconclusive,
			MaskedUnattributable:  MaskedInconclusive,
			EmptyRun:              Absent,
			PartialAfterRed:       Red,
			PartialOnly:           Absent,
			RedThenWrites:         Stale,
			WritesAfterGreen:      Stale,
			WriteDuringRun:        Stale,
			StaleForeign:          Stale,
			NeverRunAfterWrite:    Absent,
			BackgroundLaunch:      Backgrounded,
			OrderUnknown:          Unknown,
			OutcomeUnknown:        Unknown,
			RecordingIncomplete:   Unknown,
			Budget:                Unknown,
			OK:                    Green,
		}
	})
	return reasonStateOnce.v
}

// reasonStateOnce holds what reasonState returns, built on its first call and
// never at init (RES-2).
var reasonStateOnce struct {
	sync.Once
	v map[Reason]State
}

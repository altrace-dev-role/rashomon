package pattern

import "testing"

// frozenOrder is the technical spec's "The frozen `steer.PatternID` set"
// paragraph (Contracts and names), copied as string literals and in order.
// Literals, not this package's constants: a list built from the constants
// moves with them, so a renamed id would pass. A change to this slice is a
// contract change, not a test fix.
var frozenOrder = []ID{
	"subtask-gate", "regression-location", "continuity-briefing", "declared-fact",
	"learned-fact", "platform-rule", "integrity", "identical-retry", "cred-spray",
	"ordering-rule", "agent-interference", "subagent-claim", "model-switch",
	"claim-vs-record", "masked-status", "sequence-risk",
}

func sameIDs(got, want []ID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestDEL22_PatternIDSetEqualsTheFrozenList pins the sixteen ids, their
// order, and the closed kind list under each of the six that have one (the
// frozen-set paragraph, decided 2026-10-13). A reorder, an addition, a
// removal, a renamed id or a kind typo all fail here first.
func TestDEL22_PatternIDSetEqualsTheFrozenList(t *testing.T) {
	if got := All(); !sameIDs(got, frozenOrder) {
		t.Fatalf("All() = %v, want %v", got, frozenOrder)
	}

	wantKinds := map[ID][]string{
		ClaimVsRecord: {
			"red", "masked", "stale", "absent", "backgrounded",
			"silent-failure", "done-unverified",
		},
		PlatformRule: {"edit", "command", "pragma"},
		Integrity: {
			"skip-added", "test-deleted", "assert-drop", "frozen-path",
			"tests-only-green", "spec-edited", "steer-config",
			"store-touched", "suppression-added", "generated-edited",
			"gate-weakened", "gate-config-edited",
		},
		SubtaskGate:  {"commit", "push", "pr"},
		LearnedFact:  {"local", "pack"},
		SequenceRisk: {"secret-then-egress", "lockfile-unverified", "dep-unresolved"},
		// declared-fact takes an open "<class>" kind, not a closed list:
		// Kinds returns nil for it, which is why it is absent here rather
		// than mapped to an empty slice.
	}
	for _, id := range frozenOrder {
		want := wantKinds[id] // nil for every ID the spec gives no closed list
		if got := Kinds(id); !sameStrings(got, want) {
			t.Errorf("Kinds(%q) = %v, want %v", id, got, want)
		}
	}
}

// TestSTR16_EveryCandidatePatternHasAPriority enumerates the frozen set:
// every id carries a step 8 number (Technical spec 4.2.4) except the two
// own-channel ids, continuity-briefing and model-switch. integrity (95) and
// sequence-risk (62) are pinned although neither writes a candidate yet,
// because the number also orders flags.
func TestSTR16_EveryCandidatePatternHasAPriority(t *testing.T) {
	noPriority := map[ID]bool{
		ContinuityBriefing: true,
		ModelSwitch:        true,
	}
	for _, id := range frozenOrder {
		p, ok := Priority(id)
		if noPriority[id] {
			if ok {
				t.Errorf("Priority(%q) = (%d, true), want ok=false", id, p)
			}
			continue
		}
		if !ok {
			t.Errorf("Priority(%q) ok=false, want a step 8 value", id)
		}
	}
}

// TestPriority_StepEightValues pins the actual numbers, not just their
// presence, against Technical spec 4.2.4's step 8 table.
func TestPriority_StepEightValues(t *testing.T) {
	want := map[ID]int{
		Integrity:          95,
		SubtaskGate:        90,
		ClaimVsRecord:      85,
		RegressionLocation: 80,
		DeclaredFact:       75,
		LearnedFact:        70,
		CredSpray:          65,
		SequenceRisk:       62,
		PlatformRule:       60,
		AgentInterference:  55,
		SubagentClaim:      50,
		MaskedStatus:       45,
		IdenticalRetry:     40,
		OrderingRule:       30,
	}
	for id, wantP := range want {
		gotP, ok := Priority(id)
		if !ok {
			t.Errorf("Priority(%q) ok=false, want true", id)
			continue
		}
		if gotP != wantP {
			t.Errorf("Priority(%q) = %d, want %d", id, gotP, wantP)
		}
	}
}

func TestValid(t *testing.T) {
	for _, id := range frozenOrder {
		if !Valid(id) {
			t.Errorf("Valid(%q) = false, want true", id)
		}
	}
	if Valid(ID("not-a-real-pattern")) {
		t.Error(`Valid("not-a-real-pattern") = true, want false`)
	}
	if Valid(ID("")) {
		t.Error(`Valid("") = true, want false`)
	}
}

// TestKinds_ReturnsACopy: the kind lists are frozen, so a caller that edits
// what Kinds returned must not edit the table every later caller reads.
func TestKinds_ReturnsACopy(t *testing.T) {
	got := Kinds(SubtaskGate)
	got[0] = "edited"
	if again := Kinds(SubtaskGate); again[0] != "commit" {
		t.Fatalf("Kinds(%q)[0] = %q after a caller edited its copy, want %q", SubtaskGate, again[0], "commit")
	}
}

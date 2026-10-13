package verify

import (
	"errors"
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestVER54e_ToSteerMapsEveryVerifyPattern: every pattern VERIFY raises maps
// to a candidate with the pattern's priority, the proposed level and stop
// form, the fact's seq as AfterSeq, and the dedupe key unchanged; a pattern
// without a priority, a finding without a fact, and an unknown id are
// errors, never a zero-valued candidate.
func TestVER54e_ToSteerMapsEveryVerifyPattern(t *testing.T) {
	seq := int64(48)
	green := int64(37)
	cases := []struct {
		name  string
		f     Finding
		level steer.Level
		form  steer.StopForm
		after int64
		kind  steer.Kind
	}{
		{"claim", Finding{Pattern: pattern.ClaimVsRecord, Class: FalseClaim, DedupeKey: "claim-vs-record|learned:go-test|p1", Claim: &ClaimFact{Class: ClaimTestsPass, Strict: true, Verdict: Verdict(Red)}}, steer.LevelStop, steer.StopFormStopContinue, 212, "red"},
		{"gate", Finding{Pattern: pattern.RegressionLocation, Class: Literal, DedupeKey: "regression-location|learned:go-test|p1", Gate: &GateFact{GateID: "learned:go-test", State: Red, Reason: FailedExit, LastRunSeq: &seq, LastGreenSeq: &green}}, steer.LevelNudge, steer.StopFormNone, 48, ""},
		{"subtask", Finding{Pattern: pattern.SubtaskGate, Class: Absence, DedupeKey: "subtask-gate|learned:go-test|p1", Gate: &GateFact{GateID: "learned:go-test", State: Absent, LastRunSeq: &seq}}, steer.LevelStop, steer.StopFormDeny, 48, ""},
		{"rule", Finding{Pattern: pattern.PlatformRule, Class: Literal, DedupeKey: "platform-rule|no-shell-true|p1", Rule: &RuleFact{RuleID: "no-shell-true", Kind: spec.ForbidCode, Trust: spec.Trusted, Seq: 44}}, steer.LevelStop, steer.StopFormDeny, 44, "forbid_code"},
		{"ordering", Finding{Pattern: pattern.OrderingRule, Class: Literal, DedupeKey: "ordering-rule|export-after-verify|p1", Rule: &RuleFact{RuleID: "export-after-verify", Kind: spec.Order, Trust: spec.Managed, Seq: 50}}, steer.LevelStop, steer.StopFormDeny, 50, ""},
		{"integrity", Finding{Pattern: pattern.Integrity, Class: Literal, DedupeKey: "integrity|skip_added|p1", Integrity: &IntegrityFact{Kind: SkipAdded, Count: 1, Seq: 40}}, steer.LevelFlag, steer.StopFormNone, 40, "skip_added"},
		{"subagent", Finding{Pattern: pattern.SubagentClaim, Class: FalseClaim, DedupeKey: "subagent-claim|a1|p1", Claim: &ClaimFact{Class: ClaimDone, Verdict: Verdict(Stale)}}, steer.LevelFlag, steer.StopFormNone, 212, ""},
	}
	for _, c := range cases {
		got, err := ToSteer(c.f, 212, "a1")
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		wantPriority, _ := pattern.Priority(c.f.Pattern)
		if got.Pattern != c.f.Pattern || got.Key != c.f.DedupeKey || got.Priority != wantPriority || got.AgentID != "a1" {
			t.Errorf("%s: identity lost: %+v", c.name, got)
		}
		if got.MaxLevel != c.level || got.StopForm != c.form || got.AfterSeq != c.after || got.Kind != c.kind {
			t.Errorf("%s: level %v form %q after %d kind %q; want %v %q %d %q", c.name, got.MaxLevel, got.StopForm, got.AfterSeq, got.Kind, c.level, c.form, c.after, c.kind)
		}
		if len(got.Evidence) == 0 {
			t.Errorf("%s: no evidence seq", c.name)
		}
		if !reflect.ValueOf(got.Message).IsZero() {
			t.Errorf("%s: a message was rendered by VERIFY", c.name)
		}
	}
	userOnly := cases[0].f
	userOnly.UserOnly = true
	if got, err := ToSteer(userOnly, 212, ""); err != nil || got.MaxLevel != steer.LevelFlag || got.StopForm != steer.StopFormNone {
		t.Errorf("a user-only finding must cap at flag: %+v, %v", got, err)
	}
	var me *MappingError
	if _, err := ToSteer(Finding{Pattern: pattern.ClaimVsRecord, DedupeKey: "x"}, 1, ""); !errors.As(err, &me) || me.Reason != "no fact" {
		t.Errorf("a finding without a fact: %v", err)
	}
	if _, err := ToSteer(Finding{Pattern: pattern.ID("purple"), Claim: &ClaimFact{}}, 1, ""); !errors.As(err, &me) {
		t.Errorf("an unknown pattern: %v", err)
	}
	if _, err := ToSteer(Finding{Pattern: pattern.ContinuityBriefing, Claim: &ClaimFact{}}, 1, ""); !errors.As(err, &me) || me.Reason != "no candidate priority" {
		t.Errorf("a pattern with no priority: %v", err)
	}
	if _, err := ToSteer(Finding{Pattern: pattern.CredSpray, Claim: &ClaimFact{}}, 1, ""); !errors.As(err, &me) || me.Reason != "no level for a VERIFY pattern" {
		t.Errorf("a pattern VERIFY does not raise: %v", err)
	}
}

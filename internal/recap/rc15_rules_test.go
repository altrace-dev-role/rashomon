package recap

import (
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// matched is a binding rule's verdict as StopView.Violations holds it: the
// rule id, its kind, and the turn's seqs.
func matched(id string, kind spec.RuleKind, seqs ...int64) verify.RuleVerdict {
	return verify.RuleVerdict{Rule: verify.SlotID(id), Kind: kind, Verdict: verify.Violation, Seqs: seqs}
}

// TestRC15d_TheStopNamesEachRuleTheTurnMatched (card RECALL-15): the value
// line counts the record's findings and names none (1.8 Gate B F1); under
// it, one line per rule the turn matched names the rule and the turn's
// calls, in the words the post note used at the call, before the pointer.
// A protect_path rule names the calls that wrote, and a require_evidence
// rule the Stop, whose seq is no call's. An id the rule id slot refuses
// reads "a spec rule"; past two rules one line counts the rest; past 80
// columns the calls go on a line of their own. Break: drop the lines from
// the value line, name every kind as a command's, print a refused id, lift
// the cap or the width rule, and this fails.
func TestRC15d_TheStopNamesEachRuleTheTurnMatched(t *testing.T) {
	oneFile := verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 5}
	const head = "1 file edited; no test or build is known for this project yet; "
	const long = "no-force-push-to-a-branch-the-lane-was-not-given" // 48, the slot's most
	for _, c := range []struct {
		name  string
		rules []verify.RuleVerdict
		want  string
	}{
		{"one call", []verify.RuleVerdict{matched("no-rm-glob", spec.ForbidCommand, 3)},
			indent + "spec rule no-rm-glob matched call 3\n"},
		{"two calls", []verify.RuleVerdict{matched("no-rm-glob", spec.ForbidCommand, 3, 7)},
			indent + "spec rule no-rm-glob matched calls 3 and 7\n"},
		{"four calls", []verify.RuleVerdict{matched("no-rm-glob", spec.ForbidCommand, 3, 7, 9, 12)},
			indent + "spec rule no-rm-glob matched calls 3, 7 and 2 more\n"},
		{"a protected path", []verify.RuleVerdict{matched("frozen-api", spec.ProtectPath, 4)},
			indent + "spec rule frozen-api protects a path call 4 wrote\n"},
		{"evidence at the Stop", []verify.RuleVerdict{matched("tests-at-done", spec.RequireEvidence, 11)},
			indent + "spec rule tests-at-done requires evidence that is not green at the Stop\n"},
		{"an id the slot refuses", []verify.RuleVerdict{matched("Bad Rule", spec.ForbidCommand, 3)},
			indent + "a spec rule matched call 3\n"},
		{"three rules", []verify.RuleVerdict{matched("no-fetch-all", spec.ForbidCommand, 2),
			matched("no-rm-glob", spec.ForbidCommand, 3), matched("frozen-api", spec.ProtectPath, 4)},
			indent + "spec rule no-fetch-all matched call 2\n" + indent + "spec rule no-rm-glob matched call 3\n" +
				indent + "and 1 more spec rule matched\n"},
		{"past 80 columns", []verify.RuleVerdict{matched(long, spec.ForbidCommand, 3, 7, 9)},
			indent + "spec rule " + long + ":\n" + indent + "  matched calls 3, 7 and 1 more\n"},
	} {
		v := viewOf(verify.RowNoGate, oneFile)
		v.Summary.Findings = len(c.rules)
		v.Violations = c.rules
		want := prefix + head + strconv.Itoa(len(c.rules)) + " record finding(s) in this turn.\n" + c.want +
			indent + "→ rashomon report --session sess-1"
		if got := ValueLine(v, Impact{}, "sess-1", false); got != want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}
}

// TestRC15e_TheDoneFlagAndTheRuleLineNameTheTurnsRules (card RECALL-15): the
// done flag carries the rule lines under its sentence, and where no other
// line speaks, a turn that matched a rule and wrote nothing (RowNone, B237),
// RuleLine says so and names them; a turn that matched none has no such line.
// Break: drop the lines from the done flag, or print RuleLine for none, and
// this fails.
func TestRC15e_TheDoneFlagAndTheRuleLineNameTheTurnsRules(t *testing.T) {
	const pointer = "→ rashomon report --session sess-1"
	fetch := matched("no-fetch-all", spec.ForbidCommand, 2)
	done := viewOf(verify.RowNone, verify.TurnCounts{})
	done.Scan = verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(verify.Absent)}}}
	done.Violations = []verify.RuleVerdict{fetch}
	flag, ok := DoneFlag(done, "sess-1", false)
	if want := prefix + doneLead + noRunSentence + ".\n" + indent + "spec rule no-fetch-all matched call 2\n" + indent + pointer; !ok || flag != want {
		t.Errorf("done flag:\n got %q (%v)\nwant %q", flag, ok, want)
	}

	none := viewOf(verify.RowNone, verify.TurnCounts{})
	if got := RuleLine(none, "sess-1", false); got != "" {
		t.Errorf("RuleLine for a turn that matched no rule = %q, want nothing", got)
	}
	none.Violations = []verify.RuleVerdict{fetch}
	if got, want := RuleLine(none, "sess-1", false),
		prefix+"a spec rule matched in this turn.\n"+indent+"spec rule no-fetch-all matched call 2\n"+indent+pointer; got != want {
		t.Errorf("RuleLine, one rule:\n got %q\nwant %q", got, want)
	}
	none.Violations = append(none.Violations, matched("no-rm-glob", spec.ForbidCommand, 3))
	if got, want := RuleLine(none, "sess-1", true), prefix+"2 spec rules matched in this turn.\n"+
		indent+"spec rule no-fetch-all matched call 2\n"+indent+"spec rule no-rm-glob matched call 3\n"+
		indent+"→ /rashomon:report --session sess-1"; got != want {
		t.Errorf("RuleLine, two rules, plugin:\n got %q\nwant %q", got, want)
	}
}

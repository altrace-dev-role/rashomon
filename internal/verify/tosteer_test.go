package verify

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

func seqp(n int64) *int64   { return &n }
func strp(s string) *string { return &s }

// at is the point a test finding was evaluated at: the last record seq the
// decision saw is 57 (the spec's example line, tech 4.2.3). A per-call
// finding is judged at no boundary point, so its kind is empty.
func at(kind string) Point { return Point{Kind: kind, Seq: 57, SessionID: "s1", PromptID: strp("p1")} }

// perCall is the point of a finding raised on one call (regression,
// platform-rule, ordering-rule, integrity).
var perCall = at("")

// mapCase is one finding, the point it was judged at, and the candidate the
// cards give it.
type mapCase struct {
	name     string
	f        Finding
	pt       Point
	kind     steer.Kind
	level    steer.Level
	form     steer.StopForm
	key      string
	evidence []int64
}

func ev(event steer.Event) string { return string(event) }

func mappingCases() []mapCase {
	red := &GateFact{GateID: "learned:go-test", Program: "go", State: Red, Reason: FailedExit, LastRunSeq: seqp(52), LastGreenSeq: seqp(37)}
	claim := func(v State) *ClaimFact {
		return &ClaimFact{Class: ClaimTestsPass, Source: SourceFinalMessage, Strict: true, Verdict: Verdict(v)}
	}
	stop := ev(steer.EventStop)
	post := ev(steer.EventPostToolUse)
	return []mapCase{
		{"claim red", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Red), Gate: red}, at(PointStop), "red", steer.LevelStop, steer.StopFormStopContinue, "tests_pass", []int64{37, 52}},
		{"claim masked", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(MaskedInconclusive)}, at(PointStop), "masked", steer.LevelFlag, steer.StopFormNone, "tests_pass", []int64{57}},
		{"claim stale", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Stale)}, at(PointStop), "stale", steer.LevelNudge, steer.StopFormNone, "tests_pass", []int64{57}},
		{"claim absent", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Absent)}, at(PointStop), "absent", steer.LevelNudge, steer.StopFormNone, "tests_pass", []int64{57}},
		{"claim backgrounded", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Backgrounded)}, at(PointStop), "backgrounded", steer.LevelStop, steer.StopFormStopContinue, "tests_pass", []int64{57}},
		{"done over absent", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: &ClaimFact{Class: ClaimDone, Source: SourceFinalMessage, Strict: true, Verdict: Verdict(Absent)}}, at(PointStop), "done-unverified", steer.LevelNudge, steer.StopFormNone, "done", []int64{57}},
		{"done over stale", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: &ClaimFact{Class: ClaimDone, Source: SourceFinalMessage, Strict: true, Verdict: Verdict(Stale)}}, at(PointStop), "done-unverified", steer.LevelNudge, steer.StopFormNone, "done", []int64{57}},
		{"silent failure", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: &ClaimFact{Class: ClaimTestsPass, Source: SourceFinalMessage, Strict: true, Verdict: Verdict(Green), UnrecoveredFailures: 2}}, at(PointStop), "silent-failure", steer.LevelStop, steer.StopFormStopContinue, "tests_pass", []int64{57}},
		// A masked verdict stays in the user's Stop line even beside
		// unrecovered failures: it never reaches the model as silent-failure.
		{"masked with failures", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: &ClaimFact{Class: ClaimTestsPass, Source: SourceFinalMessage, Strict: true, Verdict: Verdict(MaskedInconclusive), UnrecoveredFailures: 2}}, at(PointStop), "masked", steer.LevelFlag, steer.StopFormNone, "tests_pass", []int64{57}},
		{"loose claim", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: &ClaimFact{Class: ClaimTestsPass, Source: SourceFinalMessage, Verdict: Verdict(Red)}}, at(PointStop), "red", steer.LevelFlag, steer.StopFormNone, "tests_pass", []int64{57}},
		{"regression", Finding{Pattern: pattern.RegressionLocation, Event: ev(steer.EventPostToolUseFailure), Gate: red}, perCall, "", steer.LevelNudge, steer.StopFormNone, "learned:go-test", []int64{37, 52}},
		{"subtask nudge", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: &GateFact{GateID: "learned:go-test", State: Stale, LastRunSeq: seqp(37), LastGreenSeq: seqp(37)}}, Point{Kind: PointSubtask, Seq: 57, TaskRef: strp("7")}, "", steer.LevelNudge, steer.StopFormNone, "task:7", []int64{37}},
		{"subtask deny", Finding{Pattern: pattern.SubtaskGate, Event: ev(steer.EventPreToolUse), Gate: &GateFact{GateID: "learned:go-test", State: Absent}}, Point{Kind: PointSubtask, Seq: 57, TaskRef: strp("7")}, "", steer.LevelStop, steer.StopFormDeny, "task:7", nil},
		{"todo nudge", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: &GateFact{GateID: "learned:go-test", State: Absent}}, at(PointSubtask), "", steer.LevelNudge, steer.StopFormNone, "todo", nil},
		{"teammate", Finding{Pattern: pattern.SubtaskGate, Event: "TaskCompleted", Gate: &GateFact{GateID: "learned:go-test", State: Absent}}, Point{Kind: PointSubtask, Seq: 57, TaskRef: strp("t-2")}, "", steer.LevelFlag, steer.StopFormNone, "task:t-2", nil},
		{"commit", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: &GateFact{GateID: "learned:go-test", State: Stale, LastRunSeq: seqp(37)}}, at(PointCommit), "commit", steer.LevelNudge, steer.StopFormNone, "learned:go-test", []int64{37}},
		{"edit", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: &RuleFact{RuleID: "no-shell-true", Kind: spec.ForbidCode, Trust: spec.Trusted, Seq: 44, Added: 1, Path: pathp("tools/x.py")}}, perCall, "edit", steer.LevelNudge, steer.StopFormNone, "no-shell-true|tools/x.py", []int64{44}},
		{"pragma", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: &RuleFact{RuleID: "no-nosec", Kind: spec.ForbidCode, Trust: spec.Managed, Seq: 44, Added: 1, Pragma: true, Path: pathp("a.py")}}, perCall, "pragma", steer.LevelNudge, steer.StopFormNone, "no-nosec|a.py", []int64{44}},
		{"command", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: &RuleFact{RuleID: "no-verify", Kind: spec.ForbidCommand, Trust: spec.Trusted, Seq: 50}}, perCall, "command", steer.LevelNudge, steer.StopFormNone, "no-verify", []int64{50}},
		{"untrusted rule", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: &RuleFact{RuleID: "no-verify", Kind: spec.ForbidCommand, Trust: spec.Untrusted, Seq: 50}}, perCall, "command", steer.LevelFlag, steer.StopFormNone, "no-verify", []int64{50}},
		// Another hook rewrote the input: a flag whatever the trust (40.4.6
		// "flag when another hook rewrote the input"; 40.4.10 "capped at flag").
		{"rewritten command", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: &RuleFact{RuleID: "no-verify", Kind: spec.ForbidCommand, Trust: spec.Managed, Seq: 50, Rewritten: true}}, perCall, "command", steer.LevelFlag, steer.StopFormNone, "no-verify", []int64{50}},
		{"rewritten ordering ask", Finding{Pattern: pattern.OrderingRule, Event: ev(steer.EventPreToolUse), Rule: &RuleFact{RuleID: "export-after-verify", Kind: spec.Order, Trust: spec.Trusted, Seq: 50, Rewritten: true}}, perCall, "", steer.LevelFlag, steer.StopFormNone, "export-after-verify", []int64{50}},
		{"ordering note", Finding{Pattern: pattern.OrderingRule, Event: post, Rule: &RuleFact{RuleID: "export-after-verify", Kind: spec.Order, Trust: spec.Managed, Seq: 50}}, perCall, "", steer.LevelNudge, steer.StopFormNone, "export-after-verify", []int64{50}},
		{"ordering ask", Finding{Pattern: pattern.OrderingRule, Event: ev(steer.EventPreToolUse), Rule: &RuleFact{RuleID: "export-after-verify", Kind: spec.Order, Trust: spec.Trusted, Seq: 50}}, perCall, "", steer.LevelStop, steer.StopFormAsk, "export-after-verify", []int64{50}},
		{"integrity", Finding{Pattern: pattern.Integrity, Event: post, Integrity: &IntegrityFact{Kind: SkipAdded, Count: 1, Seq: 40, Marker: "go.t_skip"}}, perCall, "skip-added", steer.LevelFlag, steer.StopFormNone, "40", []int64{40}},
		{"frozen path", Finding{Pattern: pattern.Integrity, Event: post, Rule: &RuleFact{RuleID: "keep-ci", Kind: spec.ProtectPath, Trust: spec.Trusted, Seq: 41, Path: pathp(".github/workflows/ci.yaml")}}, perCall, "frozen-path", steer.LevelFlag, steer.StopFormNone, "keep-ci|.github/workflows/ci.yaml", []int64{41}},
		{"subagent", Finding{Pattern: pattern.SubagentClaim, Event: "SubagentStop", Claim: &ClaimFact{Class: ClaimDone, Source: SourceSubagentStop, Strict: true, Verdict: Verdict(Absent)}}, Point{Kind: PointSubagentStop, Seq: 57, AgentID: strp("a1")}, "", steer.LevelFlag, steer.StopFormNone, "a1|done", []int64{57}},
	}
}

func pathp(s string) *PathSlot { p := PathSlot(s); return &p }

// TestVER54e_ToSteerMapsEveryVerifyPattern: every pattern VERIFY raises maps
// to a candidate with the pattern's priority, the level and stop form its
// card gives at the finding's event, a frozen kind, the card's dedupe key,
// the point seq as AfterSeq, and the fact seqs ascending as Evidence; no
// message is rendered by VERIFY, and every candidate passes steer's own
// Check.
func TestVER54e_ToSteerMapsEveryVerifyPattern(t *testing.T) {
	for _, c := range mappingCases() {
		got, err := ToSteer(c.f, c.pt, "")
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		wantPriority, _ := pattern.Priority(c.f.Pattern)
		if got.Pattern != c.f.Pattern || got.Priority != wantPriority || got.AgentID != "" {
			t.Errorf("%s: identity lost: %+v", c.name, got)
		}
		if got.Kind != c.kind || got.MaxLevel != c.level || got.StopForm != c.form || got.Key != c.key {
			t.Errorf("%s: kind %q level %v form %q key %q; want %q %v %q %q", c.name, got.Kind, got.MaxLevel, got.StopForm, got.Key, c.kind, c.level, c.form, c.key)
		}
		if got.AfterSeq != c.pt.Seq || !slices.Equal(got.Evidence, c.evidence) {
			t.Errorf("%s: after %d evidence %v; want %d %v", c.name, got.AfterSeq, got.Evidence, c.pt.Seq, c.evidence)
		}
		if !reflect.ValueOf(got.Message).IsZero() {
			t.Errorf("%s: a message was rendered by VERIFY", c.name)
		}
		if err := got.Check(); err != nil {
			t.Errorf("%s: the candidate fails steer's check: %v", c.name, err)
		}
	}
}

// TestVER54f_ToSteerEmitsOnlyFrozenKinds: every value VERIFY's vocabularies
// can put on a fact crosses as a frozen kind (tech 2.2.6's integrity order;
// forbid_code and forbid_command as edit and command; masked_inconclusive as
// masked), and nothing outside pattern.Kinds reaches STEER.
func TestVER54f_ToSteerEmitsOnlyFrozenKinds(t *testing.T) {
	want := []steer.Kind{"test-deleted", "skip-added", "assert-drop", "suppression-added", "generated-edited", "tests-only-green", "gate-weakened", "gate-config-edited"}
	for i, k := range IntegrityKinds {
		f := Finding{Pattern: pattern.Integrity, Event: string(steer.EventPostToolUse), Integrity: &IntegrityFact{Kind: k, Count: 1, Seq: 40}}
		got, err := ToSteer(f, at(PointStop), "")
		if err != nil || got.Kind != want[i] {
			t.Errorf("integrity %s -> %q, %v; want %q", k, got.Kind, err, want[i])
		}
	}
	for _, c := range mappingCases() {
		got, err := ToSteer(c.f, c.pt, "")
		if err == nil && !pattern.KindValid(got.Pattern, string(got.Kind)) {
			t.Errorf("%s: %s/%q is not a frozen kind", c.name, got.Pattern, got.Kind)
		}
	}
}

// TestVER54g_ToSteerRefusesWhatCannotBeValid: a finding outside the cards
// is an error, never a candidate: a pattern VERIFY does not raise, a fact
// set that does not fit the pattern or more than one, a claim with no
// contradiction (green, unknown, no_gate_known), a rule of another
// pattern's kind, an event the card does not raise it at, a query point, an
// evidence seq after the point, and an id outside its charset.
func TestVER54g_ToSteerRefusesWhatCannotBeValid(t *testing.T) {
	stop, post := string(steer.EventStop), string(steer.EventPostToolUse)
	gate := &GateFact{GateID: "learned:go-test", State: Red, LastRunSeq: seqp(52)}
	claim := func(v Verdict) *ClaimFact {
		return &ClaimFact{Class: ClaimTestsPass, Source: SourceFinalMessage, Strict: true, Verdict: v}
	}
	subClaim := func(v Verdict) *ClaimFact { c := claim(v); c.Source = SourceSubagentStop; return c }
	// A code or path rule names its file, so each case below fails for the
	// reason it names, not for a missing path (ruleShapeFits).
	rule := func(k spec.RuleKind) *RuleFact {
		r := &RuleFact{RuleID: "r1", Kind: k, Trust: spec.Trusted, Seq: 50}
		if k == spec.ForbidCode || k == spec.ProtectPath {
			r.Path = pathp("a.py")
		}
		return r
	}
	for _, c := range []struct {
		name string
		f    Finding
		pt   Point
	}{
		{"unknown pattern", Finding{Pattern: "purple", Event: stop, Claim: claim(Verdict(Red))}, at(PointStop)},
		{"no priority", Finding{Pattern: pattern.ContinuityBriefing, Event: stop, Claim: claim(Verdict(Red))}, at(PointStop)},
		{"not raised by verify", Finding{Pattern: pattern.CredSpray, Event: post, Claim: claim(Verdict(Red))}, at(PointStop)},
		{"no fact", Finding{Pattern: pattern.ClaimVsRecord, Event: stop}, at(PointStop)},
		{"claim pattern with a gate only", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Gate: gate}, at(PointStop)},
		{"two facts", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate, Rule: rule(spec.ForbidCode)}, at(PointStop)},
		{"claim and rule", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Verdict(Red)), Rule: rule(spec.ForbidCode)}, at(PointStop)},
		{"green claim", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Verdict(Green))}, at(PointStop)},
		{"unknown claim", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Verdict(Unknown))}, at(PointStop)},
		{"no gate known", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(NoGateKnown)}, at(PointStop)},
		{"green subagent", Finding{Pattern: pattern.SubagentClaim, Event: "SubagentStop", Claim: subClaim(Verdict(Green))}, Point{Kind: PointSubagentStop, Seq: 57, AgentID: strp("a1")}},
		{"subagent without its id", Finding{Pattern: pattern.SubagentClaim, Event: "SubagentStop", Claim: subClaim(Verdict(Red))}, at(PointSubagentStop)},
		{"require_evidence as a platform rule", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.RequireEvidence)}, at(PointStop)},
		{"order as a platform rule", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.Order)}, at(PointStop)},
		{"forbid_code as an ordering rule", Finding{Pattern: pattern.OrderingRule, Event: post, Rule: rule(spec.ForbidCode)}, at(PointStop)},
		{"forbid_code as integrity", Finding{Pattern: pattern.Integrity, Event: post, Rule: rule(spec.ForbidCode)}, at(PointStop)},
		{"pragma on a command rule", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: &RuleFact{RuleID: "r1", Kind: spec.ForbidCommand, Trust: spec.Trusted, Seq: 50, Pragma: true}}, at(PointStop)},
		{"claim at PostToolUse", Finding{Pattern: pattern.ClaimVsRecord, Event: post, Claim: claim(Verdict(Red))}, at(PointStop)},
		{"claim at a commit point", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Verdict(Red))}, at(PointCommit)},
		{"platform rule at PreToolUse", Finding{Pattern: pattern.PlatformRule, Event: string(steer.EventPreToolUse), Rule: rule(spec.ForbidCode)}, at(PointStop)},
		{"commit kind at PreToolUse", Finding{Pattern: pattern.SubtaskGate, Event: string(steer.EventPreToolUse), Gate: gate}, at(PointCommit)},
		{"subtask-gate at a stop point", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: gate}, at(PointStop)},
		{"no event", Finding{Pattern: pattern.RegressionLocation, Gate: gate}, at(PointStop)},
		{"query point", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate}, at(PointQuery)},
		{"no point seq", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate}, Point{Kind: PointStop}},
		{"evidence after the point", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: &GateFact{GateID: "g", State: Red, LastRunSeq: seqp(58)}}, at(PointStop)},
		{"evidence seq zero", Finding{Pattern: pattern.Integrity, Event: post, Integrity: &IntegrityFact{Kind: SkipAdded, Count: 1}}, at(PointStop)},
		{"gate id outside its charset", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: &GateFact{GateID: "Go Test", State: Red}}, at(PointStop)},
		{"path outside its charset", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: &RuleFact{RuleID: "r1", Kind: spec.ForbidCode, Trust: spec.Trusted, Seq: 50, Path: pathp("../x")}}, at(PointStop)},
		{"task ref outside its charset", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: gate}, Point{Kind: PointSubtask, Seq: 57, TaskRef: strp("a b")}},
		// Only the TaskUpdate form has a deny (40.4.1: "Stop: PreToolUse(TaskUpdate)
		// permissionDecision: deny"); a TodoWrite completion names no task.
		{"todo completion denied at PreToolUse", Finding{Pattern: pattern.SubtaskGate, Event: string(steer.EventPreToolUse), Gate: gate}, at(PointSubtask)},
		{"teammate close naming no task", Finding{Pattern: pattern.SubtaskGate, Event: "TaskCompleted", Gate: gate}, at(PointSubtask)},
	} {
		var me *MappingError
		if got, err := ToSteer(c.f, c.pt, ""); !errors.As(err, &me) {
			t.Errorf("%s: (%+v, %v), want a MappingError", c.name, got, err)
		}
	}
	var me *MappingError
	if _, err := ToSteer(Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(Verdict(Red))}, at(PointStop), "not an id"); !errors.As(err, &me) {
		t.Errorf("an agent id outside its charset: %v", err)
	}
}

// TestVER54h_AfterSeqIsThePointNotTheFact: AfterSeq is the last record seq
// the decision saw, so a green that arrives later with a seq between the
// last run and the point is below after_seq and refutes the firing as
// recorded late (archive 40.4.1); the fact's own seqs go to Evidence only.
func TestVER54h_AfterSeqIsThePointNotTheFact(t *testing.T) {
	f := Finding{Pattern: pattern.SubtaskGate, Event: string(steer.EventPostToolUse), Gate: &GateFact{GateID: "learned:go-test", State: Stale, LastRunSeq: seqp(37), LastGreenSeq: seqp(37)}}
	got, err := ToSteer(f, Point{Kind: PointSubtask, Seq: 57, TaskRef: strp("7")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.AfterSeq != 57 || !slices.Equal(got.Evidence, []int64{37}) {
		t.Errorf("after %d evidence %v, want 57 [37]", got.AfterSeq, got.Evidence)
	}
	if late := int64(45); !(late < got.AfterSeq) {
		t.Errorf("a green at %d is not below after_seq %d, so it can never refute as recorded late", late, got.AfterSeq)
	}
}

// TestVER54i_KeyIsTheCardsUnit: two tasks closed against one gate in one
// prompt are two keys, so the second is neither deduped nor sharing the
// first one's two denials (archive 40.4.1: "At most 2 denials per task per
// prompt"); one rule on two files is two keys ("Once per rule per file per
// prompt", 40.4.6); VERIFY's DedupeKey is not the key.
func TestVER54i_KeyIsTheCardsUnit(t *testing.T) {
	gate := &GateFact{GateID: "learned:go-test", State: Absent}
	f := Finding{Pattern: pattern.SubtaskGate, Event: string(steer.EventPreToolUse), DedupeKey: "subtask-gate|learned:go-test|p1", Gate: gate}
	a, errA := ToSteer(f, Point{Kind: PointSubtask, Seq: 57, TaskRef: strp("1")}, "")
	b, errB := ToSteer(f, Point{Kind: PointSubtask, Seq: 60, TaskRef: strp("2")}, "")
	if errA != nil || errB != nil || a.Key == b.Key || a.Key == f.DedupeKey {
		t.Errorf("two tasks against one gate: keys %q and %q (%v, %v)", a.Key, b.Key, errA, errB)
	}
	rule := func(p string) Finding {
		return Finding{Pattern: pattern.PlatformRule, Event: string(steer.EventPostToolUse), DedupeKey: "platform-rule|r1|p1", Rule: &RuleFact{RuleID: "r1", Kind: spec.ForbidCode, Trust: spec.Trusted, Seq: 50, Path: pathp(p)}}
	}
	x, errX := ToSteer(rule("a.py"), at(PointStop), "")
	y, errY := ToSteer(rule("b.py"), at(PointStop), "")
	if errX != nil || errY != nil || x.Key == y.Key {
		t.Errorf("one rule on two files: keys %q and %q (%v, %v)", x.Key, y.Key, errX, errY)
	}
}

// TestVER54k_ToSteerChecksEveryFactID: every id, path, program and digest a
// fact carries is checked against its charset before a candidate exists,
// not only the ones a key is built from, and a rule fact carries only the
// fields its kind has (RuleFact's comment: a path for forbid_code and
// protect_path, never for forbid_command or order), so a command rule's key
// is the rule alone (P07N-13).
func TestVER54k_ToSteerChecksEveryFactID(t *testing.T) {
	stop, post := string(steer.EventStop), string(steer.EventPostToolUse)
	gate := func(mod func(*GateFact)) *GateFact {
		g := &GateFact{GateID: "learned:go-test", Program: "go", State: Red, LastGreenSeq: seqp(37), LastRunSeq: seqp(52)}
		mod(g)
		return g
	}
	claim := func(mod func(*ClaimFact)) *ClaimFact {
		c := &ClaimFact{Class: ClaimTestsPass, Source: SourceFinalMessage, Strict: true, Verdict: Verdict(Red), Gates: []SlotID{"learned:go-test"}}
		mod(c)
		return c
	}
	rule := func(k spec.RuleKind, mod func(*RuleFact)) *RuleFact {
		r := &RuleFact{RuleID: "r1", Kind: k, Trust: spec.Trusted, Seq: 50}
		mod(r)
		return r
	}
	prog, badProg, badGate := Program("git"), Program("rm -rf /"), SlotID("Bad Gate")
	six := []PathSlot{"a", "b", "c", "d", "e", "f"}
	for _, c := range []struct {
		name string
		f    Finding
	}{
		{"gate path outside its charset", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(func(g *GateFact) { g.Paths = []PathSlot{"../etc/passwd"} })}},
		{"gate program outside its charset", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(func(g *GateFact) { g.Program = "Go Test" })}},
		{"more gate paths than MaxPaths", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(func(g *GateFact) { g.Paths = six })}},
		{"a negative omitted count", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(func(g *GateFact) { g.PathsOmitted = -1 })}},
		{"claim gate outside its charset", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.Gates = []SlotID{"Bad Gate"} })}},
		{"claim quote digest not hex", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.QuoteHMAC = "the claim sentence" })}},
		{"integrity path outside its charset", Finding{Pattern: pattern.Integrity, Event: post, Integrity: &IntegrityFact{Kind: SkipAdded, Count: 1, Seq: 40, Paths: []PathSlot{"../x"}}}},
		{"rule program outside its charset", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.ForbidCommand, func(r *RuleFact) { r.Program = &badProg })}},
		{"order gate outside its charset", Finding{Pattern: pattern.OrderingRule, Event: post, Rule: rule(spec.Order, func(r *RuleFact) { r.Gate = &badGate })}},
		{"a command rule with a path", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.ForbidCommand, func(r *RuleFact) { r.Path = pathp("a.py") })}},
		{"a code rule with no path", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.ForbidCode, func(*RuleFact) {})}},
		{"a code rule with a program", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.ForbidCode, func(r *RuleFact) { r.Path = pathp("a.py"); r.Program = &prog })}},
		{"a frozen path with no path", Finding{Pattern: pattern.Integrity, Event: post, Rule: rule(spec.ProtectPath, func(*RuleFact) {})}},
		{"an order rule with a path", Finding{Pattern: pattern.OrderingRule, Event: post, Rule: rule(spec.Order, func(r *RuleFact) { r.Path = pathp("a.py") })}},
	} {
		pt := perCall
		if c.f.Pattern == pattern.ClaimVsRecord {
			pt = at(PointStop)
		}
		var me *MappingError
		if got, err := ToSteer(c.f, pt, ""); !errors.As(err, &me) {
			t.Errorf("%s: (%+v, %v), want a MappingError", c.name, got, err)
		}
	}
	// The valid neighbours still map: every fact id in its charset.
	for _, f := range []Finding{
		{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(func(g *GateFact) { g.Paths = six[:MaxPaths]; g.PathsOmitted = 1 })},
		{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.QuoteHMAC = QuoteHMAC(QuoteHMACPrefix + strings.Repeat("ab", 32)) })},
		{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.ForbidCommand, func(r *RuleFact) { r.Program = &prog })},
		{Pattern: pattern.OrderingRule, Event: post, Rule: rule(spec.Order, func(r *RuleFact) { r.Program = &prog; r.Gate = pslot("learned:go-test") })},
	} {
		pt := perCall
		if f.Pattern == pattern.ClaimVsRecord {
			pt = at(PointStop)
		}
		got, err := ToSteer(f, pt, "")
		if err != nil {
			t.Errorf("%s: %v", f.Pattern, err)
		}
		if f.Pattern == pattern.PlatformRule && got.Key != "r1" {
			t.Errorf("a command rule keys %q, want the rule alone", got.Key)
		}
	}
}

func pslot(s string) *SlotID { id := SlotID(s); return &id }

// TestVER54l_ToSteerRefusesValuesOutsideTheirVocabularies: the closed
// vocabularies that decide a candidate's visibility, kind and key are
// checked like the ids (P07N-13): a rule's trust outside spec.Trusts (an
// unknown trust read as trusted would reach the model; 2.2.7 "untrusted
// (default): user flags only"), a claim class outside ClaimClasses (the
// class is the key), a claim source outside the three 2.2.5 reads or
// another pattern's, a verdict outside the states, a gate reason outside
// Reasons, and a gate state that contradicts nothing for its pattern: a
// subtask-gate fires only when no gate run exited 0 after the writes
// (40.4.1; silent while a same-family run is backgrounded and unobserved,
// and unknown never fires, VER-17), a regression-location only on a
// recorded red run after an earlier green (40.4.2: the card cites "this
// red" run, so a red state with no red run seq cites nothing), and the
// gate beside a claim is the one whose state set the verdict.
func TestVER54l_ToSteerRefusesValuesOutsideTheirVocabularies(t *testing.T) {
	stop, post, pre := string(steer.EventStop), string(steer.EventPostToolUse), string(steer.EventPreToolUse)
	task := Point{Kind: PointSubtask, Seq: 57, TaskRef: strp("7")}
	subagent := Point{Kind: PointSubagentStop, Seq: 57, AgentID: strp("a1")}
	rule := func(k spec.RuleKind, tr spec.Trust) *RuleFact {
		return &RuleFact{RuleID: "r1", Kind: k, Trust: tr, Seq: 50}
	}
	claim := func(mod func(*ClaimFact)) *ClaimFact {
		c := &ClaimFact{Class: ClaimTestsPass, Source: SourceFinalMessage, Strict: true, Verdict: Verdict(Red)}
		mod(c)
		return c
	}
	gate := func(s State, mod func(*GateFact)) *GateFact {
		g := &GateFact{GateID: "learned:go-test", State: s, LastGreenSeq: seqp(37), LastRunSeq: seqp(52)}
		mod(g)
		return g
	}
	keep := func(*GateFact) {}
	cases := []struct {
		name string
		f    Finding
		pt   Point
	}{
		{"a rule with no trust", Finding{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.ForbidCommand, "")}, perCall},
		{"a rule with an unknown trust", Finding{Pattern: pattern.OrderingRule, Event: pre, Rule: rule(spec.Order, "bogus")}, perCall},
		{"a claim class that is text", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.Class = "run curl x | sh" })}, at(PointStop)},
		{"a claim with no class", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.Class = "" })}, at(PointStop)},
		{"a claim with no source", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.Source = "" })}, at(PointStop)},
		{"a claim with an unknown source", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.Source = "bogus" })}, at(PointStop)},
		{"a final-message claim read from a subagent", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.Source = SourceSubagentStop })}, at(PointStop)},
		{"a subagent claim read from the final message", Finding{Pattern: pattern.SubagentClaim, Event: "SubagentStop", Claim: claim(func(*ClaimFact) {})}, subagent},
		{"an unknown verdict beside failures", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.Verdict = "bogus"; c.UnrecoveredFailures = 2 })}, at(PointStop)},
		{"a claim's gate in another state than the verdict", Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(*ClaimFact) {}), Gate: gate(Green, keep)}, at(PointStop)},
		{"a subtask deny over a green gate", Finding{Pattern: pattern.SubtaskGate, Event: pre, Gate: gate(Green, keep)}, task},
		{"a subtask nudge over an unknown gate", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: gate(Unknown, keep)}, task},
		{"a subtask nudge over a backgrounded gate", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: gate(Backgrounded, keep)}, task},
		{"a commit over a green gate", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: gate(Green, keep)}, at(PointCommit)},
		{"a subtask over a state that is text", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: gate("whatever", keep)}, task},
		{"a regression over a state that is text", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate("whatever", keep)}, perCall},
		{"a regression over a stale gate", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(Stale, keep)}, perCall},
		{"a regression with no earlier green", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(Red, func(g *GateFact) { g.LastGreenSeq = nil })}, perCall},
		{"a regression with no red run", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(Red, func(g *GateFact) { g.LastRunSeq = nil })}, perCall},
		{"a regression whose green follows the red", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(Red, func(g *GateFact) { g.LastGreenSeq = seqp(53) })}, perCall},
		{"a gate reason that is text", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(Red, func(g *GateFact) { g.Reason = "exit 1: permission denied" })}, perCall},
		{"a red regression explained as ok", Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(Red, func(g *GateFact) { g.Reason = OK })}, perCall},
		{"a red subtask gate with a stale reason", Finding{Pattern: pattern.SubtaskGate, Event: post, Gate: gate(Red, func(g *GateFact) { g.Reason = WritesAfterGreen })}, task},
	}
	for _, c := range cases {
		var me *MappingError
		if got, err := ToSteer(c.f, c.pt, ""); !errors.As(err, &me) {
			t.Errorf("%s: (%+v, %v), want a MappingError", c.name, got, err)
		}
	}
	// The valid neighbours map: every trust, class and source in its
	// vocabulary, and every state each gate pattern fires on.
	var valid []Finding
	for _, tr := range spec.Trusts {
		valid = append(valid, Finding{Pattern: pattern.PlatformRule, Event: post, Rule: rule(spec.ForbidCommand, tr)})
	}
	for _, cl := range ClaimClasses {
		valid = append(valid, Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(c *ClaimFact) { c.Class = cl })})
	}
	for _, s := range []State{Red, MaskedInconclusive, Stale, Absent} {
		valid = append(valid, Finding{Pattern: pattern.SubtaskGate, Event: pre, Gate: gate(s, keep)})
	}
	valid = append(valid,
		Finding{Pattern: pattern.ClaimVsRecord, Event: stop, Claim: claim(func(*ClaimFact) {}), Gate: gate(Red, func(g *GateFact) { g.Reason = FailedExit })},
		Finding{Pattern: pattern.RegressionLocation, Event: post, Gate: gate(Red, func(g *GateFact) { g.Reason = FailedOutput })},
	)
	for _, f := range valid {
		pt := perCall
		switch f.Pattern {
		case pattern.ClaimVsRecord:
			pt = at(PointStop)
		case pattern.SubtaskGate:
			pt = task
		}
		if _, err := ToSteer(f, pt, ""); err != nil {
			t.Errorf("%s: %v", f.Pattern, err)
		}
	}
	for _, src := range []ClaimSource{SourceSubagentStop, SourceHandback} {
		f := Finding{Pattern: pattern.SubagentClaim, Event: "SubagentStop", Claim: claim(func(c *ClaimFact) { c.Source = src })}
		if _, err := ToSteer(f, subagent, ""); err != nil {
			t.Errorf("a subagent claim from %s: %v", src, err)
		}
	}
}

// TestVER54m_EveryGateReasonExplainsOneState: a gate's reason explains its
// state (2.2.4; archive 20.2.4 algorithm, which names the one state each
// closed reason comes with), so a fact whose reason explains another state
// contradicts itself and is refused like a value outside its vocabulary
// (P07N-13). Every reason in Reasons has exactly one state; each is
// accepted beside that state and refused beside every other, and a reason
// outside the list is refused as such, not as a state mismatch.
func TestVER54m_EveryGateReasonExplainsOneState(t *testing.T) {
	// Frozen from archive 20.2.4 steps 1 to 6 and tech 2.2.4, not read
	// from reasonState, so a wrong entry there cannot move the expectation.
	frozen := map[Reason]State{
		FailedExit: Red, FailedOutput: Red, PartialAfterRed: Red,
		MaskedWithSuccessLine: Green, OK: Green,
		MaskedNoSuccessLine: MaskedInconclusive, MaskedUnattributable: MaskedInconclusive,
		EmptyRun: Absent, PartialOnly: Absent, NeverRunAfterWrite: Absent,
		RedThenWrites: Stale, WritesAfterGreen: Stale, WriteDuringRun: Stale,
		BackgroundLaunch: Backgrounded,
		OrderUnknown:     Unknown, OutcomeUnknown: Unknown, RecordingIncomplete: Unknown, Budget: Unknown,
	}
	if len(frozen) != len(Reasons) || len(reasonState()) != len(Reasons) {
		t.Fatalf("frozen %d, reasonState %d, Reasons %d: a reason has no state", len(frozen), len(reasonState()), len(Reasons))
	}
	beside := func(r Reason, s State) Finding {
		return Finding{
			Pattern: pattern.ClaimVsRecord, Event: string(steer.EventStop),
			Claim: &ClaimFact{Class: ClaimTestsPass, Source: SourceFinalMessage, Strict: true, Verdict: Verdict(s)},
			Gate:  &GateFact{GateID: "learned:go-test", State: s, Reason: r, LastGreenSeq: seqp(37), LastRunSeq: seqp(52)},
		}
	}
	for _, r := range Reasons {
		want, ok := frozen[r]
		if !ok {
			t.Errorf("reason %q has no frozen state", r)
			continue
		}
		for _, s := range States {
			got := vocabValid(beside(r, s))
			switch {
			case s == want && got != "":
				t.Errorf("reason %q beside its own state %q refused: %s", r, s, got)
			case s != want && got != "a gate reason that explains another state":
				t.Errorf("reason %q beside %q: %q, want the mismatch refused", r, s, got)
			}
		}
	}
	if got := vocabValid(beside("exit 1: permission denied", Red)); got != "gate reason outside its vocabulary" {
		t.Errorf("a reason that is text: %q, want it refused as outside the vocabulary", got)
	}
	if got := vocabValid(beside("", Red)); got != "" {
		t.Errorf("a gate with no reason: %q, want it accepted", got)
	}
}

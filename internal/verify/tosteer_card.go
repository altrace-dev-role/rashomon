package verify

import (
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// The hook events a VERIFY finding is raised at that STEER never prints
// on: a teammate's TaskCompleted (its flag queues for the recap, archive
// 40.4.1) and SubagentStop (subagent-claim's trigger, archive 40.4.12).
const (
	eventTaskCompleted = "TaskCompleted"
	eventSubagentStop  = "SubagentStop"
)

// ceiling is one card row: the highest level the pattern can use at the
// event, and the shape that level takes at stop.
type ceiling struct {
	level steer.Level
	form  steer.StopForm
}

var (
	nudge = ceiling{steer.LevelNudge, steer.StopFormNone}
	flag  = ceiling{steer.LevelFlag, steer.StopFormNone}
)

// ceilingFor is the per-card ceiling at the finding's event (P-07 proposal;
// the gap row's "`StopForm` per card"):
//
//   - claim-vs-record, raised at Stop: a flag there and a note on the next
//     prompt, and at a project `stop` level one continuation with the same
//     text (archive 40.4.14; tech 4.2.13), so stop with stop_continue; the
//     masked kind stays in the user's Stop line (ED-3), so flag.
//   - subtask-gate: a candidate nudge at PostToolUse of the completion or
//     boundary call; deny on PreToolUse of the TaskUpdate (40.4.1: "Stop:
//     PreToolUse(TaskUpdate) `permissionDecision: deny`"), so only a
//     completion that names its task, never a TodoWrite one (TaskRef nil,
//     the nudge form only); a teammate's TaskCompleted, which always names
//     its task ("teammate task {task_id}"), is a flag.
//   - regression-location: nudge at PostToolUseFailure, or PostToolUse for
//     a masked run with a failure line (40.4.2); no stop form.
//   - platform-rule: nudge at PostToolUse, no stop form (40.4.6;
//     4.2.12: "`stop` with no stop form is `nudge`").
//   - ordering-rule: nudge after the call; ask on PreToolUse of the Bash
//     call (40.4.10; STR-68).
//   - integrity and subagent-claim: user flags at release (2.2.9's table).
//
// A loose claim, an untrusted rule and a finding VERIFY marks UserOnly are
// user flags whatever the card allows (2.2.5 step 8; 2.2.9). ok is false
// for an event the card does not raise the finding at.
func ceilingFor(f Finding, kind steer.Kind, at Point) (steer.Level, steer.StopForm, bool) {
	c, ok := cardCeiling(f.Pattern, kind, f.Event, at.TaskRef != nil)
	if !ok {
		return 0, "", false
	}
	loose := f.Claim != nil && !f.Claim.Strict
	untrusted := f.Rule != nil && f.Rule.Trust == spec.Untrusted
	if f.UserOnly || loose || untrusted {
		c = flag
	}
	return c.level, c.form, true
}

// cardCeiling is the card row for (pattern, kind, event); task is true when
// the point names the task a subtask completion closed.
func cardCeiling(id pattern.ID, kind steer.Kind, event string, task bool) (ceiling, bool) {
	pre, post := string(steer.EventPreToolUse), string(steer.EventPostToolUse)
	switch {
	case id == pattern.ClaimVsRecord && event == string(steer.EventStop):
		if kind == "masked" {
			return flag, true
		}
		return ceiling{steer.LevelStop, steer.StopFormStopContinue}, true
	case id == pattern.SubtaskGate && event == post:
		return nudge, true
	case id == pattern.SubtaskGate && kind == "" && task && event == pre:
		return ceiling{steer.LevelStop, steer.StopFormDeny}, true
	case id == pattern.SubtaskGate && kind == "" && task && event == eventTaskCompleted:
		return flag, true
	case id == pattern.RegressionLocation && (event == string(steer.EventPostToolUseFailure) || event == post):
		return nudge, true
	case id == pattern.PlatformRule && event == post:
		return nudge, true
	case id == pattern.OrderingRule && event == post:
		return nudge, true
	case id == pattern.OrderingRule && event == pre:
		return ceiling{steer.LevelStop, steer.StopFormAsk}, true
	case id == pattern.Integrity && (event == post || event == string(steer.EventStop)):
		return flag, true
	case id == pattern.SubagentClaim && (event == eventSubagentStop || event == post):
		return flag, true
	}
	return ceiling{}, false
}

// keyFor is the card's unit of limit (P-07 proposal; archive 40.3.4: "dedupe
// key within a prompt (fact id, gate family digest, task id)"). Kind is
// already beside the key in the dedupe triple, so the key never repeats it:
//
//   - subtask-gate, a task completion: "task:<task_ref>", so the budget's
//     two denials are per task (40.4.1: "At most 2 denials per task per
//     prompt"); a TodoWrite completion names no task and keys "todo".
//   - subtask-gate at a commit, push or pr, and regression-location: the
//     gate id, the family (40.4.2: "Once per F per prompt").
//   - platform-rule and integrity/frozen-path: "<rule>|<path>" for a
//     forbid_code or protect_path rule, which always names its file, and
//     the rule alone for a forbid_command rule, which never does
//     (ruleShapeFits; 40.4.6: "Once per rule per file per prompt").
//   - ordering-rule: the rule.
//   - integrity from a check: the seq of the edit that moved it, the fact.
//   - claim-vs-record: the claim class, one per class per prompt; the one
//     continuation per prompt is the budget's (tech 4.2.5).
//   - subagent-claim: "<subagent id>|<class>", one per subagent and class.
func keyFor(f Finding, kind steer.Kind, at Point) (string, string) {
	switch f.Pattern {
	case pattern.SubtaskGate:
		if kind != "" {
			return string(f.Gate.GateID), ""
		}
		if at.TaskRef == nil {
			return "todo", ""
		}
		if !refForm(*at.TaskRef) {
			return "", "task ref outside its charset"
		}
		return "task:" + *at.TaskRef, ""
	case pattern.RegressionLocation:
		return string(f.Gate.GateID), ""
	case pattern.PlatformRule, pattern.OrderingRule, pattern.Integrity:
		if f.Rule == nil {
			return strconv.FormatInt(f.Integrity.Seq, 10), ""
		}
		if f.Rule.Kind == spec.ForbidCode || f.Rule.Kind == spec.ProtectPath {
			return string(f.Rule.RuleID) + "|" + string(*f.Rule.Path), ""
		}
		return string(f.Rule.RuleID), ""
	case pattern.SubagentClaim:
		if at.AgentID == nil || !refForm(*at.AgentID) {
			return "", "subagent-claim without the subagent's id"
		}
		return *at.AgentID + "|" + string(f.Claim.Class), ""
	}
	return string(f.Claim.Class), ""
}

// refForm reports a task or agent reference in slot.TaskID's charset
// (archive 40.3.8: ^[A-Za-z0-9._-]{1,64}$).
func refForm(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !alnum(c) && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

package hookio

import (
	"encoding/json"

	"github.com/altrace-dev-role/rashomon/internal/events"
)

// The plan-mode exit as Claude Code reports it (W-M2.6): a PostToolUse of
// the ExitPlanMode tool, whose tool_input holds the plan's markdown under
// "plan" (and the file it was kept in under "planFilePath"; PLAN.md section
// 14 records both keys, read at Claude Code 2.1.280). A PreToolUse of the
// same tool is the act before it ran and stays act_declared, so a plan that
// was rejected, which fires a Pre and no Post, is never a plan event. The two
// literals below appear in this file only.

// PlanEvent maps a hook event and tool to events.Plan: a PostToolUse of
// ExitPlanMode. Any other pair is false, and EventKind is unchanged.
func PlanEvent(hookEvent, tool string) (events.Kind, bool) {
	if hookEvent == "PostToolUse" && tool == "ExitPlanMode" {
		return events.Plan, true
	}
	return "", false
}

// PlanText is the plan's markdown from an ExitPlanMode tool_input: the
// "plan" key's string. It is false when the key is absent, not a string or
// empty. The caller checks PlanEvent first: a real plan was 119,885 bytes,
// and no other call pays for this decode.
func PlanText(toolInput json.RawMessage) ([]byte, bool) {
	var in struct {
		Plan string `json:"plan"`
	}
	if json.Unmarshal(toolInput, &in) != nil || in.Plan == "" {
		return nil, false
	}
	return []byte(in.Plan), true
}

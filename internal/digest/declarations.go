package digest

import "github.com/altrace-dev-role/rashomon/internal/store"

// turnUnterminated is store.Run.Unterminated, scoped to one turn's own
// declarations against the WHOLE run's terminals -- a terminal answers a
// tool_use_id directly, so no window is needed to tell whether one of this
// turn's own declarations was closed.
func turnUnterminated(w turnWindow, terms []store.Terminal) []string {
	ids := turnToolUseIDs(w)
	closed := map[string]bool{}
	for _, t := range terms {
		if ids[t.ToolUseID] {
			closed[t.ToolUseID] = true
		}
	}
	var out []string
	for _, d := range w.declarations {
		if !closed[d.ToolUseID] {
			out = append(out, d.ToolUseID)
		}
	}
	return out
}

// turnDropped is store.Run.Dropped -- terminals matching no declaration
// ANYWHERE in the run -- windowed to this turn by the terminal's own
// RECORDED TIME, because a dropped terminal's declaration never landed and
// so never carried a prompt_id to key on in the first place. See
// Declarations.Dropped's doc for the limitation this admits: the attribution
// is the best the records support, not an exact one.
func turnDropped(run *store.Run, w turnWindow) []string {
	if !w.known {
		return nil
	}
	all := run.Dropped()
	if len(all) == 0 {
		return nil
	}
	recordedAt := make(map[string]int64, len(run.Terminals))
	for _, t := range run.Terminals {
		recordedAt[t.ToolUseID] = t.RecordedAtMS
	}
	var out []string
	for _, id := range all {
		if ms, ok := recordedAt[id]; ok && w.contains(ms) {
			out = append(out, id)
		}
	}
	return out
}

// subagentCounts is what a subagent contributed to this turn: declarations
// carrying a non-null agent_id, and the executions that answer them. Both are
// already inside turnExecs/w.declarations' totals -- this is a breakdown, not
// an addition -- see SubagentCounts' doc.
func subagentCounts(w turnWindow, turnExecs []store.Execution) SubagentCounts {
	var sc SubagentCounts
	subagentIDs := map[string]bool{}
	for _, d := range w.declarations {
		if d.AgentID != nil && *d.AgentID != "" {
			sc.Declarations++
			subagentIDs[d.ToolUseID] = true
		}
	}
	for _, x := range turnExecs {
		if subagentIDs[x.ToolUseID] {
			sc.Executions++
		}
	}
	return sc
}

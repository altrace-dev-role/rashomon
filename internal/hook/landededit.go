package hook

import "github.com/altrace-dev-role/rashomon/internal/steer"

// LandedGoEdit is a success event's landed Go edit as cmd's unit-brief read
// takes it (card m2-unitbrief-hook1 D4): the session and agent, the index
// root and the file's slash path under it, the seq the execution landed at,
// and whether the call's record scope is v5 (else the finding's record form
// holds digests, as the prompt's graph brief does).
type LandedGoEdit struct {
	Session, AgentID string
	Root, Rel        string
	Seq              int64
	ScopeV5          bool
}

// LandedGoEdit is the call's landed Go edit, false for any other call:
// a PostToolUse whose Write, Edit or MultiEdit passed the overlay writer's
// own gate (keptGoEdit), so a session whose content is not kept pays one
// marker read here and nothing else.
func (p *Post) LandedGoEdit() (LandedGoEdit, bool) {
	if p.event != string(steer.EventPostToolUse) {
		return LandedGoEdit{}, false
	}
	_, root, rel, ok := keptGoEdit(p.st, p.sessionID, p.toolName, p.toolInput, p.cwd, p.landedSeq)
	if !ok {
		return LandedGoEdit{}, false
	}
	return LandedGoEdit{Session: p.sessionID, AgentID: p.agentID, Root: root, Rel: rel, Seq: p.landedSeq, ScopeV5: p.scopeV5}, true
}

package hook

import (
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// LandedGoEdit is a success event's landed Go edit as cmd's unit-brief read
// takes it (card m2-unitbrief-hook1 D4): the session and agent, the index
// root and the file's slash path under it, the hook's working directory
// (the note names the unit relative to it, as the prompt's brief names its
// paths), the seq the execution landed at, and whether the call's record
// scope is v5 (else the finding's record form holds digests, as the
// prompt's graph brief does).
type LandedGoEdit struct {
	Session, AgentID string
	Root, Rel        string
	Cwd              string
	Seq              int64
	ScopeV5          bool
	// Group is the edit's landed overlay group, set by LandedOverlay only.
	Group graph.OverlayGroup
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
	return LandedGoEdit{Session: p.sessionID, AgentID: p.agentID, Root: root, Rel: rel, Cwd: p.cwd, Seq: p.landedSeq, ScopeV5: p.scopeV5}, true
}

// Unheard is whether a finding joined for the call under env would reach no
// one: the join's own gate (steering off, a config that does not read, no
// consent, no prompt), read from the fields contextOf takes (heard), before
// any context is built. cmd's unit-brief read asks it before it reads its
// file (ruling 04:58 C, cause-first), so a session whose notes reach no one
// pays the switch's read, not the read, decode and render the join drops.
func (p *Post) Unheard(env SteerEnv) bool { return unheard(p.heard(env)) }

// LandedOverlay is LandedGoEdit as the overlay writer kept it at this post,
// with the edit's landed group (card m2-seam1 D3): the writer passed the
// same gate (keptGoEdit) and resolved the file once, so the conflict check
// repeats neither the gate nor the parse. False for a call whose writer
// kept no group, and for any event but PostToolUse.
func (p *Post) LandedOverlay() (LandedGoEdit, bool) {
	if !p.landedKept || p.event != string(steer.EventPostToolUse) {
		return LandedGoEdit{}, false
	}
	e := p.landed
	e.Session, e.AgentID, e.Cwd, e.Seq, e.ScopeV5 = p.sessionID, p.agentID, p.cwd, p.landedSeq, p.scopeV5
	return e, true
}

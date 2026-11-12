package main

import (
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// conflictKeyPrefix opens a conflict's steer key: the prefix, the
// conflict's identity (engine Conflict.Key) as a keyed digest, "@", and the
// commit's first twelve characters, so a conflict nudges once a session per
// commit (card m2-seam1 D6).
const conflictKeyPrefix = "conflict:"

// conflictTier is the tier a conflict's source carries: the rule is
// declared, by a spec or a knowledge row.
const conflictTier = "DECLARED"

// conflictJoin is the post's conflict check (W-M2.4 hook seam, card m2-seam1
// D3 to D6), nil for no finding. The checks run cheapest first: the overlay
// writer's own gate and group (Post.LandedOverlay: no second gate and no
// second parse), the session's conflict base for the index (one small read;
// none is a miss and nothing is checked, never an index load), the base at
// the group's commit (a base at another commit says nothing of this tree),
// and the file in a rule's scope (engine.ConflictBase.InScope; out of scope
// raises nothing and records nothing). Then the edit's conflicts, the
// engine's own DiffOf and Conflicts over the base and the group, of the live
// kinds (verify.ConflictRowKinds, none withheld), are kept as one
// conflict_check record, possibly empty, whatever is then delivered (D4),
// and each not yet given at this commit becomes a nudge (D6): pattern
// declared-fact, kind conflict, never above nudge, never a stop (R2).
func conflictJoin(st *store.Store, p *hook.Post, env hook.SteerEnv) []steer.Finding {
	e, ok := p.LandedOverlay()
	if !ok {
		return nil
	}
	cb, ok := readConflictBase(conflictBasePath(st.Root(), e.Session, e.Root))
	if !ok || cb.Commit != e.Group.Commit || e.Group.File != graph.FileID(e.Rel) {
		return nil
	}
	unit, ok := gopg.Resolver{}.Unit("", e.Rel)
	base := engine.ConflictBase{Records: cb.Records, Digests: cb.Digests}
	if !ok || !base.InScope(cb.Rules, e.Group.File, unit) {
		return nil
	}
	var live []engine.Conflict
	for _, c := range engine.EditConflicts(base.Store(cb.Commit), cb.Rules, e.Group, cb.Digests[e.Group.File]) {
		if k := string(c.Kind); slices.Contains(verify.ConflictRowKinds, k) && !slices.Contains(verify.WithheldConflictKinds, k) {
			live = append(live, c)
		}
	}
	key := st.Key()
	digest := func(s string) string { return intent.PathDigest(key, s) }
	rows := make([]store.ConflictCheckRow, len(live))
	for i, c := range live {
		rows[i] = conflictCheckRow(c, cb.Repo, e.ScopeV5, digest)
	}
	_ = st.AppendRecord(e.Session, store.NewConflictCheck(e.Session, e.Seq, cb.Commit, digest(string(e.Group.File)), rows,
		env.Now.UnixMilli()))
	if len(live) == 0 || p.Unheard(env) {
		return nil
	}
	return conflictFindings(st, e, cb.Repo, cb.Commit, live, rows)
}

// conflictCheckRow is c as its record row holds it: at record scope v5 the
// unit and subject as global ids and the unit's path; else both as keyed
// digests and no path. Kind and rule id are in the clear, the key a digest.
func conflictCheckRow(c engine.Conflict, repo string, v5 bool, digest func(string) string) store.ConflictCheckRow {
	unit, subject := string(graph.GlobalID(repo, c.Unit)), string(graph.GlobalID(repo, c.Subject))
	row := store.ConflictCheckRow{Kind: string(c.Kind), RuleID: c.RuleID, Key: digest(c.Key())}
	if v5 {
		row.Unit, row.UnitPath, row.Subject = unit, unitDir(c.Unit), subject
	} else {
		row.Unit, row.Subject = digest(unit), digest(subject)
	}
	return row
}

// unitDir is a unit id's path in its repository: the part after
// "unit:<lang>:", "" for an id of another form.
func unitDir(u graph.ID) string {
	rest, ok := strings.CutPrefix(string(u), "unit:")
	if !ok {
		return ""
	}
	_, p, ok := strings.Cut(rest, ":")
	if !ok {
		return ""
	}
	return p
}

// conflictFindings is a nudge for each of live not given at commit in the
// session's steer log (hook.Given, as the plan check and the unit brief read
// theirs): the note names the unit relative to the hook's working directory
// at every scope, and where the record scope is not v5 the steer line holds
// the record form, which names no unit.
func conflictFindings(st *store.Store, e hook.LandedGoEdit, repo, commit string, live []engine.Conflict,
	rows []store.ConflictCheckRow) []steer.Finding {
	lines, _, _ := st.ReadSteer(e.Session)
	prio, _ := pattern.Priority(pattern.DeclaredFact)
	at := commit[:min(12, len(commit))]
	var out []steer.Finding
	for i, c := range live {
		key := conflictKeyPrefix + rows[i].Key + "@" + at
		given := false
		hook.Given(lines, pattern.DeclaredFact, key, true, func(string) { given = true })
		if given {
			continue
		}
		m, rec, ok := conflictMessages(c, slot.RelPath(relTo(e.Cwd, e.Root)(unitDir(c.Unit))), e.ScopeV5)
		if !ok {
			continue
		}
		out = append(out, steer.Finding{Pattern: pattern.DeclaredFact, Kind: steer.KindConflict, Key: key, Priority: prio,
			MaxLevel: steer.LevelNudge, StopForm: steer.StopFormNone, Message: m, Record: rec, AfterSeq: e.Seq,
			Unit: string(graph.GlobalID(repo, c.Unit)), AgentID: e.AgentID,
			Source: steer.Source{Kind: steer.SourceSpec, ID: string(graph.GlobalID(repo, graph.ID("spec:"+c.RuleID))), Tier: conflictTier}})
	}
	return out
}

// conflictMessages is c's note naming unit and, unless v5, its record form.
// A rule id the slot refuses is left out, the note saying "a spec rule" as
// the Stop's line does; ok is false when no form renders.
func conflictMessages(c engine.Conflict, unit slot.RelPath, v5 bool) (m, rec render.Message, ok bool) {
	kind := slot.NewEnum(slot.EnumConflictKind, string(c.Kind))
	rule := slot.RuleID(strings.TrimPrefix(c.RuleID, "spec:"))
	var err error
	if rule.Validate() == nil {
		m, err = render.New(pattern.DeclaredFact, render.ConflictBinding{Unit: unit, Kind: kind, Rule: rule})
		if err == nil && !v5 {
			rec, err = render.New(pattern.DeclaredFact, render.ConflictRecordBinding{Kind: kind, Rule: rule})
		}
	} else {
		m, err = render.New(pattern.DeclaredFact, render.ConflictUnruledBinding{Unit: unit, Kind: kind})
		if err == nil && !v5 {
			rec, err = render.New(pattern.DeclaredFact, render.ConflictUnruledRecordBinding{Kind: kind})
		}
	}
	return m, rec, err == nil
}

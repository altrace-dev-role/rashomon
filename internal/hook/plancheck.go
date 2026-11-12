package hook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The plan-check note (W-M2.6 hook half): after the plan-mode exit lands
// (the PostToolUse of ExitPlanMode, the plan the person accepted), the paths
// the plan names that a spec rule covers are said once, in the next request
// after the call. The channel is Post only: a rejected plan fires a Pre and
// no Post, so it gets no line. The plan's words are read in memory and kept
// nowhere: the note names paths (the delivered form), the steer line holds a
// count and keyed digests at every record scope, and the plan_check record
// (store.PlanCheck) holds counts, rule ids and keyed digests.

// PlanItem is one (path, rule) the plan check found: the path as the plan
// named it, whether the project does not hold it yet (a planned file), the
// rule's id and kind, and the trust word of the spec source that declared
// the rule.
type PlanItem struct {
	Path, RuleID, Kind, Trust string
	Planned                   bool
}

// PlanPort is the check's port: Check reads the plan's items against the
// index found from cwd and the spec the hook discovered for the call. cmd
// wires it to PlanFacts; the hook links no graph reader. A port with Check
// nil reads nothing.
type PlanPort struct {
	Check func(plan []byte, cwd string, sp *spec.Compiled) ([]PlanItem, bool)
}

// planDomain is the domain of the plan's keyed digest, apart from the other
// keyed digests of the store's key.
const planDomain = "\x00plan-check"

// planDigest is 16 hex digits of an HMAC-SHA256 under key over the plan's
// bytes: the finding key's unit, equal for an equal plan, and not the plan.
func planDigest(key, plan []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(planDomain))
	m.Write([]byte{0})
	m.Write(plan)
	return hex.EncodeToString(m.Sum(nil))[:16]
}

// plan is the call's plan-check finding, or false: a success event's
// ExitPlanMode that landed, with the port wired, all three read before the
// input is decoded (a real plan was 119,885 bytes, and every other call pays
// nothing). ctx is the call's steer context, built here from contextOf when
// the judge built none (ok false); a context whose note could reach no one
// (steering off, no consent, no prompt) reads nothing: the one consent
// decision is the context's, and no marker or settings file is opened here.
// Whenever the plan is checked a plan_check record is kept, delivered or not.
// A plan the session was already given the note for says nothing again.
func (p *Post) plan(env SteerEnv, ctx *steer.Context, ok bool) (steer.Finding, bool) {
	if env.Plan.Check == nil || p.landedSeq <= 0 {
		return steer.Finding{}, false
	}
	if _, isPlan := hookio.PlanEvent(p.event, p.toolName); !isPlan {
		return steer.Finding{}, false
	}
	text, found := hookio.PlanText(p.toolInput)
	if !found {
		return steer.Finding{}, false
	}
	if !ok {
		*ctx = p.contextOf(env, verify.SteerView{ProjectRoot: blastRoot(p.cwd)})
	}
	if ctx.SteerOff || ctx.ConfigUnreadable || !ctx.Consent || ctx.PromptID == nil {
		return steer.Finding{}, false
	}
	sp := p.sp
	if sp == nil {
		sp = p.discoverFor(env, baseline.Key(p.cwd), nil, false)
	}
	items, _ := env.Plan.Check(text, p.cwd, sp)
	key := p.st.Key()
	paths, planned, rules := planSets(items)
	digests := make([]string, len(paths))
	for i, path := range paths {
		digests[i] = intent.PathDigest(key, path)
	}
	ruleIDs := make([]string, len(rules))
	for i, r := range rules {
		ruleIDs[i] = r.ID
	}
	_ = p.st.AppendRecord(p.sessionID, store.NewPlanCheck(p.sessionID, p.landedSeq, ruleIDs, len(paths), planned, len(items),
		digests, env.Now.UnixMilli()))
	if len(items) == 0 {
		return steer.Finding{}, false
	}
	fkey := "plan@" + planDigest(key, text)
	lines, _, _ := p.st.ReadSteer(p.sessionID)
	if planGiven(lines, fkey) {
		return steer.Finding{}, false
	}
	slotPaths := make([]slot.Path, len(paths))
	for i, path := range paths {
		slotPaths[i] = slot.Path(path)
	}
	m, err := render.New(pattern.PlanCheck, render.PlanCheckBinding{
		Paths: slot.NewPathSet(slotPaths, 0, 0), Rules: slot.NewRuleRefs(rules, 0, 0, 0)})
	if err != nil {
		return steer.Finding{}, false
	}
	rec, err := render.New(pattern.PlanCheck, render.PlanCheckBinding{
		Paths: slot.NewDigestPathSet(digests, 0, 0), Rules: slot.NewRuleRefs(rules, 0, 0, 0)})
	if err != nil {
		return steer.Finding{}, false
	}
	f := steer.Finding{Pattern: pattern.PlanCheck, Key: fkey, MaxLevel: steer.LevelNudge, StopForm: steer.StopFormNone,
		Message: m, Record: rec, AfterSeq: p.landedSeq, AgentID: p.agentID}
	f.Priority, _ = pattern.Priority(pattern.PlanCheck)
	return f, true
}

// planSets is the distinct paths of items in the order found, how many of
// them are planned, and the distinct rules as the note names them.
func planSets(items []PlanItem) (paths []string, planned int, rules []slot.RuleRef) {
	var seen []string
	for _, it := range items {
		if !slices.Contains(paths, it.Path) {
			paths = append(paths, it.Path)
			if it.Planned {
				planned++
			}
		}
		if !slices.Contains(seen, it.RuleID) {
			seen = append(seen, it.RuleID)
			rules = append(rules, slot.RuleRef{ID: it.RuleID, Trust: slot.NewEnum(slot.EnumTrust, it.Trust)})
		}
	}
	return paths, planned, rules
}

// planGiven reports a plan-check line of the session with this key that was
// given: delivered (a held-out line keeps that state), or withheld by the off
// arm, whose decision gave it (as lastGiven reads a blast-radius line).
func planGiven(lines [][]byte, key string) bool {
	var sp [4]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, blastKeys(), sp[:]) != nil || spanText(b, sp[0]) != string(pattern.PlanCheck) || spanText(b, sp[2]) != key {
			continue
		}
		if spanText(b, sp[1]) == string(steer.LineStateDelivered) || spanText(b, sp[3]) == string(steer.SuppressedByArmOff) {
			return true
		}
	}
	return false
}

package intent

import (
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// The finding's keys: the first prompt's note carries the brief, every
// later one is a note; ReadDelivered reads the brief's key back.
const (
	keyBrief = "brief"
	keyNote  = "note"
)

// Finding is the request's task-preflight finding, or false when the
// request has nothing new to say. The first note of a session carries the
// brief when b has anything in it, whatever the request names. Otherwise a
// request that named no path and no identifier gets none, and one that did
// gets a note only when a path, rule, gate or evidence kind is new to the
// session or the request was cut. Paths and rules given earlier are counted;
// gates and evidence print in full. The note steps down to fit the briefing
// cap: whys dropped first, then one item per list.
func Finding(in Intent, d Delivered, b *Brief, afterSeq int64) (steer.Finding, bool) {
	return FindingKeyed(in, d, b, afterSeq, nil)
}

// FindingKeyed is Finding for a line recorded at record scope v4 (card
// PREFLIGHT1b). digest, when not nil, is the keyed digest of a path
// (PathDigest): the delivered set holds digests, so a candidate path is
// digested before the "given earlier" check, and the finding's Record is the
// message the steer line holds, each path list as its count and its keyed
// digests. The message that prints is Finding's, unchanged. A nil digest is
// scope v5: the line records what printed.
func FindingKeyed(in Intent, d Delivered, b *Brief, afterSeq int64, digest func(string) string) (steer.Finding, bool) {
	first := !d.BriefGiven && b.has()
	var paths []string
	var rules []Rule
	earlierPaths, earlierRules := 0, 0
	seen := func(p string) bool {
		if digest != nil {
			return d.Paths[digest(p)]
		}
		return d.Paths[p]
	}
	for _, p := range in.Paths {
		if seen(p) {
			earlierPaths++
		} else {
			paths = append(paths, p)
		}
	}
	for _, r := range in.Rules {
		if d.Rules[r.ID] {
			earlierRules++
		} else {
			rules = append(rules, r)
		}
	}
	fresh := len(paths) > 0 || len(rules) > 0
	for _, g := range in.Gates {
		fresh = fresh || !d.Gates[g.Words]
	}
	for _, k := range in.Evidence {
		fresh = fresh || !d.Evidence[k]
	}
	if !first && (!in.Named || !fresh && !in.Truncated) {
		return steer.Finding{}, false
	}

	unknown := in.Unknown
	if first {
		if b.Module == "" {
			unknown = append(unknown[:len(unknown):len(unknown)], unknownNoModule)
		}
		if b.WalkCapped {
			unknown = append(unknown[:len(unknown):len(unknown)], unknownCapped)
		}
	}
	source := "inferred from the request"
	if in.FromTask {
		source = "task file"
	}
	for step := range 3 {
		n := -1 // every item
		if step == 2 {
			n = 1
		}
		pf := render.TaskPreflightBinding{
			Paths:    pathSet(paths, n, earlierPaths),
			Rules:    ruleRefs(rules, n, earlierRules, in.Withheld, step == 0),
			Gates:    gateRefs(in.Gates, n, in.Learned),
			Source:   slot.NewEnum(slot.EnumEvidenceSource, source),
			Evidence: slot.NewEnumList(slot.EnumEvidenceKinds, in.Evidence),
			Unknown:  slot.NewEnumList(slot.EnumPreflightUnknown, unknown),
		}
		var binding any = pf
		key := keyNote
		if first {
			key = keyBrief
			binding = render.TaskPreflightFirstBinding{
				Instructions: pathSet(b.Instructions, n, 0), Manifests: pathSet(b.Manifests, n, 0), GoDirs: slot.Count(b.GoDirs),
				Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source, Evidence: pf.Evidence, Unknown: pf.Unknown,
			}
		}
		m, err := render.New(pattern.TaskPreflight, binding)
		if err != nil {
			continue
		}
		var rec render.Message
		if digest != nil {
			if rec, err = render.New(pattern.TaskPreflight, recordBinding(pf, first, b, n, digest)); err != nil {
				continue
			}
		}
		prio, _ := pattern.Priority(pattern.TaskPreflight)
		return steer.Finding{Pattern: pattern.TaskPreflight, Key: key, Priority: prio, MaxLevel: steer.LevelNudge,
			StopForm: steer.StopFormNone, Message: m, Record: rec, AfterSeq: afterSeq}, true
	}
	return steer.Finding{}, false
}

// recordBinding is the binding the steer line holds at record scope v4: pf's
// every path list as keyed digests (the same items pf lists at this step),
// the rest as it is.
func recordBinding(pf render.TaskPreflightBinding, first bool, b *Brief, n int, digest func(string) string) any {
	keyed := func(s slot.PathSet) slot.PathSet {
		ps := s.Paths()
		ds := make([]string, len(ps))
		for i, p := range ps {
			ds[i] = digest(string(p))
		}
		return slot.NewDigestPathSet(ds, s.More(), s.Earlier())
	}
	pf.Paths = keyed(pf.Paths)
	if !first {
		return pf
	}
	return render.TaskPreflightFirstBinding{
		Instructions: keyed(pathSet(b.Instructions, n, 0)), Manifests: keyed(pathSet(b.Manifests, n, 0)), GoDirs: slot.Count(b.GoDirs),
		Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source, Evidence: pf.Evidence, Unknown: pf.Unknown,
	}
}

// listed is the first n items (every item for n < 0) and the count of the
// rest.
func listed[T any](items []T, n int) ([]T, int) {
	if n < 0 || len(items) <= n {
		return items, 0
	}
	return items[:n], len(items) - n
}

func pathSet(paths []string, n, earlier int) slot.PathSet {
	shown, more := listed(paths, n)
	ps := make([]slot.Path, len(shown))
	for i, p := range shown {
		ps[i] = slot.Path(p)
	}
	return slot.NewPathSet(ps, more, earlier)
}

func ruleRefs(rules []Rule, n, earlier, withheld int, whys bool) slot.RuleRefs {
	shown, more := listed(rules, n)
	refs := make([]slot.RuleRef, len(shown))
	for i, r := range shown {
		refs[i] = slot.RuleRef{ID: r.ID, Trust: slot.NewEnum(slot.EnumTrust, string(r.Trust))}
		if whys {
			refs[i].Why = slot.Approved(r.Why)
		}
	}
	return slot.NewRuleRefs(refs, more, earlier, withheld)
}

func gateRefs(gates []Gate, n, learned int) slot.GateRefs {
	shown, more := listed(gates, n)
	refs := make([]slot.GateRef, len(shown))
	for i, g := range shown {
		refs[i] = slot.GateRef{Words: slot.Gate(g.Words), Trust: slot.NewEnum(slot.EnumTrust, string(g.Trust))}
	}
	return slot.NewGateRefs(refs, more, learned)
}

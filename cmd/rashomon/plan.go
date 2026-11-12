package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// planNames is what a plan's text names in the index under cwd: the index
// (store, header repository, root) and the diff the names make. The plan's
// text is not kept; only ids the index or the project holds are.
type planNames struct {
	Store *graph.Store
	Repo  string
	Root  string
	Diff  engine.Diff
	// Planned is the root-relative paths the plan names that the project
	// does not hold and whose parent directory it does (intent.ResolvePlan):
	// files the plan may add. They are not in Diff.
	Planned []string
}

// planNamesOf resolves the plan's named paths and symbols against the index
// found from cwd (graphBrief's lookup). false when there is no index or it
// cannot be read. A named path the index holds is a file with its unit; one
// it lacks (a file on disk the index has not seen) is still a file, with
// the unit its claiming resolver gives, so a planned new file has a unit. A
// named symbol adds its unit. The diff has a Rev, Touched and Files alone.
func planNamesOf(plan []byte, cwd string) (planNames, bool) {
	if cwd == "" || len(plan) == 0 {
		return planNames{}, false
	}
	root := graph.FindIndex(cwd)
	if root == "" {
		return planNames{}, false
	}
	s, h, err := graph.LoadFile(root)
	if err != nil {
		return planNames{}, false
	}
	rev := graph.RevCommit(h.Rev.Commit)
	var symUnits []graph.ID
	in := intent.Resolve(plan, false, intent.Project{Root: root, Cwd: cwd, Symbols: planSymbols(s, rev, &symUnits)})

	var reg lang.Registry
	reg.Register(gopg.Resolver{})
	var files, units []graph.ID
	for _, rel := range in.Paths {
		fid := graph.FileID(rel)
		files = append(files, fid)
		if _, held := s.Node(fid, rev); held {
			if u := unitOf(s, rev, fid); u != "" {
				units = append(units, u)
			}
		} else if r, ok := reg.For(root, rel); ok {
			if u, ok := r.Unit(root, rel); ok {
				units = append(units, u)
			}
		}
	}
	units = append(units, symUnits...)
	_, planned := intent.ResolvePlan(intent.NamedPaths(plan), intent.Project{Root: root, Cwd: cwd})
	return planNames{Store: s, Repo: h.Repo, Root: root, Diff: engine.Diff{Rev: rev,
		Touched: slices.Compact(slices.Sorted(slices.Values(units))),
		Files:   slices.Compact(slices.Sorted(slices.Values(files)))}, Planned: planned}, true
}

// planSymbols answers intent's symbol lookup from the index's sym nodes at
// rev: a node whose name attr is the identifier or ends in "."+identifier.
// Every unit of a match is appended to units. At most four matches an
// identifier.
func planSymbols(s *graph.Store, rev graph.Rev, units *[]graph.ID) func([]string) []intent.Symbol {
	return func(names []string) []intent.Symbol {
		var out []intent.Symbol
		seen := map[string]int{}
		for _, n := range s.Nodes(rev) {
			if n.Kind != "sym" {
				continue
			}
			for _, name := range names {
				got := n.Attrs["name"]
				if got != name && !strings.HasSuffix(got, "."+name) || seen[name] == 4 {
					continue
				}
				seen[name]++
				u := unitOf(s, rev, n.ID)
				if u == "" {
					continue
				}
				*units = append(*units, u)
				out = append(out, intent.Symbol{Name: name, Dir: strings.TrimPrefix(string(u), "unit:"), Callers: len(s.In(n.ID, rev, graph.Calls))})
			}
		}
		return out
	}
}

// The kinds of a PlanFact, from the engine's rows: the three conflicts the
// plan check reads and the boundary caution.
const (
	PlanKindProtect   = "protect_path"
	PlanKindUntouched = "paths_untouched"
	PlanKindDecision  = "decision"
	PlanKindBoundary  = "boundary"
)

// PlanFact is one (path, rule) the plan check found: a path as the plan
// named it and a rule that covers it. Planned says the path is one the
// project does not hold yet (intent.ResolvePlan), which reaches path rules
// alone (a planned file has no unit, symbols or imports). Subject is the
// conflict's subject as the lines print it (the path, or a unit or symbol
// for a decision) and Path is Subject when that is a file, else "". A
// boundary caution is From in Subject and To in To. Source is the knowledge
// row a decision was compiled from and Trust the trust of the spec source
// that declared the rule ("trusted" for a rule of an approved row).
type PlanFact struct {
	Path, Subject, To string
	Planned           bool
	RuleID, Kind      string
	Source, Trust     string
}

// PlanFacts is the plan-time check over the index under cwd, one fact per
// (path, rule), in the order PlanLines prints them: the conflicts in the
// engine's order, then the boundary cautions, then the planned files' path
// rules. The spec is the caller's (hook.DiscoverSpec, as brief.go reads it),
// never discovered here. It writes nothing and returns nil, false when there
// is no index or nothing to say.
func PlanFacts(plan []byte, cwd string, sp *spec.Compiled) ([]PlanFact, bool) {
	n, ok := planNamesOf(plan, cwd)
	if !ok {
		return nil, false
	}
	rows, _ := knowledge.ReadDir(n.Root)
	rules := engine.RulesFor(sp, rows, n.Store, n.Repo, n.Diff.Rev)
	byID := map[string]engine.Rule{}
	for _, r := range rules {
		byID[r.ID] = r
	}
	var facts []PlanFact
	for _, c := range engine.Conflicts(n.Store, n.Diff, rules) {
		r := byID[c.RuleID]
		subject := planWord(strings.TrimPrefix(string(c.Subject), "file:"))
		f := PlanFact{Subject: subject, RuleID: r.ID, Source: r.Source, Trust: planTrust(sp, r.ID)}
		if strings.HasPrefix(string(c.Subject), "file:") {
			f.Path = subject
		}
		switch {
		case c.Kind == engine.ConflictProtectedPath:
			f.Kind = PlanKindProtect
		case c.Kind == engine.ConflictDecisionContradicted && r.Predicate == spec.PredicatePathsUntouched:
			f.Kind = PlanKindUntouched
		case c.Kind == engine.ConflictDecisionContradicted:
			f.Kind = PlanKindDecision
		default:
			continue
		}
		facts = append(facts, f)
	}
	for _, c := range engine.BoundaryCautions(n.Diff.Touched, rules) {
		facts = append(facts, PlanFact{Subject: string(c.From), To: string(c.To), RuleID: planWord(c.RuleID),
			Kind: PlanKindBoundary, Trust: planTrust(sp, c.RuleID)})
	}
	facts = append(facts, plannedFacts(n, rules, sp)...)
	if len(facts) == 0 {
		return nil, false
	}
	return facts, true
}

// plannedFacts is the path rules the planned files fall under: a protect_path
// rule matching on its Paths (ruling 5: no boundary, decision or duplicate
// rule reads a file that has no unit yet). One fact a (file, rule).
func plannedFacts(n planNames, rules []engine.Rule, sp *spec.Compiled) []PlanFact {
	if len(n.Planned) == 0 {
		return nil
	}
	var paths []engine.Rule
	for _, r := range rules {
		if r.Kind == string(spec.ProtectPath) && len(r.Paths) > 0 {
			paths = append(paths, r)
		}
	}
	files := make([]graph.ID, len(n.Planned))
	for i, rel := range n.Planned {
		files[i] = graph.FileID(rel)
	}
	var out []PlanFact
	for _, c := range engine.Conflicts(n.Store, engine.Diff{Rev: n.Diff.Rev, Files: files}, paths) {
		if c.Kind != engine.ConflictProtectedPath {
			continue
		}
		subject := planWord(strings.TrimPrefix(string(c.Subject), "file:"))
		out = append(out, PlanFact{Path: subject, Subject: subject, Planned: true, RuleID: c.RuleID,
			Kind: PlanKindProtect, Trust: planTrust(sp, c.RuleID)})
	}
	return out
}

// planTrust is the trust word of the spec source that declared rule id, and
// "trusted" for a rule that is no spec's (one compiled from an approved
// knowledge row).
func planTrust(sp *spec.Compiled, id string) string {
	if sp != nil {
		for _, d := range sp.Rules {
			if d.Rule.ID == id {
				return string(sp.TrustOf(d.Source))
			}
		}
	}
	return string(spec.Trusted)
}

// PlanLines is the plan-time check over the index under cwd: at most two
// lines, one finding, rendered over PlanFacts (the spec is the caller's, as
// there). Path-rule lines lead (protect_path, then paths_untouched), then
// another decision's, then a boundary caution. A planned file has no line
// here; the hook's note names it. It writes nothing and returns nil, false
// when there is no index or nothing to say.
func PlanLines(plan []byte, cwd string, sp *spec.Compiled) ([]string, bool) {
	facts, ok := PlanFacts(plan, cwd, sp)
	if !ok {
		return nil, false
	}
	var protect, untouched, decision, caution []string
	for _, f := range facts {
		r := engine.Rule{ID: f.RuleID, Source: f.Source}
		switch {
		case f.Planned:
		case f.Kind == PlanKindProtect:
			protect = append(protect, fmt.Sprintf("the plan names %s; rule %s protects that path", f.Subject, planWord(f.RuleID)))
		case f.Kind == PlanKindUntouched:
			untouched = append(untouched, fmt.Sprintf("the plan names %s; %s says it stays untouched", f.Subject, planDecision(r)))
		case f.Kind == PlanKindDecision:
			decision = append(decision, fmt.Sprintf("the plan touches %s; %s applies", f.Subject, planDecision(r)))
		case f.Kind == PlanKindBoundary:
			caution = append(caution, fmt.Sprintf("the plan names %s and %s; rule %s forbids edges from %s into %s",
				f.Subject, f.To, f.RuleID, f.Subject, f.To))
		}
	}
	var lines []string
	for _, group := range [][]string{protect, untouched, decision, caution} {
		lines = append(lines, group...)
	}
	if len(lines) == 0 {
		return nil, false
	}
	return lines[:min(len(lines), 2)], true
}

// planDecision names a decision rule, with the knowledge row it was
// compiled from when it has one.
func planDecision(r engine.Rule) string {
	if r.Source == "" {
		return "decision " + planWord(r.ID)
	}
	return fmt.Sprintf("decision %s (%s)", planWord(r.ID), planWord(r.Source))
}

// planWord is s as one printable token: control characters dropped, cut to
// 60 runes.
func planWord(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	for utf8.RuneCountInString(s) > 60 {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	return s
}

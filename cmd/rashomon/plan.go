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
	return planNames{Store: s, Repo: h.Repo, Root: root, Diff: engine.Diff{Rev: rev,
		Touched: slices.Compact(slices.Sorted(slices.Values(units))),
		Files:   slices.Compact(slices.Sorted(slices.Values(files)))}}, true
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

// PlanLines is the plan-time check over the index under cwd: at most two
// lines, one finding. The spec is the caller's (hook.DiscoverSpec, as
// brief.go reads it), never discovered here. Path-rule lines lead
// (protect_path, then paths_untouched), then another decision's, then a
// boundary caution. It writes nothing and returns nil, false when there is
// no index or nothing to say.
func PlanLines(plan []byte, cwd string, sp *spec.Compiled) ([]string, bool) {
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
	var protect, untouched, decision, caution []string
	for _, c := range engine.Conflicts(n.Store, n.Diff, rules) {
		r := byID[c.RuleID]
		subject := planWord(strings.TrimPrefix(string(c.Subject), "file:"))
		switch {
		case c.Kind == engine.ConflictProtectedPath:
			protect = append(protect, fmt.Sprintf("the plan names %s; rule %s protects that path", subject, planWord(r.ID)))
		case c.Kind == engine.ConflictDecisionContradicted && r.Predicate == spec.PredicatePathsUntouched:
			untouched = append(untouched, fmt.Sprintf("the plan names %s; %s says it stays untouched", subject, planDecision(r)))
		case c.Kind == engine.ConflictDecisionContradicted:
			decision = append(decision, fmt.Sprintf("the plan touches %s; %s applies", subject, planDecision(r)))
		}
	}
	for _, c := range engine.BoundaryCautions(n.Diff.Touched, rules) {
		caution = append(caution, fmt.Sprintf("the plan names %s and %s; rule %s forbids edges from %s into %s",
			c.From, c.To, planWord(c.RuleID), c.From, c.To))
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

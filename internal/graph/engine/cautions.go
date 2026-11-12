package engine

import (
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// filesUnderPaths is the files in d.Files, in order, that a pattern of
// r.Paths matches under spec.MatchPath on the project-relative path and that
// r.AppliesTo does not already name: the files a Paths match adds, so a file
// is raised once however it matched.
func filesUnderPaths(d Diff, r Rule) []graph.ID {
	if len(r.Paths) == 0 {
		return nil
	}
	var out []graph.ID
	for _, id := range d.Files {
		rel, ok := strings.CutPrefix(string(id), "file:")
		if !ok || slices.Contains(r.AppliesTo, id) || slices.Contains(out, id) {
			continue
		}
		if slices.ContainsFunc(r.Paths, func(p string) bool { return spec.MatchPath(p, rel) }) {
			out = append(out, id)
		}
	}
	return out
}

// Caution is a plan-time boundary caution: a plan that names a unit under a
// boundary rule's From and a unit under its To. It is not a conflict. No
// edge exists yet, so it says only which direction the rule forbids.
type Caution struct {
	RuleID   string
	From, To graph.ID
}

// BoundaryCautions is one Caution per boundary rule in rules, in rule
// order, when touched holds a unit under the rule's From and a different
// unit under its To (underAny, by path segment); From and To are the first
// such units in id order. Rules of other kinds give none, and Conflicts is
// unchanged by it.
func BoundaryCautions(touched []graph.ID, rules []Rule) []Caution {
	units := slices.Sorted(slices.Values(touched))
	var out []Caution
	for _, r := range rules {
		if spec.RuleKind(r.Kind) != spec.Boundary {
			continue
		}
		for _, f := range units {
			if !underAny(f, r.From) {
				continue
			}
			if i := slices.IndexFunc(units, func(t graph.ID) bool { return t != f && underAny(t, r.To) }); i >= 0 {
				out = append(out, Caution{RuleID: r.ID, From: f, To: units[i]})
				break
			}
		}
	}
	return out
}

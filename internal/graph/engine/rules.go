package engine

import (
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// RulesFor is the engine rules a check over the index reads: the spec's
// boundary and protect_path rules from a source whose trust is Trusted,
// Managed or Builtin (an untrusted repository's rule is withheld; a source
// out of the spec's range is untrusted rather than a panic), then the rules
// knowledge.Rules compiles from the approved DECLARED rows. AppliesTo is
// resolved at rev: a global id of the repository rid keeps its local id when
// the node exists at rev (R9: the engine compares AppliesTo raw, so a row in
// the documented global form would never fire unlocalised); a global id of
// another repository and an absent node are dropped, never matched by name
// (graph/global.go); a local-form id passes through unchanged. The rows
// are not mutated: knowledge.Records still reads their global ids. Paths
// carry over. Pure: the caller reads the spec (hook.DiscoverSpec) and the
// rows (knowledge.ReadDir). The plan check (cmd/rashomon/plan.go), the steer
// of W-M2.4 and QA's J3 judge all take their rules from this one function.
func RulesFor(sp *spec.Compiled, rows []knowledge.Row, g Reader, rid string, rev graph.Rev) []Rule {
	var out []Rule
	if sp != nil {
		for _, d := range sp.Rules {
			k := spec.RuleKind(d.Rule.Kind)
			if (k == spec.Boundary || k == spec.ProtectPath) && ruleSourceTrusted(sp, d.Source) {
				out = append(out, ruleOf(d.Rule, g, rid, rev))
			}
		}
	}
	compiled, _ := knowledge.Rules(rows)
	for _, r := range compiled {
		out = append(out, ruleOf(r, g, rid, rev))
	}
	return out
}

// ruleSourceTrusted restates internal/intent/intent.go:375's guard (a
// source that is neither SourceImplied nor in range of sp.Sources is
// Untrusted) and :383's set (Trusted, Managed and Builtin keep text) against
// spec's own constants, since engine does not import intent.
func ruleSourceTrusted(sp *spec.Compiled, source int) bool {
	if source != spec.SourceImplied && (source < 0 || source >= len(sp.Sources)) {
		return false
	}
	switch sp.TrustOf(source) {
	case spec.Trusted, spec.Managed, spec.Builtin:
		return true
	}
	return false
}

// ruleOf is a spec rule as the detectors read it, AppliesTo resolved.
func ruleOf(r spec.Rule, g Reader, rid string, rev graph.Rev) Rule {
	er := Rule{ID: r.ID, Kind: r.Kind, From: slices.Clone(r.From), To: slices.Clone(r.To),
		Predicate: r.Predicate, Paths: slices.Clone(r.Paths), Source: r.Source}
	for _, a := range r.AppliesTo {
		repo, local, ok := graph.SplitGlobal(graph.ID(a))
		if !ok {
			er.AppliesTo = append(er.AppliesTo, graph.ID(a))
			continue
		}
		if repo != rid {
			continue
		}
		if _, held := g.Node(local, rev); held {
			er.AppliesTo = append(er.AppliesTo, local)
		}
	}
	return er
}

package main

import (
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// PlanRules is the engine rules a check over the index reads: the spec's
// boundary and protect_path rules from a source whose trust is Trusted,
// Managed or Builtin (an untrusted repository's rule is withheld, a source
// outside the spec's is untrusted), then the rules knowledge.Rules compiles
// from the approved DECLARED rows. AppliesTo is resolved at rev: a global id
// of the repository rid keeps its local id when the node exists at rev; an
// id of another repository, a local-form id and an absent node are dropped,
// never matched by name. Paths carry over. Pure: the caller reads the spec
// (hook.DiscoverSpec) and the rows (knowledge.ReadDir). W-M2.4's steer and
// the J3 judge reuse this one function.
func PlanRules(sp *spec.Compiled, rows []knowledge.Row, s *graph.Store, rid string, rev graph.Rev) []engine.Rule {
	var out []engine.Rule
	if sp != nil {
		for _, d := range sp.Rules {
			k := spec.RuleKind(d.Rule.Kind)
			if (k == spec.Boundary || k == spec.ProtectPath) && planTrusted(sp, d.Source) {
				out = append(out, engineRule(d.Rule, s, rid, rev))
			}
		}
	}
	compiled, _ := knowledge.Rules(rows)
	for _, r := range compiled {
		out = append(out, engineRule(r, s, rid, rev))
	}
	return out
}

// planTrusted is whether a declaration from source may reach a line: the
// set intent.Resolve keeps (Trusted, Managed, Builtin), a source out of
// range untrusted rather than a panic.
func planTrusted(sp *spec.Compiled, source int) bool {
	if source != spec.SourceImplied && (source < 0 || source >= len(sp.Sources)) {
		return false
	}
	switch sp.TrustOf(source) {
	case spec.Trusted, spec.Managed, spec.Builtin:
		return true
	}
	return false
}

func engineRule(r spec.Rule, s *graph.Store, rid string, rev graph.Rev) engine.Rule {
	er := engine.Rule{ID: r.ID, Kind: r.Kind, From: slices.Clone(r.From), To: slices.Clone(r.To),
		Predicate: r.Predicate, Paths: slices.Clone(r.Paths), Source: r.Source}
	for _, a := range r.AppliesTo {
		repo, local, ok := graph.SplitGlobal(graph.ID(a))
		if !ok || repo != rid {
			continue
		}
		if _, held := s.Node(local, rev); held {
			er.AppliesTo = append(er.AppliesTo, local)
		}
	}
	return er
}

# UK-1: spec.UnenforcedRuleKinds names exactly the kinds whose rule in a
# spec file changes no finding, and the readers behind the other kinds stay
# wired. Each break below either lets a rule change no read through the real
# hook entries where a row says it does (TestRuleKinds_EnforcedChangeAFinding,
# TestRuleKinds_KnowledgeDecisionChangesAFinding), or lets a rule change a
# read a limit row pins as unchanged (TestRuleKinds_LimitsChangeNothing).
E = "internal/graph/engine/conflicts.go"
R = "internal/graph/engine/rules.go"
V = "internal/verify/rules.go"
ENF = "TestRuleKinds_EnforcedChangeAFinding"
KNOW = "TestRuleKinds_KnowledgeDecisionChangesAFinding"
LIM = "TestRuleKinds_LimitsChangeNothing"
m("UK-1 the conflict check drops the boundary case", E,
  "\t\tcase spec.Boundary:\n\t\t\tout = append(out, boundaryConflicts(g, d, r)...)\n",
  "\t\tcase spec.Boundary:\n",
  ENF)
m("UK-1 the conflict check drops the decision case", E,
  "\t\tcase spec.Decision:\n\t\t\tout = append(out, decisionConflicts(g, d, r)...)\n",
  "\t\tcase spec.Decision:\n",
  KNOW)
m("UK-1 RulesFor admits a spec file's decision rule", R,
  "\t\t\tif (k == spec.Boundary || k == spec.ProtectPath) && ruleSourceTrusted(sp, d.Source) {\n",
  "\t\t\tif (k == spec.Boundary || k == spec.ProtectPath || k == spec.Decision) && ruleSourceTrusted(sp, d.Source) {\n",
  LIM)
m("UK-1 RulesFor drops a spec file's boundary rule", R,
  "\t\t\tif (k == spec.Boundary || k == spec.ProtectPath) && ruleSourceTrusted(sp, d.Source) {\n",
  "\t\t\tif k == spec.ProtectPath && ruleSourceTrusted(sp, d.Source) {\n",
  ENF)
m("UK-1 RulesFor drops the rules compiled from knowledge rows", R,
  "\tfor _, r := range compiled {\n\t\tout = append(out, ruleOf(r, g, rid, rev))\n",
  "\tfor _, r := range compiled[:0] {\n\t\tout = append(out, ruleOf(r, g, rid, rev))\n",
  KNOW)
m("UK-1 the stored base drops a decision's predicate", "cmd/rashomon/conflictbase.go",
  "\t\tif spec.RuleKind(r.Kind) != spec.Decision || !slices.Contains(spec.RulePredicates, r.Predicate) {\n",
  "\t\tif spec.RuleKind(r.Kind) == spec.Decision || !slices.Contains(spec.RulePredicates, r.Predicate) {\n",
  KNOW)
m("UK-1 symbol_present fires at an overlay revision", "internal/graph/engine/decision.go",
  "\t\tif cur, ok := g.Node(n.ID, d.Rev); !ok || cur.Rev.Kind == graph.RevKindOverlay {\n",
  "\t\tif _, ok := g.Node(n.ID, d.Rev); !ok {\n",
  LIM)
m("UK-1 the Stop judges no forbid_code rule", V,
  "\t\t\tr = codeOver(&dr.Rule, c, rels, at)\n",
  "\t\t\tcontinue\n",
  ENF)
m("UK-1 the Stop judges no require_evidence rule", V,
  "\t\t\tif r, ok = evidenceAt(&dr.Rule, snap, wrote); !ok {\n",
  "\t\t\tif r, ok = evidenceAt(&dr.Rule, snap, wrote); ok || !ok {\n",
  ENF)
m("UK-1 Decision deleted from the list", "internal/spec/kinds.go",
  "var UnenforcedRuleKinds = []RuleKind{Decision, Reuse}\n",
  "var UnenforcedRuleKinds = []RuleKind{Reuse}\n",
  ENF)
m("UK-1 an enforced kind added to the list", "internal/spec/kinds.go",
  "var UnenforcedRuleKinds = []RuleKind{Decision, Reuse}\n",
  "var UnenforcedRuleKinds = []RuleKind{Decision, ForbidCode, Reuse}\n",
  ENF)
m("UK-1 Boundary put back on the list", "internal/spec/kinds.go",
  "var UnenforcedRuleKinds = []RuleKind{Decision, Reuse}\n",
  "var UnenforcedRuleKinds = []RuleKind{Boundary, Decision, Reuse}\n",
  ENF)
m("UK-1 the post checks an edit no boundary or path rule scopes", "internal/graph/engine/conflictbase.go",
  "\t\tcase pathRule(r):\n\t\t\tif slices.Contains(r.AppliesTo, unit) || slices.Contains(r.AppliesTo, file) ||\n",
  "\t\tcase pathRule(r) || spec.RuleKind(r.Kind) == spec.Decision:\n\t\t\tif slices.Contains(r.AppliesTo, unit) || slices.Contains(r.AppliesTo, file) ||\n",
  LIM)

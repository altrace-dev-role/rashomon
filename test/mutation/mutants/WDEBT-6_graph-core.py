# WDEBT-6: the graph core's guards (W-DEBT.6, PLAN.md section 6.1; the
# sweep over internal/graph). The index refuses a unit with no language or
# an unknown one and never walks a dot, node_modules or __pycache__
# directory; the Go mapper keeps a may-call uncertain and never a Calls
# edge, carries implements-via-pointer's via, emits one edge per distinct
# fact, drops an edge into an unkept package, reads a proposed row as
# inferred and an unknown tier as nothing; a store under another schema or
# a header with no repository or commit is refused; an empty repository or
# local id is no global id; a traversal records the shortest distance, walks
# both directions and stops at its depth; a file outside the reader's root,
# a symbol declared there and a table such a symbol touches are out of
# reach. Each break below is one of those promises undone. Not anchored:
# symOf's pkg: refusal, shadowed by the unit lookup (a pkg: id never prefixes
# a synced package with a dot), so a break there has no observable effect.
I = "internal/graph/index/index.go"
m("WDEBT-6 a unit with no lang is accepted", I,
  "\t\tif u.Lang == \"\" {\n\t\t\treturn nil, fmt.Errorf(\"graph: unit %q declares no lang\", u.Path)\n\t\t}\n", "",
  "TestBuild_UnitLangRequired")
m("WDEBT-6 a unit of an unknown lang resolves as Python", I,
  "\treturn nil, fmt.Errorf(\"graph: unit lang %q is not %s or %s\", name, gopg.Lang, pylite.Lang)\n",
  "\treturn python(c), nil\n",
  "TestBuild_UnitLangRequired")
m("WDEBT-6 a dot directory is walked", I,
  "(strings.HasPrefix(d.Name(), \".\") || d.Name() == \"node_modules\" || d.Name() == \"__pycache__\")",
  "(d.Name() == \"node_modules\" || d.Name() == \"__pycache__\")",
  "TestBuild_SkipsDotAndVendorDirs")
m("WDEBT-6 node_modules is walked", I,
  "(strings.HasPrefix(d.Name(), \".\") || d.Name() == \"node_modules\" || d.Name() == \"__pycache__\")",
  "(strings.HasPrefix(d.Name(), \".\") || d.Name() == \"__pycache__\")",
  "TestBuild_SkipsDotAndVendorDirs")
m("WDEBT-6 __pycache__ is walked", I,
  "(strings.HasPrefix(d.Name(), \".\") || d.Name() == \"node_modules\" || d.Name() == \"__pycache__\")",
  "(strings.HasPrefix(d.Name(), \".\") || d.Name() == \"node_modules\")",
  "TestBuild_SkipsDotAndVendorDirs")
G = "internal/graph/lang/gopg/gopg.go"
m("WDEBT-6 a may-call is certain", G,
  "\t\tif pred == graph.MayCall {\n\t\t\treturn graph.Algorithmic, mayCallConf, true\n\t\t}\n", "",
  "TestGopg_TiersPreserved")
m("WDEBT-6 a may-call row is a Calls edge", G,
  "\tcase platform.PredMayCall:\n\t\treturn graph.MayCall, \"\", true\n",
  "\tcase platform.PredMayCall:\n\t\treturn graph.Calls, \"\", true\n",
  "TestGopg_TiersPreserved")
m("WDEBT-6 implements-via-pointer loses its via", G,
  "\tcase platform.PredImplPtr:\n\t\treturn graph.ResolvesTo, \"pointer\", true\n",
  "\tcase platform.PredImplPtr:\n\t\treturn graph.ResolvesTo, \"\", true\n",
  "TestGopg_TiersPreserved")
m("WDEBT-6 an edge into an unkept package is kept", G,
  "\t\tdst, ok := m.symOf(f.Object)\n\t\tif !ok || !m.keepPkg(dst.pkg) {\n\t\t\tcontinue\n\t\t}\n",
  "\t\tdst, ok := m.symOf(f.Object)\n\t\tif !ok {\n\t\t\tcontinue\n\t\t}\n",
  "TestGopg_FilesRestrict")
m("WDEBT-6 a fact seen twice is two edges", G,
  "\t\tif seen[k] {\n\t\t\tcontinue\n\t\t}\n", "",
  "TestGopg_FactsDedupAndTiers")
m("WDEBT-6 a proposed row is proven", G,
  "\tcase platform.TierProposed:\n\t\treturn graph.Inferred, mayCallConf, true\n",
  "\tcase platform.TierProposed:\n\t\treturn graph.Proven, 1.0, true\n",
  "TestGopg_FactsDedupAndTiers")
m("WDEBT-6 an unknown tier is proven", G,
  "\treturn \"\", 0, false\n}\n", "\treturn graph.Proven, 1.0, true\n}\n",
  "TestGopg_FactsDedupAndTiers")
P = "internal/graph/persist.go"
m("WDEBT-6 a store under another schema loads", P,
  "\tif h.Schema != Schema {\n\t\treturn nil, h, fmt.Errorf(\"graph: index schema %d, want %d; run rashomon graph index\", h.Schema, Schema)\n\t}\n", "",
  "TestLoad_RefusesOtherSchema")
IMPORTS["WDEBT-6 a store under another schema loads"] = (P, '\t"fmt"\n', "")
m("WDEBT-6 a header with no repository or commit saves", P,
  "\tif h.Repo == \"\" || h.Rev.Commit == \"\" {\n\t\treturn errors.New(\"graph: the header needs a repo and a rev commit\")\n\t}\n", "",
  "TestHeader_RoundTrip")
m("WDEBT-6 an empty repository or local id is a global id", "internal/graph/global.go",
  "\tif !found || r == \"\" || l == \"\" {\n", "\tif !found {\n",
  "TestGlobalID_RejectsMalformed")
T = "internal/graph/engine/traverse.go"
m("WDEBT-6 a node reached again is re-queued at the longer distance", T,
  "\t\t\t\tif _, ok := seen[e.Dst]; !ok {\n\t\t\t\t\tseen[e.Dst] = d\n", "\t\t\t\tif true {\n\t\t\t\t\tseen[e.Dst] = d\n",
  "TestTraverse_ShortestDistanceBothWaysToDepth")
m("WDEBT-6 the in-edges are not walked", T,
  "\t\t\tfor _, e := range g.In(id, rev, preds...) {\n\t\t\t\tif _, ok := seen[e.Src]; !ok {\n\t\t\t\t\tseen[e.Src] = d\n\t\t\t\t\tnext = append(next, e.Src)\n\t\t\t\t}\n\t\t\t}\n", "",
  "TestTraverse_ShortestDistanceBothWaysToDepth")
m("WDEBT-6 the traversal runs one level past its depth", T,
  "\tfor d := 1; d <= depth && len(frontier) > 0; d++ {\n", "\tfor d := 1; d <= depth+1 && len(frontier) > 0; d++ {\n",
  "TestTraverse_ShortestDistanceBothWaysToDepth")
R = "internal/graph/engine/reach.go"
m("WDEBT-6 a file outside the reader's root is in reach", R,
  "\t\t\treturn strings.HasPrefix(rel(strings.TrimPrefix(s, \"file:\")), \"../\")\n", "\t\t\treturn false\n",
  "TestGR1_OutOfReachCodeRanksAboveWhatTheAgentCanRead")
m("WDEBT-6 a symbol's reach ignores its declaring file", R,
  "\t\t\tfor _, e := range g.In(id, rev, graph.Declares) {\n\t\t\t\treturn outside(e.Src)\n\t\t\t}\n", "",
  "TestGR1_OutOfReachCodeRanksAboveWhatTheAgentCanRead")
m("WDEBT-6 a table's reach ignores the symbols that touch it", R,
  "\t\t\tfor _, e := range g.In(id, rev, graph.Writes, graph.Reads) {\n\t\t\t\tif outside(e.Src) {\n\t\t\t\t\treturn true\n\t\t\t\t}\n\t\t\t}\n", "",
  "TestGR1_OutOfReachCodeRanksAboveWhatTheAgentCanRead")

# GS-1 .. GS-6: the graph brief through the typed channel (GRAPH-SLOT1). A
# line is a slot value the checks screen, the record form holds digests and
# counts, the brief is a task-preflight finding routed before the prompt's
# early return with paths from the hook's cwd, the learn loop reads the
# knowledge it named, and a lesson's own words never seed it. Each break
# below is one of those promises undone.
S = "internal/steer/render/slot/graph.go"
m("GS-1 a line holding two facts is bound", S,
  "\tif err := l.oneFact(); err != nil {\n\t\treturn nil, err\n\t}\n\tm := l.more\n", "\tm := l.more\n",
  "TestGS1_BindLeavesOutAndCountsWhatAChecksRefuses")
m("GS-1 the record form keeps the path", S,
  "\t\tout.digest = digest(string(c.Path))\n\t\tout.code = &CodeFact{Role: c.Role, Lessons: slices.Clone(c.Lessons)}\n",
  "\t\tout.digest = digest(string(c.Path))\n\t\tout.code = &CodeFact{Path: c.Path, Role: c.Role, Lessons: slices.Clone(c.Lessons)}\n",
  "TestGS1_RecordFormHoldsDigestsCountsAndIds")
m("GS-1 a refused name reaches the line", S,
  "\t\tc.Names, m.Names = keptIdents(c.Names, m.Names, maxLineIdents)\n", "\t\tc.Names, m.Names = c.Names, m.Names\n",
  "TestGS1_BindLeavesOutAndCountsWhatAChecksRefuses")
m("GS-3 the slot's lessons are recorded under another key", "internal/steer/slots_graph.go",
  "\tLessons      []string        `json:\"lessons\"`\n", "\tLessons      []string        `json:\"lessons_\"`\n",
  "TestGS3_")
m("GS-4 the delivered set skips the graph lines", "internal/learn/deliver.go",
  "\t\t\tids = append(ids, graphKnowledge(b, sp[dkSlots])...)\n", "", "TestGS4_")
P = "cmd/rashomon/preflight.go"
m("GS-5 the brief is read after the early return", P,
  "\tgb, graphed := graphBrief(storeRoot, p.CWD, p.SessionID, p.PromptID, p.Prompt)\n\tif !named && !proj.lessons.Any() && !done && p.owed.Seq <= 0 && !briefed && !graphed {\n\t\treturn steer.Output{}, steer.Client{}\n\t}\n",
  "\tif !named && !proj.lessons.Any() && !done && p.owed.Seq <= 0 && !briefed {\n\t\treturn steer.Output{}, steer.Client{}\n\t}\n\tgb, graphed := graphBrief(storeRoot, p.CWD, p.SessionID, p.PromptID, p.Prompt)\n",
  "TestGS5_")
m("GS-5 the graph finding is not routed", P,
  "\tif f, ok := graphFinding(gb, run.LastSeq(), digest); graphed && ok {\n",
  "\tif f, ok := graphFinding(gb, run.LastSeq(), digest); false && graphed && ok {\n", "TestGS5_")
m("GS-5 paths print from the index root", "cmd/rashomon/graph.go",
  "\t\tr, err := filepath.Rel(cwd, filepath.Join(root, filepath.FromSlash(p)))\n\t\tif err != nil {\n\t\t\treturn p\n\t\t}\n\t\treturn filepath.ToSlash(r)\n",
  "\t\treturn p\n", "TestGS5_")
m("GS-6 a lesson's words seed it", "internal/graph/engine/scope.go",
  "\t\tif n.Kind == \"mod\" || IsKnowledge(n.Kind) {\n", "\t\tif n.Kind == \"mod\" {\n",
  "TestGS6_AKnowledgeNodeNeverSeeds")
m("GS-6 the declared file target is ignored", "internal/graph/engine/facts.go",
  "\t\tif strings.HasPrefix(string(e.Dst), \"file:\") && (target == anchor || e.Dst < target) {\n", "\t\tif false {\n",
  "TestGS6_EachLineIsASlotRenderedByTheGraphTemplate")
# `_ = i` keeps the loop index in use: the decision was its only reader, so
# dropping the line outright left i declared and unused (BUILD BROKEN, W-M1.9).
m("GS-6 a refused line leaves no decision", "internal/graph/engine/brief.go",
  "\t\t\tb.Decisions = append(b.Decisions, Decision{ID: l.Key, Line: i + 1, Why: \"refused: \" + err.Error()})\n", "\t\t\t_ = i\n",
  "TestGS6_ALineTheSlotRefusesIsDroppedWithADecision")

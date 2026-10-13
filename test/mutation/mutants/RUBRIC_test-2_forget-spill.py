# test-2, conc-1: forget's spill rewrite keeps every line outside what it
# removed, at both sites, and re-reads the spill file under its lock, dropping
# only the lines its plan counted, as a multiset.
m("test-2 forget wipes every kept spill line", "internal/store/gaps.go",
  "\t\tif err := rewriteSpill(spillPath, spillLines, keptSpill); err != nil {\n\t\t\treturn &g, err\n\t\t}\n\t}\n\treturn &g, nil\n}\n\nfunc partition(",
  "\t\tif err := rewriteSpill(spillPath, spillLines, keptSpill[:0]); err != nil {\n\t\t\treturn &g, err\n\t\t}\n\t}\n\treturn &g, nil\n}\n\nfunc partition(",
  "TestRubricTEST2_ForgetWindowKeepsSpillOutsideTheWindow|TestH15_ForgetKeepsSpillOutsideTheWindow")
m("test-2 forget --host wipes every kept spill line", "internal/store/gaps.go",
  "\t\tif err := rewriteSpill(spillPath, spillLines, keptSpill); err != nil {\n\t\t\treturn &g, err\n\t\t}\n\t}\n\treturn &g, nil\n}\n\n// namesHost",
  "\t\tif err := rewriteSpill(spillPath, spillLines, keptSpill[:0]); err != nil {\n\t\t\treturn &g, err\n\t\t}\n\t}\n\treturn &g, nil\n}\n\n// namesHost",
  "TestRubricTEST2_ForgetHostKeepsSpillOfOtherCalls")
m("conc-1 the spill is rewritten from the plan's stale read", "internal/store/gaps.go",
  "\treturn rewrite(f, out)\n", "\treturn rewrite(f, kept)\n", "TestRubricCONC1_")
m("conc-1 a counted line drops every copy of itself", "internal/store/gaps.go",
  "\t\t\tcounted[string(line)]--\n\t\t\tcontinue\n", "\t\t\tcontinue\n", "TestRubricCONC1_SpillRewriteDropsOnlyCountedLines")

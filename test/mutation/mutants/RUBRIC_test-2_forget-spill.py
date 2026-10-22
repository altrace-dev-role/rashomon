# test-2: forget's spill rewrite keeps every line outside what it removed, at
# both sites. Its re-read under the lock is conc-1's (RUBRIC_conc-1_*).
m("test-2 forget wipes every kept spill line", "internal/store/gaps.go",
  "\t\tif err := rewriteSpill(spillPath, spillLines, keptSpill); err != nil {\n\t\t\treturn &g, err\n\t\t}\n\t}\n\treturn &g, nil\n}\n\nfunc partition(",
  "\t\tif err := rewriteSpill(spillPath, spillLines, keptSpill[:0]); err != nil {\n\t\t\treturn &g, err\n\t\t}\n\t}\n\treturn &g, nil\n}\n\nfunc partition(",
  "TestRubricTEST2_ForgetWindowKeepsSpillOutsideTheWindow|TestH15_ForgetKeepsSpillOutsideTheWindow")
m("test-2 forget --host wipes every kept spill line", "internal/store/gaps.go",
  "\t\tif err := rewriteSpill(spillPath, spillLines, keptSpill); err != nil {\n\t\t\treturn &g, err\n\t\t}\n\t}\n\treturn &g, nil\n}\n\n// namesHost",
  "\t\tif err := rewriteSpill(spillPath, spillLines, keptSpill[:0]); err != nil {\n\t\t\treturn &g, err\n\t\t}\n\t}\n\treturn &g, nil\n}\n\n// namesHost",
  "TestRubricTEST2_ForgetHostKeepsSpillOfOtherCalls")

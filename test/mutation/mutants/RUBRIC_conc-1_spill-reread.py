# conc-1: forget re-reads the spill file under its lock and drops only the
# lines its plan counted, as a multiset, so a spill appended after the plan
# survives the rewrite.
m("conc-1 the spill is rewritten from the plan's stale read", "internal/store/gaps.go",
  "\treturn rewrite(f, out)\n", "\treturn rewrite(f, kept)\n", "TestRubricCONC1_")
m("conc-1 a counted line drops every copy of itself", "internal/store/gaps.go",
  "\t\t\tcounted[string(line)]--\n\t\t\tcontinue\n", "\t\t\tcontinue\n", "TestRubricCONC1_SpillRewriteDropsOnlyCountedLines")

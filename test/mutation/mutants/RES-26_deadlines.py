# RES-26: every entry's soft deadline comes from the table, and the one
# leftover phase (`probe start`'s cost finalisation) is capped and never
# negative. Each break below removes one of those guarantees.
m("RES-26 an entry past the table reads its own soft budget as nonzero",
  "internal/budget/deadlines.go",
  "\tif e >= deadlineEntryCount {\n\t\treturn 0\n\t}\n",
  "\tif e > deadlineEntryCount {\n\t\treturn 0\n\t}\n", "TestRES26_")
m("RES-26 the cost-scan leftover is never capped at 50ms",
  "internal/budget/deadlines.go",
  "\tif left > probeStartCostScanCap {\n\t\treturn probeStartCostScanCap\n\t}\n",
  "", "TestRES26_")
m("RES-26 the cost-scan leftover goes negative once the five phases ahead overspend",
  "internal/budget/deadlines.go",
  "\tif left < 0 {\n\t\treturn 0\n\t}\n",
  "", "TestRES26_")
m("RES-26 probe start's five fixed phases run out of order",
  "internal/budget/deadlines.go",
  "\treturn []Phase{Labels, Mining, Evict, Compact, Briefing}\n",
  "\treturn []Phase{Mining, Labels, Evict, Compact, Briefing}\n", "TestRES26_")
m("RES-26 ProbeStartFixedBudget has no guard against a Phase past the end",
  "internal/budget/deadlines.go",
  "\tif p >= phaseCount {\n\t\treturn 0\n\t}\n",
  "", "TestRES26_ProbeStartFixedBudgetGuardsAPhasePastTheEnd")

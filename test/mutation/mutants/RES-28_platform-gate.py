# RES-28: an entry or case gates only with a reading of its own -- the
# reference platform's row, or an override measured on the platform under
# test -- and otherwise reports, naming why, UNLESS the platform has a
# baseline of its own that simply does not cover this id, which fails closed
# instead (RES-28s: a measurement debt is not a brand-new platform). Each
# break below removes one of those branches; entryBudget and caseBudget
# share the same shape.
m("RES-28 entryBudget ignores its own platform override", "test/perf/gate.go",
  '\tif o, ok := overrides[e.ID]; ok {\n\t\treturn o.P50, true, false, ""\n\t}\n', "", "TestRES28q_")
m("RES-28 entryBudget gates every platform against the reference row", "test/perf/gate.go",
  "\tif plat == referencePlatform {\n\t\treturn e.MaxRSSMiBP50, true, false, \"\"\n\t}\n",
  "\t{\n\t\treturn e.MaxRSSMiBP50, true, false, \"\"\n\t}\n", "TestRES28q_")
m("RES-28 entryBudget passes an unmeasured id on a platform that has a baseline", "test/perf/gate.go",
  "\tif len(overrides) == 0 {\n\t\treturn e.MaxRSSMiBP50, false, true, fmt.Sprintf(\n",
  "\tif true {\n\t\treturn e.MaxRSSMiBP50, false, true, fmt.Sprintf(\n", "TestRES28q_")
m("RES-28 caseBudget ignores its own platform override", "test/perf/gate.go",
  '\tif v, ok := overrides[c.ID]; ok {\n\t\treturn v, true, false, ""\n\t}\n', "", "TestRES28r_")
m("RES-28 caseBudget gates every platform against RES-28's line", "test/perf/gate.go",
  "\tif plat == referencePlatform {\n\t\treturn c.BudgetMiB, true, false, \"\"\n\t}\n",
  "\t{\n\t\treturn c.BudgetMiB, true, false, \"\"\n\t}\n", "TestRES28r_")
m("RES-28 caseBudget passes an unmeasured case on a platform that has a baseline", "test/perf/gate.go",
  "\tif len(overrides) == 0 {\n\t\treturn c.BudgetMiB, false, true, fmt.Sprintf(\n",
  "\tif true {\n\t\treturn c.BudgetMiB, false, true, fmt.Sprintf(\n", "TestRES28r_")

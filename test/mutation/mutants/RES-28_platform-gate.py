# RES-28: an entry or case gates only with a reading of its own -- the
# reference platform's row, or an override measured on the platform under
# test -- and otherwise reports, naming why. Each break below removes one of
# those branches; entryBudget and caseBudget share the same shape.
m("RES-28 entryBudget ignores its own platform override", "test/perf/gate.go",
  '\tif o, ok := overrides[e.ID]; ok {\n\t\treturn o.P50, true, ""\n\t}\n', "", "TestRES28q_")
m("RES-28 entryBudget gates every platform against the reference row", "test/perf/gate.go",
  "\tif plat == referencePlatform {\n\t\treturn e.MaxRSSMiBP50, true, \"\"\n\t}\n",
  "\t{\n\t\treturn e.MaxRSSMiBP50, true, \"\"\n\t}\n", "TestRES28q_")
m("RES-28 entryBudget never names the reaper's own floor", "test/perf/gate.go",
  '"this entry\'s own peak does not exceed the reaper\'s own floor on %s, so no reading here can be attributed to it yet"',
  '"no reading of its own on %s yet"', "TestRES28q_")
m("RES-28 caseBudget ignores its own platform override", "test/perf/gate.go",
  '\tif v, ok := overrides[c.ID]; ok {\n\t\treturn v, true, ""\n\t}\n', "", "TestRES28r_")
m("RES-28 caseBudget gates every platform against RES-28's line", "test/perf/gate.go",
  "\tif plat == referencePlatform {\n\t\treturn c.BudgetMiB, true, \"\"\n\t}\n",
  "\t{\n\t\treturn c.BudgetMiB, true, \"\"\n\t}\n", "TestRES28r_")

# RES-28: the case set is the spec's. Moving a small case to P-10 skips it,
# dropping one stops gating it, and narrowing CaseGates skips a family, and
# each still passes every comparison that remains.
m("RES-28 the 400 B hook case moves to P-10", "test/perf/budgets.json",
  '"payload_bytes": 400, "budget_mib": 6.5008, "gates_from": "P-08"', '"payload_bytes": 400, "budget_mib": 6.5008, "gates_from": "P-10"', "TestRES28j_")
m("RES-28 the 40 KB post case leaves budgets.json", "test/perf/budgets.json",
  '    {"id": "post.40KB", "entry": "post", "payload_bytes": 40000, "budget_mib": 6.5839, "gates_from": "P-08"},\n', '', "TestRES28j_")
m("RES-28 only hook cases gate", "test/perf/runs.go",
  'func CaseGates(c budget.RSSCase) bool { return c.GatesFrom == "P-08" }', 'func CaseGates(c budget.RSSCase) bool { return c.GatesFrom == "P-08" && c.Entry == "hook" }', "TestRES28j_")

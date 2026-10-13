# RES-28: peak RSS follows the payload budget. The gate's comparison, its
# tolerance schedule and its exact-size payloads are what make the measured
# figure mean something; loosening any of them is the whole regression.
m("RES-28 the comparison lets ten times the budget pass", "test/perf/gate.go",
  "\treturn measured > budget*(1+tolerance)\n", "\treturn measured > budget*(1+tolerance)*10\n", "TestRES28c_")
m("RES-28 a zero budget reads as unlimited", "test/perf/gate.go",
  "\tif budget <= 0 {\n\t\treturn true\n\t}\n", "\tif budget <= 0 {\n\t\treturn false\n\t}\n", "TestRES28c_")
m("RES-28 an unparseable tolerance date keeps the looser tolerance", "test/perf/gate.go",
  "\tif err != nil {\n\t\treturn tight\n\t}\n", "\tif err != nil {\n\t\treturn tolerance\n\t}\n", "TestRES28d_")
m("RES-28 a payload one byte short passes as exact", "test/perf/payload/payload.go",
  "\tif len(out) != total {\n", "\tif len(out) != total && len(out) != total-1 {\n", "TestRES28f_")
m("RES-28 a red on the tolerance switch date reads as a plain regression", "test/perf/gate.go",
  '\t\treturn "tolerance tightened per budgets.json tolerance_until " + until + " (" + percent(tolerance) + " to " + percent(after) + ")"\n',
  '\t\treturn ""\n', "TestRES28g_")

m("RES-28 a line is padded with the tolerance before the measurement is compared", "test/perf/gate.go",
  "\tif measuredMiB > lineMiB {\n", "\tif measuredMiB > lineMiB*1.1 {\n", "TestRES28h_")
m("RES-28 a case with no measured figure beside its line passes", "test/perf/gate.go",
  "\tif measuredMiB <= 0 {\n", "\tif measuredMiB < 0 {\n", "TestRES28h_")

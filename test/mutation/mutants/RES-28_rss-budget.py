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
# RES-28: every entry row's p50 is a median of nine measured on the reference
# machine; a row that goes back to a figure carried from the archive is the
# regression.
m("RES-28 an entry row's p50 goes back to the archive's derivation", "test/perf/budgets.json",
  '"basis": "measured 2026-10-13 on the reference machine (go1.27.2, release flags; TestRES28b_ on a pinned, installed store; remeasured on the P-10 tree, bounded intake over group commit): median of nine, the middle of three runs (5.95 / 6.05 / 5.98 MiB); closing a started session; expected per archive 85.4.1: one record, 6 / 6.5 MiB"',
  '"basis": "derived from archive 85.4.1: one record"', "TestRES28b_")
# RES-28: a basis that still waits on a PR this tree carries contradicts the
# measured figure beside it.
m("RES-28 post.edit_314k's basis goes back to waiting on P-10", "test/perf/budgets.json",
  '"basis": "measured 2026-10-13 on the reference machine (go1.27.2, release flags; TestRES28b_ on a pinned, installed store; remeasured on the P-10 tree, bounded intake over group commit): median of nine, the middle of three runs (6.73 / 6.67 / 6.73 MiB); p95 is RES-28\'s line at 314 KB (7.16 MiB), which the archive applies from P-10 in place of 8 / 10 MiB; 60 runs gave p95 6.92, max 6.95"',
  '"basis": "measured 2026-10-13 on the reference machine (go1.27.2, release flags; TestRES28b_ on a pinned, installed store; remeasured on the P-10 tree, bounded intake over group commit): median of nine, the middle of three runs (6.73 / 6.67 / 6.73 MiB); expected per archive 85.4.1: 8 / 10 MiB until P-10, then per RES-28 (7.16 MiB at 314 KB)"',
  "TestRES28b_")

# RH-6: a masked test run whose outcome was read from its output (the
# runner's summary line, or the status its line printed) is counted passed or
# failed by that read, only where its call ended ok, and the test-run line
# says where each outcome was read, the wrapped runs among the exit status's
# named. Each row is killed by a subtest of TestRH6_PipedRunReadFromOutputCounts.

B = "internal/report/testbending.go"
T = "internal/report/text.go"
P = "TestRH6_"
m("RH-6 a masked run's summary line is not read", B,
  "\t\t\t\tcase readFromSummary(rec):\n\t\t\t\t\tr = out.ReadFromSummary\n",
  "", P)
m("RH-6 a masked run whose call ended failed is read from its output", B,
  "\t\t\t\tcase o != store.ExecOK:\n",
  "", P)
m("RH-6 the breakdown credits the exit status with the wrapped runs", T,
  "\t\tif t.Wrapped > 0 {\n",
  "\t\tif false && t.Wrapped > 0 {\n", P)
m("RH-6 the test runs line says neither when every masked run was read", T,
  "*t.Masked > read {", "*t.Masked > 0 {", P)
m("RH-6 the test runs line counts a read masked run as neither too", T,
  "*t.Masked-read)", "*t.Masked)", P)

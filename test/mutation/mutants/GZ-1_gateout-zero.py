# GATEOUT-ZERO1 (R29): pytest's fail forms are its summary line, with a count
# of at least 1. A delegated run is matched against every table, so a bare
# count read another tool's "0 failed" or "0 errors", or a test's own log
# line, as a failure. Every row is killed by the named GZ1 test.
T = "internal/gateout/tables.go"
Q = "`^(\\d+ \\w+, )*[1-9]\\d* (failed|errors?)(, [^=]*)? in \\d`"
B = "`^=+ .*[1-9]\\d* (failed|errors?).* in \\d[\\d.]*s.* =+$`"

m("GZ-1 pytest's fail form is unanchored", T,
  Q + ", " + B, "`\\d+ (failed|error)`",
  "TestGZ1_AZeroCountOrALogLineIsNotAFailure|TestGZ1b_")
m("GZ-1 a zero count fails in the -q form", T,
  Q, "`^(\\d+ \\w+, )*\\d+ (failed|errors?)(, [^=]*)? in \\d`",
  "TestGZ1_AZeroCountOrALogLineIsNotAFailure")
m("GZ-1 the -q fail form is dropped", T,
  "fail: compile(" + Q + ", ", "fail: compile(",
  "TestGZ1_PytestSummariesStillFail")
m("GZ-1 the banner fail form is dropped", T,
  ", " + B + ", `Fatal", ", `Fatal",
  "TestGZ1_PytestSummariesStillFail")

# The tables are pinned per shape.RulesVersion (gateout_test.go tablePins): a
# table change with no new version fails, and so does a bump with no pin.
m("GZ-1 a table changes with no new RulesVersion", T,
  "pass: compile(`^test result: ok`)", "pass: compile(`^test result: ok\\b`)",
  "TestGO10_TablesPinned")
m("GZ-1 RulesVersion moves with no table pin", "internal/shape/masked.go",
  "const RulesVersion = 12\n", "const RulesVersion = 13\n",
  "TestGO10_TablesPinned")

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

# H-175: the regression-location note names only writes whose call ended ok.
# A shell line that failed and may have written stands between the green and
# the red as no write, so the red raises no note (regressionLocations' cost).
m("H-175 a failed shell write is a write between the runs", "internal/verify/held.go",
  "\treturn EndFailedCall\n}", "\treturn EndOK\n}", "TestH175_")

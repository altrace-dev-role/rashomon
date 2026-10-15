# RES-29: statusMasked re-runs the whole parsed line once per distinctly-
# worded runner group, so a line of thousands of them is quadratic. Lifting
# the cap, or the comparison letting one group past it, each let a line this
# shaped back into the loop that reads the line once per group.
m("RES-29 the runner-group cap is lifted", "internal/shape/masked.go",
  "const maxRunnerGroups = 64\n", "const maxRunnerGroups = 1 << 30\n", "TestStatusMaskedRunnerGroupsAreCapped")
m("RES-29 the runner-group cap is off by one", "internal/shape/masked.go",
  "\tif len(p.runners) > maxRunnerGroups {\n", "\tif len(p.runners) > maxRunnerGroups+1 {\n", "TestStatusMaskedRunnerGroupsAreCapped")

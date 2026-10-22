# RES-36: the gate fails at more than 10% over, and that ten percent is the
# only margin. A looser constant leaves every row as it was and still passes
# a 21% regression, so the constant itself is pinned.
m("RES-36 the bench gate's tolerance loosens to 21%", "scripts/benchgate/main.go",
  "const tolerance = 0.10\n", "const tolerance = 0.21\n", "TestRES36e_")

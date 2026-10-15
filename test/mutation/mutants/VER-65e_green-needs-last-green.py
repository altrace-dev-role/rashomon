# VER-65e: a Green gate with no LastGreen is a defect (a run that never
# happened), not a pass at seq 0. Evaluate itself never builds that shape,
# but TurnSummary must not read it as one anyway (VER 11).
m("VER-65e green with no last_green stops counting as a defect", "internal/verify/evaluate.go",
  "\t\tif g.State == Green && g.LastGreen == nil {\n\t\t\tdefect = true\n\t\t\tcontinue\n\t\t}\n",
  "", "TestVER65d_")

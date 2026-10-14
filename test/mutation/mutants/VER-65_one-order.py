# VER-65: one ordered state over the turn gates, read by TurnSummary and the
# claim comparator alike (VER 11, decided (a)). Ranking a gate's own unknown
# before red, a consumer growing its own order, or a stale gate reading as a
# pass is the whole regression.
m("VER-65 a gate's own unknown ranks before red again", "internal/verify/state.go",
  "var Order = []State{Red, MaskedInconclusive, Backgrounded, Stale, Absent, Unknown, Green}",
  "var Order = []State{Unknown, Red, MaskedInconclusive, Backgrounded, Stale, Absent, Green}", "TestVER65b_")
m("VER-65 TurnSummary stops reading the one order", "internal/verify/evaluate.go",
  "\tworst, ok := Worst(states)\n",
  "\tworst, ok := State(\"\"), len(states) > 0\n\tif ok {\n\t\tworst = states[0]\n\t}\n", "TestVER65b_")
m("VER-65 a stale gate reads as a pass in the value line", "internal/verify/evaluate.go",
  "\tcase Stale, Absent:\n\t\treturn RowUnverified\n",
  "\tcase Stale, Absent:\n\t\treturn RowFindings\n", "TestVER65b_")
m("VER-65 the unnamed-write rule reads the latest green instead of the earliest", "internal/verify/evaluate.go",
  "\t\tif earliestGreen < 0 || green < earliestGreen {\n", "\t\tif earliestGreen < 0 || green > earliestGreen {\n", "TestVER65c_")

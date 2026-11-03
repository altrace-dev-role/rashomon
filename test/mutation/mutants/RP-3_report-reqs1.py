# REPORT-REQS1: the session report's count line of mistakes against intent.
# Each finding line, delivered or withheld, lands in one list by the first
# rule that applies, and whether the agent was told is read from delivery,
# never from the finding's state. Each row drops one rule, or prints a later
# find nothing records as 0.
MI = "internal/recap/mistakes.go"
TX = "internal/report/text_autosteer.go"
m("RP-3 a withheld finding resolved by a green counts as caught", MI,
  "\t\tcase f.told && f.State == FindingResolved:\n", "\t\tcase f.State == FindingResolved:\n", "TestRP3b_|TestRP3_")
m("RP-3 a line delivered at a Stop counts as caught once resolved", MI,
  "\t\tcase f.told && atStop(f.Event):\n", "\t\tcase false:\n", "TestRP3b_|TestRP3_")
m("RP-3 an open finding no printed clause counted reached the person", MI,
  "\t\tcase f.told && carried:\n", "\t\tcase f.told:\n", "TestRP3b_")
m("RP-3 a finding naming no gate is counted as missed or reached", MI,
  "\t\tcase f.State == FindingUnchecked:\n", "\t\tcase false:\n", "TestRP3b_|TestRP3_")
m("RP-3 a withheld line reads as told", MI,
  "\t\tf.told = delivered && (told[id] || atStop(f.Event))\n", "\t\tf.told = delivered || true\n", "TestRP3_")
m("RP-3 a later find nothing records prints as 0", TX,
  "\tif m.Later == nil {\n", "\tif false {\n", "TestRP3d_|TestRP3_")
m("RP-3 a caught finding falls through to fixed as well", MI,
  "\t\t\tm.Caught = append(m.Caught, f.FindingRef)\n", "\t\t\tm.Caught = append(m.Caught, f.FindingRef)\n\t\t\tfallthrough\n",
  "TestRP3b_|TestRP3_")
m("RP-3 an open flag to the person counts as missed", MI,
  "\t\tcase f.flagged:\n", "\t\tcase false:\n", "TestRP3b_")
m("RP-3 a held-out line reads as flagged to the person", MI,
  "\t\tf.flagged = delivered && !f.told && text(b, sp[lkUserText]) != \"\"\n", "\t\tf.flagged = delivered && !f.told\n", "TestRP3_")

# TL-1: the Stop's turn rows (W-M3.1, internal/verify/ledger_turn.go) and the
# turn they are read in. Q4 (ruling 2026-11-12 14:54): with no prompt id
# named, the turn at a point is read from the declarations at or before the
# point, so a Stop over the whole record reads the ledger its prefix reads
# (INV-11). The break drops that bound: latestPrompt reads every declaration,
# and an unnamed Stop at a seq inside an earlier turn resolves to the
# record's last turn. Each killer replays one reader of the bound: the
# ledger (TurnWindow and turnProjects through EvaluateStop), countTurn, and
# TurnWindow beside countTurn.
m("TL-1 the current turn of an unnamed prompt is read past the point", "internal/verify/stop.go",
  "\t\tif d.Seq > at || d.PromptID == nil || *d.PromptID == \"\" {\n",
  "\t\tif d.PromptID == nil || *d.PromptID == \"\" {\n",
  "TestLedger_Replay$|TestCountTurn_EmptyIDReadsTheTurnAtThePoint|TestTurnWindow_EmptyPromptIDResolvesAsCountTurn")

# TL-1: the Stop's turn rows (W-M3.1, internal/verify/ledger_turn.go) and the
# turn they are read in. Under card W-M3.1, Q4 (ruling 2026-11-12 14:54),
# with no prompt id named, the turn at a point is read from the declarations
# at or before the point, so a Stop over the whole record reads the ledger
# its prefix reads (INV-11). The break drops that bound: latestPrompt reads
# every declaration, and an unnamed Stop at a seq inside an earlier turn
# resolves to the record's last turn. Each killer replays one reader of the
# bound: the
# ledger (TurnWindow and turnProjects through EvaluateStop), countTurn, and
# TurnWindow beside countTurn.
m("TL-1 the current turn of an unnamed prompt is read past the point", "internal/verify/stop.go",
  "\t\tif d.Seq > at || d.PromptID == nil || *d.PromptID == \"\" {\n",
  "\t\tif d.PromptID == nil || *d.PromptID == \"\" {\n",
  "TestLedger_Replay$|TestCountTurn_EmptyIDReadsTheTurnAtThePoint|TestTurnWindow_EmptyPromptIDResolvesAsCountTurn")

# The turn ledger's own guards (ruling 2026-11-12 15:21, 2b): each break is
# shown red under its killer, beside a green baseline, before it counts.
m("TL-1 a write before the turn's From is read as the turn's", "internal/verify/ledger_turn.go",
  "\t\tin := x.Seq >= w.From\n",
  "\t\tin := true\n",
  "TestLedger_TurnWindowOwesOnlyItsWrites")
m("TL-1 a red in an earlier turn owes the turn a watched row", "internal/verify/ledger_turn.go",
  "\t\tif r, ok := reds[g.Family]; ok && r.red.Seq >= w.From {\n",
  "\t\tif r, ok := reds[g.Family]; ok {\n",
  "TestLedger_RedInAnEarlierTurnOwesNoWatchedRow")
m("TL-1 every unit row reads the first unit's writes", "internal/verify/ledger_turn.go",
  "unitView(g, byUnit[u], c.unnamed",
  "unitView(g, byUnit[units[0]], c.unnamed",
  "TestLedger_UnitRowReadsItsOwnWrites")
m("TL-1 the unit-row limit never binds", "internal/verify/ledger_turn.go",
  "\t\t\tcase kept == MaxTurnUnitRows:\n",
  "\t\t\tcase kept < 0:\n",
  "TestLedger_UnitRowCapBindsAndCounts")
m("TL-1 a unit row past the limit is dropped uncounted", "internal/verify/ledger_turn.go",
  "\t\t\t\tcut++\n",
  "",
  "TestLedger_UnitRowCapBindsAndCounts")
m("TL-1 turn rows keep their table where the session rows read unknown", "internal/verify/ledger_turn.go",
  "\t\tif snap.Recording != Complete || Rank(g.State) < 0 || !known {\n",
  "\t\tif false && !known {\n",
  "TestLedger_TurnRowsReadUnknownWhereTheSessionRowsDo")
m("TL-1 a point that is not a Stop owes turn rows", "internal/verify/ledger_turn.go",
  "\tif snap.Point.Kind != PointStop {\n",
  "\tif false {\n",
  "TestLedger_SubtaskPointAddsNoTurnRows")
m("TL-1 a unit row takes its gate row's id", "internal/verify/ledger_turn.go",
  "TurnObligationID(o.Kind, idFamily(g), w, unit), unit",
  "TurnObligationID(o.Kind, idFamily(g), w, \"\"), unit",
  "TestLedger_UnitRowPerUnitWritten|TestLedger_UnitRowCapBindsAndCounts")
m("TL-1 the ledger verb names no turn row's family", "cmd/rashomon/report_ledger.go",
  "\t\tif o.Scope == verify.ScopeTurn {\n",
  "\t\tif false {\n",
  "TestReportLedger_NamesATurnRowsFamily")

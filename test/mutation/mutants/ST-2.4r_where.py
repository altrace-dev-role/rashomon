# Card 2.4r: every row clause says when the run ran. One chooser, when(),
# places a gate's last run against the turn's last edit and its first seq, and
# serves five sites: red, masked, backgrounded, unknown outcome, and the
# no-write empty or partial run. Each comparison, its boundary, the
# earlier-turn branch, red's suffix, countTurn's first seq and each site's
# call are broken once. A boundary (>= to >) is killed only by its own row:
# the tie with the last write, and the turn's first call.
VL = "internal/recap/value.go"
VS = "internal/verify/stop.go"

m("ST-2.4r before and after flipped", VL,
  '\tcase v.Turn.Writes > 0 && seq >= v.Turn.LastWriteSeq:\n',
  '\tcase v.Turn.Writes > 0 && seq < v.Turn.LastWriteSeq:\n', "TestVER65_ValueLineRowsRender|TestDONE_RedBeforeTheLastEditSaysSo")
m("ST-2.4r a tie with the last write reads as before it", VL,
  '\tcase v.Turn.Writes > 0 && seq >= v.Turn.LastWriteSeq:\n',
  '\tcase v.Turn.Writes > 0 && seq > v.Turn.LastWriteSeq:\n', "TestVER65_ValueLineRowsRender")
m("ST-2.4r the earlier-turn branch removed", VL,
  '\tcase seq >= v.Turn.FirstSeq:\n', '\tcase seq >= 0:\n', "TestDONE_RedFromAnEarlierTurnSaysSo")
m("ST-2.4r the turn's first call reads as an earlier turn's", VL,
  '\tcase seq >= v.Turn.FirstSeq:\n', '\tcase seq > v.Turn.FirstSeq:\n', "TestVER65_ValueLineRowsRender")
m("ST-2.4r red's has-not-run-since dropped", VL,
  '\t\tif since {\n\t\t\ts += " and has not run since"\n',
  '\t\tif false && since {\n\t\t\ts += " and has not run since"\n', "TestDONE_RedBeforeTheLastEditSaysSo|TestDONE_RedFromAnEarlierTurnSaysSo")
m("ST-2.4r countTurn takes the window's last seq for its first", VS,
  '\t\t\tif t.FirstSeq == 0 || d.Seq < t.FirstSeq {\n',
  '\t\t\tif t.FirstSeq == 0 || d.Seq > t.FirstSeq {\n', "TestVER67_CountTurnMatchesDigestsWindow")

# Each extra site back on its old clause.
m("ST-2.4r the masked site keeps after the last edit", VL,
  '\t\t\twhere, _ := when(v, g)\n\t\t\treturn t.name(g) + " ran" + where + "; its exit status',
  '\t\t\twhere := " after the last edit"\n\t\t\treturn t.name(g) + " ran" + where + "; its exit status', "TestVER65_ValueLineRowsRender")
m("ST-2.4r the backgrounded site keeps after the last edit", VL,
  '\t\t\twhere, _ := when(v, g)\n\t\t\treturn t.name(g) + " moved to the background"',
  '\t\t\twhere := " after the last edit"\n\t\t\treturn t.name(g) + " moved to the background"', "TestVER65_ValueLineRowsRender")
m("ST-2.4r the unknown-outcome site keeps after the last edit", VL,
  '\t\twhere, _ := when(v, g)\n\t\treturn t.name(g) + " ran" + where + " (call "',
  '\t\twhere := " after the last edit"\n\t\treturn t.name(g) + " ran" + where + " (call "', "TestVER65_ValueLineRowsRender")
m("ST-2.4r the empty-run site keeps in this turn", VL,
  '\t\twhere, _ := when(v, g)\n\t\tswitch g.Reason {\n',
  '\t\twhere := " in this turn"\n\t\tswitch g.Reason {\n', "TestVER65_ValueLineRowsRender")

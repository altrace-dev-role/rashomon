# Card 2.3c: selectTurn copies the chosen turn only. Pass one finds the turn
# from each prompt's first seq and declaration count, copying nothing; pass
# two copies that turn's declarations once, into a slice of their number.
# The minSeq update and the sort are equivalent on every read run, whose
# declarations arrive in seq order, so TestTURN_SelectTurnCopiesOnlyTheChosenTurn
# builds its run in memory and out of order (R4, Gate A A6).
T = "internal/digest/turn.go"
J = "TestTURN_SelectTurnCopiesOnlyTheChosenTurn|TestSelectTurn"

# Pass two (R1): one turn, presized.
m("ST-2.3c pass two collects every prompt", T,
  "\t\tif d.PromptID != nil && *d.PromptID == promptID {\n",
  "\t\tif d.PromptID != nil {\n", J)
m("ST-2.3c pass two's slice is not presized", T,
  "\tdecls := make([]store.Declaration, 0, g.count)\n",
  "\tdecls := make([]store.Declaration, 0)\n", J)
m("ST-2.3c pass one never counts a declaration", T,
  "\t\tg.count++\n", "", J)

# Which turn and its order (R4): separated only by the out-of-order run.
m("ST-2.3c a prompt's first seq is its first listed", T,
  "\t\tif d.Seq < g.minSeq {\n\t\t\tg.minSeq, g.startMS = d.Seq, d.RecordedAtMS\n\t\t}\n", "", J)
m("ST-2.3c the turn's declarations keep their listed order", T,
  "\tsort.SliceStable(decls, func(i, j int) bool { return decls[i].Seq < decls[j].Seq })\n", "", J)

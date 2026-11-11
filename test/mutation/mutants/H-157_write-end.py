# H-157 to H-173 and WEND (the base reporting fix): a write is an edit only
# where the record holds its call's execution and it ended ok. One break per
# behaviour: the classification of each ending, the readers that must leave
# a held write out (the turn's counts, the row's guard, the line, the rules,
# the platform row, the regression note) and the one that must keep it in
# (store_touched, D135).
C = "internal/verify/held.go"
E = "internal/verify/evaluate.go"
S = "internal/verify/stop.go"
V = "internal/recap/unconfirmed.go"

ITEMS = "TestH157_|TestH158_|TestH159_|TestH160_|TestH161_|TestH167_"
m("H-157 a failed file tool is an edit", C,
  "\tcase e.Outcome == store.ExecOK:\n\t\treturn EndOK\n",
  "\tcase e.Outcome == store.ExecOK || e.Outcome == store.ExecFailed && fileWrite(d):\n\t\treturn EndOK\n",
  ITEMS + "|TestWEND1_|TestWEND2_|TestWEND8_")
m("H-163 a call with no ending is an edit", C,
  "\tcase e == nil:\n\t\treturn EndNone\n", "\tcase e == nil:\n\t\treturn EndOK\n",
  "TestH163_|TestH164_|TestWEND1_|TestWEND2_")
m("H-166 a failed shell write is an edit", C,
  "\treturn EndFailedCall\n}", "\treturn EndOK\n}", "TestH166_|TestWEND1_|TestWEND8_")
m("WEND-1 a partial list held with no unnamed entry", C,
  "\tif named && unnamed {\n", "\tif false {\n", "TestWEND1_")
m("H-158 the row's guard reads no call after the green", E,
  "\tcase unconfirmed > 0:\n\t\trow = RowUnconfirmedWrite\n", "",
  "TestH158_|TestH159_|TestH163_|TestH166_|TestWEND4_")
m("WEND-4 the ran-only row reads no failed call after the green", E,
  "\tcase turn.Writes == 0 && unconfirmed > 0:\n\t\trow = RowUnconfirmedWrite\n", "", "TestWEND4_")
m("H-165 the guard reads a failed edit before the green", E,
  "\tif seq == 0 && t.FailedEditSeq > green {\n", "\tif seq == 0 && t.FailedEditSeq > 0 {\n", "TestH165_|TestWEND4_")
m("WEND-4 a call with no ending bears only after the green", E,
  "\tif t.UnendedWriteSeq > seq {\n", "\tif t.UnendedWriteSeq > max(seq, green) {\n", "TestWEND4_")
m("H-162 an incomplete turn that may have written reads none", E,
  " || snap.Recording != Complete && !held) {\n", " || snap.Recording != Complete && (true || !held)) {\n", "TestH162_|TestWEND4_")
m("WEND-3 a held write bears wherever the gates cover nothing", S,
  "!(Relevant(w.Class) && coveredByAny(roots, &w.write))", "!Relevant(w.Class)", "TestWEND3_")
m("WEND-3 a failed call reads as one with no ending", S,
  "\t\tcase w.End == EndNone:\n", "\t\tcase true:\n", "TestH158_|TestWEND3_")
m("WEND-5 a failed edit is named as a call that may have written", V,
  '\t\treturn call + ", an edit, failed"\n', '\t\treturn call + " failed and may have written files"\n',
  "TestH158_|TestWEND5_")

# The review's findings: each break restores what the reviewed fix printed.
m("H-168 a call is placed by the latest green", V,
  "\tcase call > latest:\n", "\tcase call > min(latest, green):\n", "TestH168_|TestWEND5_|TestWEND9_")
m("WEND-4 the row carries no green it placed the call by", E,
  "\tif row == RowUnconfirmedWrite || row == RowUnnamedWrite {\n\t\ts.GreenSeq = earliestGreen\n\t}\n", "",
  "TestH158_|TestWEND4_")
m("H-169 record green reads the gates alone", E,
  "\tif ok && slices.ContainsFunc(snap.Gates,", "\tif false && slices.ContainsFunc(snap.Gates,", "TestH169_|TestWEND10_")
m("H-169 a failed edit after the green holds record green", E,
  "\t\t\tif w.End.mayHaveWritten() && w.Path != nil &&", "\t\t\tif w.End != EndOK && w.Path != nil &&", "TestH169_|TestWEND10_")
m("WEND-10 an earlier turn's held call reads as a finding", E,
  "return g.offTurn || g.heldSince > 0 })", "return g.offTurn })", "TestWEND10_")
m("H-170 ran_only reads a call that may have written only after the green", E,
  "\tif t.FailedWriteSeq > green || ranOnly && t.FailedWriteSeq > 0 {\n", "\tif t.FailedWriteSeq > green {\n",
  "TestH170_|TestWEND4_")
m("H-171 a turn that may have written and ran no gate prints nothing", E,
  "snap.Recording == Complete && !turn.mayHaveWritten() ||", "snap.Recording == Complete ||",
  "TestH169_|TestH171_|TestWEND4_")
m("H-171 the gates not run after the call go unnamed", V,
  '\treturn joinNames(names) + " not run after it"\n', '\treturn "no test or build run after it"\n',
  "TestH171_|TestWEND5_")
m("H-172 a failed edit and a failed call share one count", S,
  "\t\tcase w.End == EndFailedEdit:\n\t\t\tt.FailedEditSeq = max(t.FailedEditSeq, w.Seq)\n", "",
  "TestH172_|TestWEND3_")
m("H-172 a later failed edit is named before a call that may have written", E,
  "\tif seq == 0 && t.FailedEditSeq > green {\n\t\tseq, end = t.FailedEditSeq, EndFailedEdit\n",
  "\tif t.FailedEditSeq > max(seq, green) {\n\t\tseq, end = t.FailedEditSeq, EndFailedEdit\n", "TestWEND4_")
m("H-173 an interrupt reads as a failure", C,
  "\tcase e.Outcome == store.ExecInterrupted:\n\t\treturn EndInterrupted\n", "", "TestH173_|TestWEND1_")
m("H-173 an interrupted call is said to have failed", V,
  '\t\treturn call + " was interrupted and may have written files"\n', '\t\treturn call + " failed and may have written files"\n',
  "TestH173_|TestWEND5_")
m("SR-1b a failed edit of a protected path is said to have written it", "internal/verify/rules_raise.go",
  "\t\tif event == eventPostFailure {\n\t\t\tunnamed = render.IntegrityFrozenPathTargetedUnnamedBinding{}\n",
  "\t\tif false {\n\t\t\tunnamed = render.IntegrityFrozenPathTargetedUnnamedBinding{}\n", "TestSR1b_|TestSR1x_")
m("WEND-6 protect_path reads no held write", "internal/verify/rules.go",
  " || heldBlocks(c.held, r.Paths, r.ExcludePaths, r.Modes, 0, at+1)\n", "\n", "TestWEND6_")
m("WEND-6 a failed file tool may have written a protected path", C,
  "w.Seq >= to || w.End == EndFailedEdit || strings", "w.Seq >= to || strings", "TestWEND6_")
m("WEND-6 an order rule's invalidated_by reads no held write", "internal/verify/rules_order.go",
  "\tif heldBlocks(c.held, r.InvalidatedBy, nil, nil, from, min(to, at+1)) {\n", "\tif false {\n", "TestWEND6_")
m("WEND-7 store_touched reads the edits alone", "internal/verify/integrity.go",
  "if h := &held[i]; h.Path != nil && h.Seq > from", "if h := &held[i]; false && h.Path != nil && h.Seq > from", "TestWEND7_")
m("PLW1 a call that may have written after the cover is passed over", "internal/verify/platform.go",
  " ||\n\t\tmayHaveTouched(p.c.held, f, cov.Seq, at):\n", ":\n", "TestPLW1m_")

# The second review (R2-1): a row that places a run after the last edit names
# a call that may have written after that run. Each break drops the clause
# from one row, or names it where the run's placement does not need it.
VL = "internal/recap/value.go"
m("H-174 the red row names no call after its run", VL,
  '\t\ts += t.heldPast(v, g, where, " after that")\n', "", "TestH174_|TestWEND11_")
m("WEND-11 the unknown row names no call after its run", VL,
  '"); its result is not in the record" + t.heldPast(v, g, where, " after that run")\n',
  '"); its result is not in the record"\n', "TestWEND11_")
m("WEND-11 the masked row names no call after a pipe hid its run", VL,
  ' hid its result (call " + seqOf(g) + ")" + t.heldPast(v, g, where, " after that")\n',
  ' hid its result (call " + seqOf(g) + ")"\n', "TestWEND11_")
m("WEND-11 the masked row names no call after its run", VL,
  ' hidden by its line (call " + seqOf(g) + ")" + t.heldPast(v, g, where, " after that")\n',
  ' hidden by its line (call " + seqOf(g) + ")"\n', "TestWEND11_")
m("WEND-11 the backgrounded row names no call after its run", VL,
  'where + "; its result is not in the record" + t.heldPast(v, g, where, " after that run")\n',
  'where + "; its result is not in the record"\n', "TestWEND11_")
m("WEND-11 the unverified row's greens name no call after them", VL,
  '\theld := t.wroteAfter(v, t.earliestGreen(), "")\n', '\theld := ""\n', "TestWEND11_")
m("WEND-11 the call is named wherever the run was placed", V,
  "\tif where != afterLastEdit || g.LastRun == nil {\n", "\tif g.LastRun == nil {\n", "TestWEND11_")
m("WEND-11 a call with no ending is placed after the run", V,
  '\tcase end == verify.EndNone:\n\t\tafter = ""\n', "", "TestWEND11_")
m("WEND-11 the call is placed after the latest green", V,
  " && g.LastGreen.Seq < call && ", " && ", "TestWEND11_")
m("WEND-11 the greens are read from the latest", V,
  "(at == 0 || g.LastGreen.Seq < at)", "(at == 0 || g.LastGreen.Seq > at)", "TestWEND11_")
m("H-174 a failed edit after the run is named", E,
  "if seq, end := t.unconfirmed(run, false); end != EndFailedEdit {", "if seq, end := t.unconfirmed(run, false); true {",
  "TestH174_|TestWEND11_")

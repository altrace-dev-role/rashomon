# VER-1.4: whether the record is complete at a point, and why not
# (internal/verify/recording.go). One break per condition, per predicate term,
# per cut in time and per note; the kill row is the named test's. No fuzz
# target: the input is the builder's records, not untrusted bytes.
R = "internal/verify/recording.go"

# skipped_lines.
m("VER-1.4 skipped lines are not a gap", R,
  '\tfault[noteSkipped] = run.Skipped > 0\n', '\tfault[noteSkipped] = false\n', "TestVER31_")
m("VER-1.4 an ignored record is a skipped line", R, 'run.Skipped > 0', 'run.Skipped+run.Ignored > 0', "TestVER31_")

# declaration_without_outcome.
m("VER-1.4 a terminal is not an outcome", R,
  '\t\tdeclared := answer(ids, t.ToolUseID, t.Seq)\n', '\t\t_, declared := ids[t.ToolUseID]\n', "TestVER32_")
# answer marks the declaration answered as a side effect, so skipping the call
# for an execution after the point leaves its declaration unanswered (B145).
m("VER-1.4 an outcome after the point is missing", R,
  '\t\tdeclared := answer(ids, e.ToolUseID, e.Seq)\n',
  '\t\tif e.Seq != nil && *e.Seq > at {\n\t\t\tcontinue\n\t\t}\n\t\tdeclared := answer(ids, e.ToolUseID, e.Seq)\n',
  "TestVER32b_")
m("VER-1.4 a declaration is due at every point", R, 's.due = s.due || d.Seq <= at', 's.due = true', "TestVER32_")
m("VER-1.4 an outcome is due before its declaration", R, 's.due && !s.answered', '!s.answered', "TestVER32_")

# outcome_without_declaration (D35).
m("VER-1.4 the declaration map is ignored", R, '\t\tids[id] = s\n\t}\n\treturn ok\n', '\t\tids[id] = s\n\t}\n\treturn true\n', "TestVER32c_")
m("VER-1.4 an orphan execution is not a gap", R,
  '\t\tif !declared && byPoint(e.Seq, at) {\n', '\t\tif !declared && false {\n', "TestVER32c_")
m("VER-1.4 an orphan terminal is not a gap", R,
  '\t\tif !declared && byPoint(t.Seq, at) {\n', '\t\tif !declared && false {\n', "TestVER32c_")
m("VER-1.4 the point does not cut an orphan", R, 'return seq == nil || *seq <= at }', 'return true }', "TestVER32c_")
m("VER-1.4 a spilled orphan is placed", R, 'return seq == nil || *seq <= at }', 'return seq != nil && *seq <= at }', "TestVER32c_")
m("VER-1.4 the point does not cut an orphan execution", R,
  '\t\tif !declared && byPoint(e.Seq, at) {\n', '\t\tif !declared {\n', "TestVER32c_")
m("VER-1.4 the point does not cut an orphan terminal", R,
  '\t\tif !declared && byPoint(t.Seq, at) {\n', '\t\tif !declared {\n', "TestVER32c_")

# spilled_outcome (D33 item 4).
m("VER-1.4 a nil Seq is not a gap", R,
  's.answered, s.spilled = true, s.spilled || seq == nil', 's.answered = true', "TestVER33_")
m("VER-1.4 a spilled outcome is due before its declaration", R, 's.due && s.spilled', 's.spilled', "TestVER33_")
m("VER-1.4 the spilled gap is the last declaration's", R,
  'fault[noteSpilled] = fault[noteSpilled] || s.due && s.spilled', 'fault[noteSpilled] = s.due && s.spilled', "TestVER36_")

# coverage_unverified and coverage_absent.
m("VER-1.4 the State is ignored", R,
  'c.State == store.StateVerified && c.Reason == nil', 'c.Reason == nil', "TestVER34_")
m("VER-1.4 a zero State is verified", R,
  'c.State == store.StateVerified && c.Reason == nil', 'c.State != store.StateUnverified && c.Reason == nil', "TestVER34_")
m("VER-1.4 a verified record with a reason is verified", R,
  'c.State == store.StateVerified && c.Reason == nil', 'c.State == store.StateVerified', "TestVER34_")
m("VER-1.4 an unknown probe is ignored", R, ' || c.Probe == store.ProbeUnknown', '', "TestVER34_")
m("VER-1.4 an unknown hook entry is ignored", R, 'c.HookEntry == store.EntryUnknown || ', '', "TestVER34_")
m("VER-1.4 an unknown is outweighed by a verified record", R, '(!verified || unknown)', '!verified', "TestVER34_")
m("VER-1.4 a start record is a post record", R,
  '\t\tif c.Phase != store.PhasePost {\n\t\t\tcontinue\n\t\t}\n', '', "TestVER34_")
m("VER-1.4 coverage absent is unverified too", R,
  'len(run.Coverage) > 0 && (!verified || unknown)', '(!verified || unknown)', "TestVER34_")
m("VER-1.4 absent and unverified are swapped", R,
  '\tfault[noteUnverified] = len(run.Coverage) > 0 && (!verified || unknown)\n\tfault[noteAbsent] = len(run.Coverage) == 0\n',
  '\tfault[noteAbsent] = len(run.Coverage) > 0 && (!verified || unknown)\n\tfault[noteUnverified] = len(run.Coverage) == 0\n',
  "TestVER34_")

# The notes.
m("VER-1.4 two conditions' notes are swapped", R, '\tnoteOrphan\n\tnoteSpilled\n', '\tnoteSpilled\n\tnoteOrphan\n', "TestVER32c_")
m("VER-1.4 the notes are given in reverse", R,
  '\tfor i, f := range fault {\n\t\tif f {\n', '\tfor i := len(fault) - 1; i >= 0; i-- {\n\t\tif f := fault[i]; f {\n', "TestVER35_")
m("VER-1.4 a note is misspelt", R, '\t"spilled_outcome",\n', '\t"spilled_outcomes",\n', "TestVER35_")
m("VER-1.4 a complete record has non-nil notes", R, '\t\treturn Complete, nil\n', '\t\treturn Complete, out\n', "TestVER30_")
m("VER-1.4 an incomplete record reads complete", R,
  '\treturn Incomplete, slices.Clone(out)\n', '\treturn Complete, slices.Clone(out)\n', "TestVER31_")

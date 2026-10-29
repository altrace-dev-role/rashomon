# VER-1.5: Evaluate, the fold (internal/verify/evaluate.go). One break per
# predicate: the cut, the silence of an incomplete record, every list's
# non-nil form, RecordGreen and its seq, Required and the index bootstrap,
# TurnSummary's advisory row (D38), ProjectRoot and the spec's identity.
# The kill row is the named test's. No fuzz target: the input is the
# builder's records, not untrusted bytes.
E = "internal/verify/evaluate.go"

# The cut.
m("VER-1.5 the cut is off by one", E, '\tat := pt.Seq\n', '\tat := pt.Seq + 1\n', "TestRES14b_")
m("VER-1.5 the gates' cut alone is off by one", E,
  '\tsnap.Gates = gateStates(runs, writes, unnamed, at, pt.Tree)\n', '\tsnap.Gates = gateStates(runs, writes, unnamed, at+1, pt.Tree)\n', "TestRES14b_")
m("VER-1.5 Seq 0 counts the first call", E, '\tat := pt.Seq\n', '\tat := max(pt.Seq, 2)\n', "TestVER42_")
m("VER-1.5 a nil run is not an empty one", E, '\tif run == nil {\n\t\trun = &store.Run{}\n\t}\n', '', "TestVER18b_")

# Silence: an incomplete record says nothing, and what the turn is still travels.
m("VER-1.5 an incomplete record still speaks", E,
  '\tif snap.Recording != Complete {\n\t\treturn snap\n\t}\n', '', "TestVER41_")
m("VER-1.5 the spec is dropped under Incomplete", E,
  '\tif snap.Recording != Complete {\n\t\treturn snap\n\t}\n',
  '\tif snap.Recording != Complete {\n\t\tsnap.Spec = nil\n\t\treturn snap\n\t}\n', "TestVER41_")
m("VER-1.5 ProjectRoot is blank under Incomplete", E,
  '\tif snap.Recording != Complete {\n\t\treturn snap\n\t}\n',
  '\tif snap.Recording != Complete {\n\t\tsnap.ProjectRoot = ""\n\t\treturn snap\n\t}\n', "TestVER41_")
m("VER-1.5 the stub's note is kept", E,
  '\tsnap.Recording, snap.Notes = recording(run, at)\n',
  '\tsnap.Recording, snap.Notes = recording(run, at)\n\tsnap.Notes = append(snap.Notes, "evaluator not built (P-41): recording reported incomplete")\n',
  "TestVER41_")

# Every list non-nil.
m("VER-1.5 Notes is nil on a complete record", E, '\tif snap.Notes == nil {\n\t\tsnap.Notes = []string{}\n\t}\n', '', "TestVER45_")
m("VER-1.5 Gates is nil on an incomplete record", E, '\t\tGates:           []GateState{},\n', '', "TestVER45_")
m("VER-1.5 Claims is nil", E, '\t\tClaims:          []ClaimVerdict{},\n', '', "TestVER45_")
m("VER-1.5 SpecVerdicts is nil", E, '\t\tSpecVerdicts:    []RuleVerdict{},\n', '', "TestVER45_")
m("VER-1.5 Unresolved is nil", E, '\t\tIntegrity:       Integrity{Unresolved: []IntegrityEntry{}},\n', '', "TestVER45_")

# RecordGreen (D36) and its seq.
m("VER-1.5 RecordGreen with no gate", E, '\tif n == 0 {\n\t\treturn false, nil\n\t}\n', '', "TestVER42_")
m("VER-1.5 RecordGreen with a required gate not green", E,
  '\t\tif g.State != Green || g.LastGreen == nil {\n', '\t\tif g.LastGreen == nil {\n', "TestVER40_")
m("VER-1.5 a green gate with no green run is a green record", E,
  '\t\tif g.State != Green || g.LastGreen == nil {\n', '\t\tif g.State != Green {\n', "TestVER40_")
m("VER-1.5 RecordGreenAtSeq is the smallest", E,
  '\t\tif n == 0 || g.LastGreen.Seq > at {\n', '\t\tif n == 0 || g.LastGreen.Seq < at {\n', "TestVER40_")

# Required: the index's word on a learned project, every gate otherwise (D38).
m("VER-1.5 a gate not required blocks", E, '\t\tif !g.Required {\n\t\t\tcontinue\n\t\t}\n', '', "TestVER46_")
m("VER-1.5 no index requires nothing", E,
  '\tif idx == nil {\n\t\treturn false\n\t}\n', '\tif idx == nil {\n\t\treturn true\n\t}\n', "TestVER46_")
m("VER-1.5 the clock in seconds", E, '\tnowMS := now.UnixMilli()\n', '\tnowMS := now.Unix()\n', "TestVER46_")
m("VER-1.5 the index is never read as learned", E,
  '\tlearned := indexLearned(idx, nowMS)\n', '\tlearned := false\n', "TestVER46_")
m("VER-1.5 the bootstrap reads the snapshot's gates", E,
  '\tlearned := indexLearned(idx, nowMS)\n',
  '\tlearned := idx != nil && slices.ContainsFunc(snap.Gates, func(g GateState) bool { return idx.Required(g.Family, nowMS) })\n',
  "TestVER46_")
m("VER-1.5 an index that requires nothing reads as learned", E,
  '\t\t\treturn true\n\t\t}\n\t}\n\treturn false\n}\n', '\t\t\treturn true\n\t\t}\n\t}\n\treturn true\n}\n', "TestVER46_")

# The row with advisory gates only (D38): capped green, red kept, learned gates alone.
# The advisory test lives in Advisory since 2.4, which TurnSummary and the
# value line's gate list both call.
m("VER-1.5 every row is advisory", E,
  '\treturn !slices.ContainsFunc(gates, func(g GateState) bool { return g.Required })\n',
  '\treturn true\n', "TestVER65c_|TestVER46_")
m("VER-1.5 advisory gates never reach the row", E,
  '\treturn !slices.ContainsFunc(gates, func(g GateState) bool { return g.Required })\n',
  '\treturn false\n', "TestVER46_")
# The advisory line is evaluate.go's one use of slices.
for name in ("VER-1.5 every row is advisory", "VER-1.5 advisory gates never reach the row"):
    IMPORTS[name] = (E, '\t"slices"\n\t"strconv"\n', '\t"strconv"\n')
m("VER-1.5 the advisory set takes every non-required gate", E,
  '\treturn g.Required || advisory && g.Source == sourceLearned\n', '\treturn g.Required || advisory\n', "TestVER65c_")
m("VER-1.5 an advisory green reads verified", E,
  '\t\trow = RowUnverified // the cap', '\t\trow = RowVerified // the cap', "TestVER46_")
m("VER-1.5 an advisory green with no write is unverified", E,
  '\tcase turn.Writes == 0:\n\t\trow = RowRanOnly\n', '\tcase turn.Writes == 0 && !advisory:\n\t\trow = RowRanOnly\n', "TestVER46_")
m("VER-1.5 an advisory red is hidden as unverified", E,
  '\tcase ok && worst != Green:\n', '\tcase ok && worst != Green && !advisory:\n', "TestVER46_")
m("VER-1.5 the advisory sentence is dropped", E,
  '\tcase advisory:\n\t\ts.Sentence = sentenceAdvisoryOnly\n', '\tcase advisory:\n', "TestVER46_")

# ProjectRoot: the nearest named project at or before the point, in no slice order.
m("VER-1.5 ProjectRoot by slice order", E, ' && (root == nil || d.Seq > seq) {\n', ' && (root == nil || d.Seq != seq) {\n', "TestVER44_")
m("VER-1.5 ProjectRoot is the first", E, ' && (root == nil || d.Seq > seq) {\n', ' && (root == nil || d.Seq < seq) {\n', "TestVER47_")
m("VER-1.5 ProjectRoot is the last declaration even when nil", E,
  '\t\tif d.Seq <= at && d.Project != nil && (', '\t\tif d.Seq <= at && (', "TestVER47_")
m("VER-1.5 ProjectRoot reads past the point", E,
  '\t\tif d.Seq <= at && d.Project != nil && (', '\t\tif d.Project != nil && (', "TestVER47_")

# The spec's identity.
m("VER-1.5 the spec path is the first source's", E,
  '\t\tinfo.Path = sp.Sources[n-1].Path\n', '\t\tinfo.Path = sp.Sources[0].Path\n', "TestVER41_")

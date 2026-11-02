# ARCH-INLINE1: after a Go edit the post names the other packages the
# platform graph shows calling into the edited file's package. The read
# (internal/platform/callers.go) counts PROVEN calls whose object is the
# package's and whose subject is another package's, a package initialiser's
# call among them, from fresh callers only, under the walk file's pair.
C = "internal/platform/callers.go"
PREDICATE = "\t\tif f.predicate != predCalls || f.tier != tierProven {\n"

m("AI-1 every tier counts", C,
  PREDICATE, "\t\tif f.predicate != predCalls {\n", "TestAI1_CallersAreProvenCallsFromFreshPackages")
m("AI-1 a may-call counts", C,
  PREDICATE, "\t\tif f.predicate != predCalls && f.predicate != predMayCall || f.tier != tierProven {\n",
  "TestAI1_CallersAreProvenCallsFromFreshPackages")
m("AI-1 the edited package is its own caller", C,
  'sub != "" && sub != site.Package', 'sub != ""', "TestAI1_CallersAreProvenCallsFromFreshPackages")
m("AI-1 a caller need not be fresh", C,
  "\t\tif !fresh(site.Root, dg, p, nil, &hashed) {\n", "\t\tif !fresh(site.Root, dg, p, nil, &hashed) && false {\n",
  "TestAI1_CallersAreProvenCallsFromFreshPackages|TestAI1_ACallerEditedSinceTheSyncIsNotNamed")
m("AI-1 a package initialiser's call is not counted", C,
  "\t\t\treturn p\n", "\t\t\treturn \"\"\n", "TestAI1_CallersAreProvenCallsFromFreshPackages")
m("AI-1 objectIn reads the subject", C,
  "return func(_, object []byte) bool {", "return func(object, _ []byte) bool {",
  "TestAI1_CallersAreProvenCallsFromFreshPackages")
m("AI-1 the log is not checked against the walk file", C,
  "if err != nil || !v.on || v.log != site.Log {", "if err != nil || !v.on {",
  "TestAI1_ASyncBetweenLocateAndReadIsNotVouched")
m("AI-1 the digests are not checked against the walk file", C,
  "\tif dg.sum != v.digests {\n", "\tif false {\n", "TestAI1_ASyncBetweenLocateAndReadIsNotVouched")
m("AI-1 Locate takes a test file", C,
  ' || strings.HasSuffix(file, "_test.go")', "", "TestAI1_LocateNamesNoSiteWhereNoSyncVouches")
m("AI-1 Locate needs no walk file", C,
  "\tif err != nil || !v.on {\n\t\treturn Site{}, false\n", "\tif err != nil && false {\n\t\treturn Site{}, false\n",
  "TestAI1_LocateNamesNoSiteWhereNoSyncVouches")

# The hook (internal/hook/blastradius.go): record scope v5 only; a context of
# its own where the judge built none; two gates (A2), the read marker taken
# before the read and the print gate on the caller set's digest; silence on
# no caller; a yielded note returns its marker, and only a delivered (or
# arm_off) line counts as given.
H = "internal/hook/blastradius.go"

m("AI-1 a v4 record names directories", H,
  "!p.scopeV5 || ", "", "TestAI1_SilenceWhereNothingIsProven")
m("AI-1 no context of its own", H,
  "\tif !ok {\n\t\t*ctx = p.contextOf(", "\tif !ok && false {\n\t\t*ctx = p.contextOf(", "TestAI1_AGoEditTellsTheCallersOnce")
m("AI-1 no read gate", H,
  "\tif !p.st.TakeBlastRead(p.sessionID, marker) {\n\t\treturn blastDraft{}, false\n\t}\n",
  "\tp.st.TakeBlastRead(p.sessionID, marker)\n", "TestAI1_AGoEditTellsTheCallersOnce")
m("AI-1 no print gate", H,
  "if lastGiven(lines, site.Package) == digest {", "if lastGiven(lines, site.Package) == digest && false {",
  "TestAI1_AReSyncSaysOnlyANewCallerSet")
m("AI-1 no caller still speaks", H,
  "if err != nil || len(callers) == 0 {", "if err != nil {", "TestAI1_SilenceWhereNothingIsProven")
m("AI-1 a yielded note keeps its marker", H,
  "\tp.st.ReturnBlastRead(p.sessionID, d.marker)\n", "", "TestAI1_AYieldedNoteComesWithTheNextEdit")
m("AI-1 a suppressed line counts as given", H,
  "if state != string(steer.LineStateDelivered) && by != string(steer.SuppressedByArmOff) {",
  "if state != string(steer.LineStateDelivered) && by != string(steer.SuppressedByArmOff) && false {",
  "TestAI1_AYieldedNoteComesWithTheNextEdit")

# The read marker (internal/store/blastread.go): taken once, O_EXCL, and
# removed on return. purge and disclose list it.
S = "internal/store/blastread.go"

m("AI-1 the read marker is taken twice", S,
  "os.O_CREATE|os.O_EXCL|os.O_WRONLY", "os.O_CREATE|os.O_WRONLY", "TestAI1_TakeBlastReadIsOncePerNameUntilReturned")
m("AI-1 a returned marker stays", S,
  "\t\t_ = os.Remove(filepath.Join(s.RunDir(sessionID), name))\n", "",
  "TestAI1_TakeBlastReadIsOncePerNameUntilReturned")
m("AI-1 purge leaves the read markers", "internal/purge/purge.go",
  " || store.IsBlastReadFile(name)", "", "TestTRU28_PurgeListsEveryNameItsWritersExport")
m("AI-1 the inventory lacks the read markers", "internal/disclose/disclose.go",
  "\t\t\tblastReadFiles(),\n", "", "TestTRU23_EveryWrittenPathIsInTheInventory")

# The id (A6, A8): priority 25 under every correction, and a reserve of one
# note for the corrections, blast-radius's alone (F4).
m("AI-1 no reserve", "internal/steer/decide.go",
  "case have.notes+pattern.Reserve(p.f.Pattern) >= maxNotes:", "case have.notes >= maxNotes:",
  "TestAI1_BlastRadiusLeavesTheLastNoteToACorrection")
m("AI-1 every id reserves a note", "internal/steer/pattern/pattern.go",
  "\tif id == BlastRadius {\n\t\treturn 1\n", "\tif true {\n\t\treturn 1\n", "TestAI1_WithNoBlastRadiusTheCapIsAsBefore")
m("AI-1 blast-radius outranks a correction", "internal/steer/pattern/pattern.go",
  "\t\t\tBlastRadius:        25,\n", "\t\t\tBlastRadius:        90,\n", "TestAI1_InOneCallTheCorrectionWins")
m("AI-1 the count is no lower bound", "internal/steer/render/render.go",
  '"(at least {count}): {dirs}; test files', '"({count}): {dirs}; test files',
  "TestAI1_BlastRadiusRowsSayALowerBoundAndNoOrder")

# After an edit of a file that a reader other than the port's claims, the
# post names the callers of the function whose body holds the edit, from the
# calls edges of the repository graph. The read (internal/hook/blastcallers.go)
# routes by the registry's claim, reads only under a consent scope the
# overlay writer's gate found, places the edit in the file as it was left,
# reads the index once per function and index in one bounded pass, and ends
# in one named case. The note (internal/hook/blastcallersnote.go) lists up to
# five callers by name and counts the rest; its record holds the counts alone.
C = "internal/hook/blastcallers.go"
N = "internal/hook/blastcallersnote.go"

m("PY-X the port takes every other language's files", C,
  "\tif claimant.Lang() == env.Blast.Lang {\n", "\tif claimant.Lang() != env.Blast.Lang {\n",
  "TestBlastCallers_RouteIsTheClaimingReaderNotTheSuffix")
m("PY-X no caller reads as a note", C,
  "\tcase len(callers) == 0:\n\t\treturn callersRead{Case: callersZero, Func: fn}\n", "",
  "TestBlastCallers_ZeroCallersPrintsNoNote")
m("PY-X the file the index read reads as changed", C,
  "\tcase digest != graph.ContentDigest(content):\n", "\tcase digest == graph.ContentDigest(content):\n",
  "TestBlastCallers_StaleIndexPrintsTheChangedClause")
m("PY-X a file the header names refused is read", C,
  "\t\tif r.Path == rel {\n", "\t\tif r.Path == rel && false {\n",
  "TestBlastCallers_RefusedFileIsNamedAndSilent")
m("PY-X a refused configuration is read past", C,
  "\treader, claims, err := reg.Claim(idxRoot, rel)\n\tif err != nil {\n",
  "\treader, claims, err := reg.Claim(idxRoot, rel)\n\tif err != nil && false {\n",
  "TestBlastCallers_RefusedFileIsNamedAndSilent")
m("PY-X a language the index does not hold is read", C,
  "hdr.Producers); un {\n", "hdr.Producers); un && false {\n",
  "TestBlastCallers_UnindexedLanguageIsSilent")
m("PY-X a new_string the file holds twice is placed", C,
  "if off < 0 || bytes.Index(content[off+1:], n) >= 0 {", "if off < 0 {",
  "TestBlastCallers_UnplacedEditIsSilent")
m("PY-X a replace_all is placed", C,
  "\t\tif e.ReplaceAll || e.New == \"\" {\n", "\t\tif e.New == \"\" {\n",
  "TestBlastCallers_UnplacedEditIsSilent")
m("PY-X a body runs to the end of the file", C,
  "if err != nil || l < def || l > end {", "if err != nil || l < def || end < 0 {",
  "TestBlastCallers_ModuleLevelEditBelowAFunctionIsSilent|TestBlastCallers_ClassBodyOutsideEveryMethodIsSilent")
m("PY-X the read marker leaves out the record count", C,
  "m.Write([]byte(string(fn) + \"\\n\" + commit + \"\\n\" + strconv.Itoa(records)))",
  "m.Write([]byte(string(fn) + \"\\n\" + commit))",
  "TestBlastCallers_OnePassPerFunctionAndCommit")
m("PY-X a note without consent", C,
  "ctx.ConfigUnreadable || !ctx.Consent || ctx.PromptID == nil", "ctx.ConfigUnreadable || ctx.PromptID == nil",
  "TestBlastCallers_NoNoteWithoutConsent")
m("PY-X a MultiEdit is read at its last edit in a body", C,
  "inBody {\n\t\t\tbreak\n", "inBody {\n\t\t\tcontinue\n",
  "TestBlastCallers_MultiEditNamesTheFirstEditInsideABody")
m("PY-X a scope below the readers' is read", C,
  "from > 0 && !p.gatePassed(from) {", "from > 0 && false {",
  "TestBlastCallers_AScopeBelowTheReadersReadsNothing")
m("PY-X the readers' own scope reads nothing", C,
  "\treturn gf >= from\n", "\treturn gf > from\n",
  "TestBlastCallers_TheReadersScopeReadsAndAsksNothingMore")
m("PY-X a function calling itself is its caller", C,
  "ok && src != fn {", "ok {",
  "TestBlastCallers_AFunctionCallingItselfIsNotItsCaller")
m("PY-X any line past the bound refuses the pass", C,
  "\t\tif errors.Is(err, bufio.ErrBufferFull) {\n\t\t\tline, err = nil, skipLong(",
  "\t\tif false {\n\t\t\tline, err = nil, skipLong(",
  "TestBlastCallers_ALongLineOfAnotherRecordIsPassedOver")
m("PY-X a long node of the edited file is passed over", C,
  "bytes.HasPrefix(first, head) || bytes.Contains(first, needle)", "bytes.Contains(first, needle)",
  "TestBlastCallers_ALongNodeOfTheEditedFileRefusesThePass")
m("PY-X the first piece of a long line is not searched", C,
  "bytes.HasPrefix(first, head) || bytes.Contains(first, needle)", "bytes.HasPrefix(first, head)",
  "TestBlastCallers_ALongCallsEdgeRefusesThePass|TestBlastCallers_ANeedleAcrossThePieceBoundaryRefuses")
m("PY-X a later piece is searched only across its start", C,
  ", needle) || bytes.Contains(piece, needle) {", ", needle) {",
  "TestBlastCallers_ALongCallsEdgeRefusesThePass|TestBlastCallers_ANeedleAcrossThePieceBoundaryRefuses")
m("PY-X a needle split between two pieces is missed", C,
  "bytes.Contains(append(carry, piece[:min(keep, len(piece))]...), needle) || ", "",
  "TestBlastCallers_ANeedleAcrossThePieceBoundaryRefuses")
m("PY-X a note lists six callers", N,
  "const maxCallersListed = 5\n", "const maxCallersListed = 6\n",
  "TestCallersMessage_ListsFiveAndCountsTheRest|TestBlastCallers_ManyCallersCapAtFive")
m("PY-X a note that does not fit drops its first name", N,
  "names[:len(names)-1], listed[:len(listed)-1], more+1", "names[1:], listed[1:], more+1",
  "TestCallersMessage_DropsTheLastNameUntilTheNoteFits")
m("PY-X a caller that gives no name is not counted", N,
  "\t\tcase !named:\n\t\t\tmore++\n", "\t\tcase !named:\n",
  "TestCallersMessage_ANameThatFailsIsCountedNotListed|TestCallersMessage_OneNameThatDoesNotFitSaysNothing")
m("PY-X a name the identifier slot refuses is listed", N,
  "\treturn n, n.Validate() == nil\n", "\treturn n, true\n",
  "TestCallersMessage_ANameThatFailsIsCountedNotListed")
m("PY-X the record holds the note's names", N,
  "\treturn m, rec, listed, true\n", "\treturn m, m, listed, true\n",
  "TestCallersMessage_ListsFiveAndCountsTheRest|TestBlastCallers_SteerPrintsTheNoteAndRecordsCountsOnly")
# The port's language is platform.Lang, held equal to gopg.Lang by its test.
m("PY-X the port serves a language the fact log does not record", "internal/platform/platform.go",
  "const Lang = \"go\"\n", "const Lang = \"gox\"\n",
  "TestLang_EqualsGopgLang")
m("PY-X a call with no port wired is read", C,
  "env.Blast.Locate == nil || env.Blast.Callers == nil || ", "",
  "TestBlastCallers_NoPortWiredReadsNothing")
m("PY-X a slot a steer line refuses is written", "internal/steer/slots.go",
  "\t\tcase slot.GraphLine:\n\t\t\terr = putJSON(put, s.Name, graphLineForm(v))\n\t\tdefault:\n",
  "\t\tcase slot.GraphLine:\n\t\t\terr = putJSON(put, s.Name, graphLineForm(v))\n\t\tcase slot.IdentList:\n\t\t\terr = putJSON(put, s.Name, v)\n\t\tdefault:\n",
  "TestSlotsOf_EverySlotTypeHasARow")

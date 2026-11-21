# After an edit of a file that a reader other than the port's claims, the
# post names the callers of the function whose body holds the edit, from the
# calls edges of the repository graph. The read (internal/hook/blastcallers.go)
# routes by the registry's claim, places the edit in the file as it was left,
# reads the index once per function and index, and ends in one named case.
C = "internal/hook/blastcallers.go"

m("PY-X the port takes every other language's files", C,
  "\tif claimant.Lang() == env.Blast.Lang {\n", "\tif claimant.Lang() != env.Blast.Lang {\n",
  "TestBlastCallers_RouteIsTheClaimingReaderNotTheSuffix")
m("PY-X a note names every caller", C,
  "\treturn c[:maxCallersNamed], len(c) - maxCallersNamed\n", "\treturn c, 0\n",
  "TestBlastCallers_ManyCallersCapAtFive")
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

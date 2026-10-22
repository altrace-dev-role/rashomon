# Rubric hld-4: Build is a read step (internal/report/gather) and then a pure
# Assemble. Each break below is one way the two halves stop meeting: the
# pure half ignoring what it was handed, or the read step not reading it.
m("RUBRIC hld-4 the accounting ignores the read step's set", "internal/report/report.go",
  "\tif read.Err != nil || read.Set == nil {\n", "\tif true {\n",
  "TestRubricHLD4_AssembleTakesWhatTheReadStepRead")
m("RUBRIC hld-4 a read with no set is taken as readable", "internal/report/report.go",
  "\tif read.Err != nil || read.Set == nil {\n", "\tif read.Err != nil {\n\t\treturn t\n\t}\n\tif read.Set == nil {\n\t\tread.Set = NewTranscriptSet()\n\t}\n\tif false {\n",
  "TestRubricHLD4_AMissingTranscriptIsAbsentAndAnUnreadOneIsUnreadable")
m("RUBRIC hld-4 the account ignores the read step's final message", "internal/report/account.go",
  "\t\tif !read.FinalRead {\n", "\t\tif true {\n",
  "TestRubricHLD4_AssembleTakesWhatTheReadStepRead|TestRubricHLD4_BuildReadsTheTranscriptsTheRunNames")
m("RUBRIC hld-4 the destinations drop the baseline's answer", "internal/report/destinations.go",
  "\t\tNovelty:               novelty,\n", "\t\tNovelty:               Novelty{},\n",
  "TestRubricHLD4_AssembleTakesWhatTheReadStepRead")
m("RUBRIC hld-4 a transcript file is not counted", "internal/report/transcript.go",
  "\tset.Files++\n", "",
  "TestRubricHLD4_TranscriptIDsReadsOneReader|TestRubricHLD4_AssembleTakesWhatTheReadStepRead")
m("RUBRIC hld-4 a transcript path is read once per declaration", "internal/report/report.go",
  "\t\tif d.TranscriptPath != \"\" && !seen[d.TranscriptPath] {\n", "\t\tif d.TranscriptPath != \"\" {\n",
  "TestRubricHLD4_TranscriptPathsAreTheDistinctNamedPathsSorted")
m("RUBRIC hld-4 the read step does not read the final message", "internal/report/gather/gather.go",
  "\tread.Final, read.FinalRead = finalText(path)\n\tread.Set, read.Err = readIDSet(path)\n",
  "\tread.Set, read.Err = readIDSet(path)\n",
  "TestRubricHLD4_AnUnreadableSubagentCostsTheCountNotTheAccount")
m("RUBRIC hld-4 an unreadable subagent costs the account", "internal/report/gather/gather.go",
  "\tread.Final, read.FinalRead = finalText(path)\n\tread.Set, read.Err = readIDSet(path)\n",
  "\tread.Set, read.Err = readIDSet(path)\n\tif read.Err != nil {\n\t\treturn read\n\t}\n\tread.Final, read.FinalRead = finalText(path)\n",
  "TestRubricHLD4_AnUnreadableSubagentCostsTheCountNotTheAccount")
m("RUBRIC hld-4 a subagent that cannot be read leaves a partial count", "internal/report/gather/gather.go",
  "\t\tif err := readIDs(s, set); err != nil {\n\t\t\treturn nil, err\n\t\t}\n",
  "\t\tif err := readIDs(s, set); err != nil {\n\t\t\tcontinue\n\t\t}\n",
  "TestRubricHLD4_AnUnreadableSubagentCostsTheCountNotTheAccount")
m("RUBRIC hld-4 the read step does not ask the project baseline", "internal/report/gather/gather.go",
  "\t\tNovelty:     novelty(run, st.Root(), forgottenBefore),\n", "\t\tNovelty:     report.Novelty{},\n",
  "TestRubricHLD4_BuildReadsTheTranscriptsTheRunNames")
m("RUBRIC hld-4 the read step does not read the sandbox trail", "internal/report/gather/gather.go",
  "\t\tNono:        readNono(cfg.nonoTrail, report.SessionWindow(run)),\n", "\t\tNono:        nono.Observation{},\n",
  "TestRubricHLD4_TheReadStepReadsTheSandboxTrail")
m("RUBRIC hld-4 the baseline's error text reaches the report", "internal/report/gather/gather.go",
  "\t\treturn \"the project baseline could not be read or written\"\n", "\t\treturn err.Error()\n",
  "TestRubricHLD4_TheBaselinesAnswerCarriesNoErrorText")
m("RUBRIC hld-4 the baseline is asked about the client plane", "internal/report/destinations.go",
  "\twire, ssh := novelHostCandidates(declaredHostsFiltered(run, forgotten), sshDeclaredFiltered(run, forgotten), madeMCPCall(run))\n",
  "\twire, ssh := declaredHostsFiltered(run, forgotten), sshDeclaredFiltered(run, forgotten)\n",
  "TestRubricHLD4_TheBaselineQuestionLeavesOutTheClientPlane")
m("RUBRIC hld-4 nono's opener is exported again", "internal/nono/nono.go",
  "func read(path string, w Window) Observation {",
  "func Read(path string) (*os.File, error) { return os.Open(path) }\nfunc read(path string, w Window) Observation {",
  "TestRubricHLD4_NoExportedNonoFunctionOpensAFile")
m("RUBRIC hld-4 nono's opener gains an exported wrapper", "internal/nono/nono.go",
  "func read(path string, w Window) Observation {",
  "func ReadPath(path string, w Window) Observation { return read(path, w) }\n\nfunc read(path string, w Window) Observation {",
  "TestRubricHLD4_NoExportedNonoFunctionOpensAFile")
m("RUBRIC hld-4 nono's opener gains an exported var", "internal/nono/nono.go",
  "func read(path string, w Window) Observation {",
  "var ReadTrail = read\n\nfunc read(path string, w Window) Observation {",
  "TestRubricHLD4_NoExportedNonoFunctionOpensAFile")
m("RUBRIC hld-4 nono's opener gains an exported method", "internal/nono/nono.go",
  "func read(path string, w Window) Observation {",
  "func (o Observation) Load(p string) Observation { return read(p, Window{}) }\n\nfunc read(path string, w Window) Observation {",
  "TestRubricHLD4_NoExportedNonoFunctionOpensAFile")

# The guard itself: it passed both wrappers above while it followed calls
# between free functions only. Skipping package vars, losing a method behind
# another method, or missing os under another name is each the regression.
m("RUBRIC hld-4 the opener guard skips package vars", "internal/nono/no_opener_test.go",
  "\t\t\t\tif d.Tok != token.VAR {\n", "\t\t\t\tif true {\n",
  "TestRubricHLD4_TheOpenerGuardSeesEveryWrapper")
m("RUBRIC hld-4 the opener guard does not follow a method call", "internal/nono/no_opener_test.go",
  "\t\t\t\t\tbySel[d.Name.Name] = append(bySel[d.Name.Name], n)\n", "",
  "TestRubricHLD4_TheOpenerGuardSeesEveryWrapper")
m("RUBRIC hld-4 the opener guard misses os under another name", "internal/nono/no_opener_test.go",
  "\t\t\t\t\tosNames[imp.Name.Name] = true\n", "",
  "TestRubricHLD4_TheOpenerGuardSeesEveryWrapper")

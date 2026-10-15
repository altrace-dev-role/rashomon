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
  "TestRubricHLD4_BuildReadsTheTranscriptsTheRunNames")
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
  "\t\tNono:        nono.Read(cfg.nonoTrail, report.SessionWindow(run)),\n", "\t\tNono:        nono.Observation{},\n",
  "TestRubricHLD4_TheReadStepReadsTheSandboxTrail")
m("RUBRIC hld-4 the baseline's error text reaches the report", "internal/report/destinations.go",
  "\t\tn.Reason = \"the project baseline could not be read or written\"\n", "\t\tn.Reason = err.Error()\n",
  "TestRubricHLD4_TheBaselinesAnswerCarriesNoErrorText")
m("RUBRIC hld-4 the baseline is asked about the client plane", "internal/report/destinations.go",
  "\t\tHosts:     novelHostCandidates(append(declaredHostsFiltered(run, forgotten), sshDeclaredFiltered(run, forgotten)...), madeMCPCall(run)),\n",
  "\t\tHosts:     append(declaredHostsFiltered(run, forgotten), sshDeclaredFiltered(run, forgotten)...),\n",
  "TestRubricHLD4_TheBaselineQuestionLeavesOutTheClientPlane")

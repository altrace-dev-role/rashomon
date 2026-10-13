m("TB a test runner is never recognised", "internal/shape/shape.go",
  "\t\t\tif runsTests(pshaped, i, prog, perr == nil) && !backgrounded(toolInput) {",
  "\t\t\tif false && runsTests(pshaped, i, prog, perr == nil) && !backgrounded(toolInput) {",
  "TestTestRunnerIsRecognised|TestTestClassCarriesNoContent|TestTestBending_")
m("TB a line the lexer could not finish is vouched for", "internal/shape/shape.go",
  "\tif !whole {\n\t\treturn false\n\t}", "\t_ = whole", "TestTestRunnerIsRecognised")
m("TB a quoted argument is compared as plain", "internal/shape/shape.go",
  "\t\t\tif t.quotedAt >= 0 || t.text != want || runsOn(toks, k) {",
  "\t\t\tif t.text != want || runsOn(toks, k) {", "TestTestRunnerIsRecognised")
m("TB an argument running into a process substitution is plain", "internal/shape/shape.go",
  "\t\t\tif t.quotedAt >= 0 || t.text != want || runsOn(toks, k) {",
  "\t\t\tif t.quotedAt >= 0 || t.text != want {", "TestTestRunnerIsRecognised")
m("TB every word of a runner is compared against the first argument", "internal/shape/shape.go",
  "\t\t\tk := i + 1 + n\n", "\t\t\tk := i + 1 + n*0\n", "TestTestRunnerIsRecognised")
# Re-anchored after the masked-status refactor: the row match moved into
# runnerOn, which compares one command's words up to its end.
m("TB a runner missing its argument still matches", "internal/shape/shape.go",
  "\t\t\tif k >= end {\n\t\t\t\tcontinue next\n", "\t\t\tif k >= end {\n\t\t\t\tbreak\n", "TestTestRunnerIsRecognised")
m("TB go test is not on the list", "internal/shape/shape.go",
  "{\"go\", \"test\"}, {\"cargo\", \"test\"}", "{\"go\", \"tset\"}, {\"cargo\", \"test\"}",
  "TestTestRunnerIsRecognised|TestTestBending_OnlyTestFiles")
m("TB the test class is missing from the vocabulary", "internal/shape/shape.go",
  "\t\tVerbPackage, VerbAgent, VerbMCP, VerbUnknown, VerbTest,\n", "\t\tVerbPackage, VerbAgent, VerbMCP, VerbUnknown,\n",
  "TestVerbClassesAreClosed|TestStoreSchemaVerbClasses")
m("TB the test-file label is not in the vocabulary", "internal/shape/label.go",
  "\t\tLabelCertificate,\n\t\tLabelTestFile,\n", "\t\tLabelCertificate,\n", "TestLabelVocabularyIsClosed|TestStoreSchemaLabels")
m("TB the label is not case-folded", "internal/shape/label.go",
  "\tbase := strings.ToLower(written)\n", "\tbase := written\n", "TestLabel")
m("TB the class-name suffixes are folded", "internal/shape/label.go",
  "\t\t\tif strings.HasSuffix(written, s) {", "\t\t\tif strings.HasSuffix(base, strings.ToLower(s)) {", "TestLabelTestFile")
m("TB test_*.py matches on its suffix alone", "internal/shape/label.go",
  "\t\t\tif strings.HasPrefix(written, ps[0]) && strings.HasSuffix(written, ps[1]) {",
  "\t\t\tif strings.HasSuffix(written, ps[1]) {",
  "TestLabelTestFile")
m("TB the test-file row is read before the sensitive rows", "internal/shape/label.go",
  "\tfor _, r := range labelTable {\n\t\tfor _, e := range r.exact {",
  "\tfor _, r := range append(labelTable[len(labelTable)-1:], labelTable...) {\n\t\tfor _, e := range r.exact {",
  "TestLabelTestFile")

m("TB detection reads calls out of seq order", "internal/report/testbending.go",
  "\tsort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Seq < sorted[j].Seq })",
  "\tsort.SliceStable(sorted, func(i, j int) bool { return false })", "TestTestBending")
m("TB a denied edit counts as an edit", "internal/report/testbending.go",
  "\t\tif outcome == LinkOutcomeDenied {\n\t\t\tcontinue\n\t\t}\n",
  "\t\tif outcome == LinkOutcomeDenied && d.Shape.VerbClass != shape.VerbWrite {\n\t\t\tcontinue\n\t\t}\n",
  "TestTestBending")
m("TB an interrupted call between two runs is not an edit", "internal/report/testbending.go",
  "\t\tif outcome == LinkOutcomeDenied {\n\t\t\tcontinue\n\t\t}\n",
  "\t\tif outcome == LinkOutcomeDenied || outcome == store.ExecInterrupted {\n\t\t\tcontinue\n\t\t}\n",
  "TestTestBending")
m("TB a test edit that did not run ok is a test edit", "internal/report/testbending.go",
  "\t\t\tcase outcome == store.ExecOK:\n\t\t\t\ttestEdits++", "\t\t\tdefault:\n\t\t\t\ttestEdits++", "TestTestBending")
m("TB an unlabelled edit is a test edit", "internal/report/testbending.go",
  "\t\t\tcase d.Shape.VerbClass != shape.VerbWrite || d.FileLabel == nil || *d.FileLabel != shape.LabelTestFile:",
  "\t\t\tcase d.Shape.VerbClass != shape.VerbWrite || d.FileLabel != nil && *d.FileLabel != shape.LabelTestFile:",
  "TestTestBending")
m("TB a shell write between two runs is not an edit", "internal/report/testbending.go",
  "\t\tif mayEdit(d) {\n",
  "\t\tif mayEdit(d) && d.ToolName != \"Bash\" {\n",
  "TestTestBending")
# Re-anchored after the masked-status refactor: isRun also refuses a masked line;
# the outcome condition is still what goes.
m("TB a run with no result is a run", "internal/report/testbending.go",
  "\t\tisRun := d.Shape.VerbClass == shape.VerbTest && !statusMasked(d) && (outcome == store.ExecOK || outcome == store.ExecFailed)\n",
  "\t\tisRun := d.Shape.VerbClass == shape.VerbTest && !statusMasked(d)\n",
  "TestTestBending")
m("TB A is raised with code edited too", "internal/report/testbending.go",
  "\t\t\tcase prev.failed && !failed && otherEdits == prev.otherEdits && testEdits > prev.testEdits:",
  "\t\t\tcase prev.failed && !failed && testEdits > prev.testEdits:", "TestTestBending")
m("TB A is raised with no test edit", "internal/report/testbending.go",
  "\t\t\tcase prev.failed && !failed && otherEdits == prev.otherEdits && testEdits > prev.testEdits:",
  "\t\t\tcase prev.failed && !failed && otherEdits == prev.otherEdits && testEdits >= prev.testEdits:", "TestTestBending")
m("TB A is raised from a run that passed", "internal/report/testbending.go",
  "\t\t\tcase prev.failed && !failed && otherEdits == prev.otherEdits && testEdits > prev.testEdits:",
  "\t\t\tcase !failed && otherEdits == prev.otherEdits && testEdits > prev.testEdits:", "TestTestBending")
m("TB B is raised across an edit", "internal/report/testbending.go",
  "\t\t\tcase prev.failed != failed && anyEdits == prev.anyEdits:", "\t\t\tcase prev.failed != failed:", "TestTestBending")
m("TB B is raised for the same outcome twice", "internal/report/testbending.go",
  "\t\t\tcase prev.failed != failed && anyEdits == prev.anyEdits:", "\t\t\tcase anyEdits == prev.anyEdits:", "TestTestBending")
m("TB a pair is made with the first run, not the previous one", "internal/report/testbending.go",
  "\t\tlast[key] = mark{\n", "\t\tif _, seen := last[key]; !seen {\n\t\t\tlast[key] = mark{\n\t\t\t\tseq: d.Seq, failed: failed, testEdits: testEdits, otherEdits: otherEdits, anyEdits: anyEdits,\n\t\t\t}\n\t\t}\n\t\t_ = mark{\n",
  "TestTestBending")
m("TB every command is the same command", "internal/report/testbending.go",
  "\t\tif prev, ok := last[key]; ok && isRun {",
  "\t\tif prev, ok := last[runKey{}]; ok && isRun {",
  "TestTestBending")
m("TB an interrupted test run is counted as a run", "internal/report/testbending.go",
  "\t\tcase store.ExecOK:\n\t\t\tout.Runs++", "\t\tcase store.ExecOK, store.ExecInterrupted:\n\t\t\tout.Runs++", "TestTestRuns_")
m("TB the timeline annotates the earlier row", "internal/report/timeline.go",
  "\t\tbending[p[1]] = TimelineBending{Kind: BendTestsOnlyThenGreen, Since: p[0]}",
  "\t\tbending[p[0]] = TimelineBending{Kind: BendTestsOnlyThenGreen, Since: p[0]}", "TestTimeline_|TestTestBending_")
m("TB the timeline drops the flaky annotation", "internal/report/timeline_text.go",
  "\treturn fmt.Sprintf(\"↳ same command had the other outcome at %d, no recorded file edit between\", t.Since)",
  "\treturn \"\"", "TestTimeline_")
# Re-anchored after the masked-status refactor: the masked count is built between
# the guard and the line it prints.
m("TB the session block renders with no test run", "internal/report/text.go",
  "\tif t == nil || t.Runs == 0 {\n\t\treturn\n\t}\n\tmasked := \"\"", "\tif t == nil {\n\t\treturn\n\t}\n\tmasked := \"\"",
  "TestTestRuns_")
m("TB the session block drops the limit", "internal/report/text.go",
  "\tif len(t.TestsOnlyThenGreen)+len(t.Flaky) > 0 {\n\t\tfmt.Fprintf(b, \"    the numbers are call seqs",
  "\tif false {\n\t\tfmt.Fprintf(b, \"    the numbers are call seqs",
  "TestTestRuns_|TestTestBending_")
m("TB the session block is never rendered", "internal/report/text.go",
  "\twriteTestRuns(b, sess.TestRuns, sess.SessionID)\n", "\t_ = writeTestRuns\n", "TestTestBending_")
m("TB the report never builds the test runs", "internal/report/report.go",
  "\t\tsess.TestRuns = buildTestRuns(run, executed, denied, tb)\n",
  "\t\t_ = buildTestRuns\n",
  "TestTestBending_")

m("TB the digest keeps another turn's tests-only pair", "internal/digest/digest.go",
  "\t\tif seqs[p[0]] && seqs[p[1]] {\n", "\t\tif true {\n", "TestBuild_TestBending")
m("TB the digest keeps another turn's flaky pair", "internal/digest/digest.go",
  "\t\tif seqs[p.Seqs[0]] && seqs[p.Seqs[1]] {\n", "\t\tif true {\n", "TestBuild_TestBending")
m("TB the digest keeps a pair with one run in the turn", "internal/digest/digest.go",
  "\t\tif seqs[p.Seqs[0]] && seqs[p.Seqs[1]] {\n", "\t\tif seqs[p.Seqs[1]] {\n", "TestBuild_TestBendingNamesNoPairAcrossTurns")
m("TB the digest keeps a tests-only pair with one run in the turn", "internal/digest/digest.go",
  "\t\tif seqs[p[0]] && seqs[p[1]] {\n", "\t\tif seqs[p[1]] {\n", "TestBuild_TestBendingNamesNoTestsOnlyPairAcrossTurns")
m("TB the digest finds pairs over the turn's calls alone", "internal/digest/digest.go",
  "\td.TestBending = turnPairs(report.DetectTestBending(run, nil), w)",
  "\td.TestBending = turnPairs(report.DetectTestBending(turnRun, nil), w)",
  "TestBuild_ALostDeclarationFormsNoPair|TestBuild_AnotherTurnsEditBetweenFormsNoPair")
m("TB the digest never carries the patterns", "internal/digest/digest.go",
  "\td.TestBending = turnPairs(report.DetectTestBending(run, nil), w)\n",
  "", "TestBuild_TestBending|TestTestBending_")
m("TB an empty digest marshals null lists", "internal/digest/digest.go",
  "\t\tTestBending:    TestBending{TestsOnlyThenGreen: []report.SeqPair{}, Flaky: []report.FlakyPair{}},\n",
  "",
  "TestBuild_TestBendingListsAreNeverNull")
m("TB truncate cuts a test-bending list before it is the last resort", "internal/digest/truncate.go",
  "\t\tfunc() bool { return trimStrings(&d.Declarations.Dropped, &d.Declarations.DroppedOmitted) },",
  "\t\tfunc() bool { return trimPairs(&d.TestBending.Flaky, &d.TestBending.FlakyOmitted) },\n\t\tfunc() bool { return trimStrings(&d.Declarations.Dropped, &d.Declarations.DroppedOmitted) },",
  "TestTruncate_TestBending")
m("TB truncate cuts tests-only before flaky", "internal/digest/truncate.go",
  "\t\tfunc() bool { return trimPairs(&d.TestBending.Flaky, &d.TestBending.FlakyOmitted) },\n\t\tfunc() bool {\n\t\t\treturn trimPairs(&d.TestBending.TestsOnlyThenGreen, &d.TestBending.TestsOnlyThenGreenOmitted)\n\t\t},",
  "\t\tfunc() bool {\n\t\t\treturn trimPairs(&d.TestBending.TestsOnlyThenGreen, &d.TestBending.TestsOnlyThenGreenOmitted)\n\t\t},\n\t\tfunc() bool { return trimPairs(&d.TestBending.Flaky, &d.TestBending.FlakyOmitted) },",
  "TestTruncate_TestBending")
m("TB a cut pair is not counted as omitted", "internal/digest/truncate.go",
  "func trimPairs[P any](s *[]P, omitted *int) bool {\n\tif len(*s) == 0 {\n\t\treturn false\n\t}\n\tcut := (len(*s) + 1) / 2\n\t*omitted += cut\n",
  "func trimPairs[P any](s *[]P, omitted *int) bool {\n\tif len(*s) == 0 {\n\t\treturn false\n\t}\n\tcut := (len(*s) + 1) / 2\n",
  "TestTruncate_TestBending")

m("TB the end-of-turn line drops the tests-only sentence", "internal/recap/recap.go",
  "\t\tfirst, len(tb.TestsOnlyThenGreen)+tb.TestsOnlyThenGreenOmitted); ok {\n\t\tsentences = append(sentences, s)",
  "\t\tfirst, len(tb.TestsOnlyThenGreen)+tb.TestsOnlyThenGreenOmitted); ok {\n\t\t_ = s",
  "TestLineTestBending|TestTestBending_")
m("TB the end-of-turn line drops the flaky sentence", "internal/recap/recap.go",
  "\t\tfirst, len(tb.Flaky)+tb.FlakyOmitted); ok {\n\t\tsentences = append(sentences, s)",
  "\t\tfirst, len(tb.Flaky)+tb.FlakyOmitted); ok {\n\t\t_ = s",
  "TestLineTestBending|TestTestBending_")
m("TB the end-of-turn line forgets what truncate cut", "internal/recap/recap.go",
  "len(tb.TestsOnlyThenGreen)+tb.TestsOnlyThenGreenOmitted)",
  "len(tb.TestsOnlyThenGreen)+tb.TestsOnlyThenGreenOmitted*0)",
  "TestLineTestBending")
m("TB the end-of-turn line hides the other pairs", "internal/recap/recap.go",
  "\t\ts += fmt.Sprintf(\", %d more\", n-1)", "\t\t_ = n", "TestLineTestBending")

m("TB a background launch is a test run", "internal/shape/shape.go",
  "\t\t\tif runsTests(pshaped, i, prog, perr == nil) && !backgrounded(toolInput) {",
  "\t\t\tif runsTests(pshaped, i, prog, perr == nil) && !backgrounded(nil) {",
  "TestBackgroundLaunchIsNotATestRun|TestTestBending_ShapesFromDerive")
# Re-anchored after the masked-status refactor: runsTests now ends in one return
# joining runnerOn and wholeCommand.
m("TB a runner piped or listed into another command is a test run", "internal/shape/shape.go",
  "testCommands) && wholeCommand(toks, i)\n", "testCommands) && wholeCommand(toks[:i+1], i)\n",
  "TestTestRunnerIsRecognised|TestTestBending_ShapesFromDerive")
m("TB a command on the runner's next line is not seen", "internal/shape/shape.go",
  "\t\tif t.nlBefore {\n\t\t\treturn false\n\t\t}\n\t}\n\treturn true\n}",
  "\t\tif t.nlBefore && false {\n\t\t\treturn false\n\t\t}\n\t}\n\treturn true\n}",
  "TestTestRunnerIsRecognised")
m("TB a cd between two runs does not break the pair", "internal/report/testbending.go",
  "func mayEdit(d store.Declaration) bool {\n",
  "func mayEdit(d store.Declaration) bool {\n\tif d.Shape.Program != nil && (*d.Shape.Program == \"cd\" || *d.Shape.Program == \"pushd\") {\n\t\treturn false\n\t}\n",
  "TestTestBending")
m("TB a denied cd breaks the pair", "internal/report/testbending.go",
  "\t\tif outcome == LinkOutcomeDenied {\n\t\t\tcontinue\n\t\t}\n",
  "\t\tif outcome == LinkOutcomeDenied && (d.Shape.Program == nil || *d.Shape.Program != \"cd\") {\n\t\t\tcontinue\n\t\t}\n",
  "TestTestBending")
m("TB the session block drops the directory limit", "internal/report/text.go",
  "\t\tfmt.Fprintln(b, \"    runs pair only when the same command line started in the same directory: the reported cwd, or where its leading plain cd steps lead;\")\n",
  "",
  "TestTestRuns_")

m("TB only a write-class call is an edit", "internal/report/testbending.go",
  "\tcase shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:\n\t\treturn false\n\t}\n\treturn true\n",
  "\tcase shape.VerbWrite:\n\t\treturn true\n\t}\n\treturn false\n",
  "TestTestBending|TestTestBending_AnyCallThatCouldChangeFilesBreaksThePair")
m("TB a read between two runs stops the pair", "internal/report/testbending.go",
  "\tcase shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:\n",
  "\tcase shape.VerbNetwork, shape.VerbAgent:\n",
  "TestTestBending")
m("TB a web fetch between two runs stops the pair", "internal/report/testbending.go",
  "\tcase shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:\n",
  "\tcase shape.VerbRead, shape.VerbAgent:\n",
  "TestTestBending")
m("TB a subagent launch between two runs stops the pair", "internal/report/testbending.go",
  "\tcase shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:\n",
  "\tcase shape.VerbRead, shape.VerbNetwork:\n",
  "TestTestBending")
m("TB another test command between two runs is not an edit", "internal/report/testbending.go",
  "func mayEdit(d store.Declaration) bool {\n",
  "func mayEdit(d store.Declaration) bool {\n\tif d.Shape.VerbClass == shape.VerbTest {\n\t\treturn false\n\t}\n",
  "TestTestBending|TestTestBending_AnyCallThatCouldChangeFilesBreaksThePair")
m("TB a run is between itself and the run it pairs with", "internal/report/testbending.go",
  "testEdits: testEdits, otherEdits: otherEdits, anyEdits: anyEdits,\n",
  "testEdits: testEdits, otherEdits: otherEdits - 1, anyEdits: anyEdits - 1,\n",
  "TestTestBending")

m("TB make check is a test run", "internal/shape/shape.go",
  "{\"gradlew\", \"test\"}, {\"make\", \"test\"},\n",
  "{\"gradlew\", \"test\"}, {\"make\", \"test\"}, {\"make\", \"check\"},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
# Re-anchored after the masked-status refactor: the refusal moved into runnerOn,
# over the runner's own command (toks up to end); the next two as well.
m("TB an argument that does not run the tests is ignored", "internal/shape/shape.go",
  "\t\treturn !refusesRun(refusalsOf(c), toks[:i], toks[i+len(c):end])",
  "\t\treturn !(false && refusesRun(refusalsOf(c), toks[:i], toks[i+len(c):end]))",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB the refusal reads only the first argument", "internal/shape/shape.go",
  "\t\treturn !refusesRun(refusalsOf(c), toks[:i], toks[i+len(c):end])",
  "\t\treturn !refusesRun(refusalsOf(c), toks[:i], toks[i+len(c):min(i+len(c)+1, end)])",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a prefix entry is compared whole", "internal/shape/shape.go",
  "\t\t\tif strings.HasPrefix(word, p) {\n",
  "\t\t\tif word == p {\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB the per-runner list is not read", "internal/shape/shape.go",
  "\tlist := notARun[strings.TrimSuffix(runner, \" build\")]\n",
  "\tlist := []string(nil)\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB help, version and watch refuse nothing", "internal/shape/shape.go",
  "|| onList(t.text, notARunAny)",
  "|| onList(t.text, nil)",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB tox with a target is a test run", "internal/shape/shape.go",
  "\"tox\": {\"-e*\", ",
  "\"tox\": {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB nox with a session is a test run", "internal/shape/shape.go",
  "\"nox\": {\"-s*\", ",
  "\"nox\": {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB go test -c is a test run", "internal/shape/shape.go",
  "\"go\":      {\"-c\", ",
  "\"go\":      {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB pytest --collect-only is a test run", "internal/shape/shape.go",
  "\t\"--collect-only\", \"--co\", ",
  "\t",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB cargo test --no-run is a test run", "internal/shape/shape.go",
  "\"cargo\":   {\"--no-run\", ",
  "\"cargo\":   {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB jest --listTests is a test run", "internal/shape/shape.go",
  "\"jest\":    {\"--listTests\", ",
  "\"jest\":    {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB watch mode is a test run", "internal/shape/shape.go",
  "\"--version\", \"--watch\", \"--watchAll\"}",
  "\"--version\"}",
  "TestTestClassRefusesWhatDoesNotRunTests")

m("TB the timeline says only test files were edited, not what was recorded", "internal/report/timeline_text.go",
  "\"↳ the only recorded edits since %d were to files named like tests, where the same command failed\"",
  "\"↳ only files named like tests edited since %d, where the same command failed\"",
  "TestTimeline_AnnotatesTheRowThatCompletesAPattern|TestTestBending_OnlyTestFilesEditedThenGreenIsFlagged")

m("TB the end-of-turn line forgets what truncate cut from flaky", "internal/recap/recap.go",
  "len(tb.Flaky)+tb.FlakyOmitted)",
  "len(tb.Flaky)+tb.FlakyOmitted*0)",
  "TestLineTestBending")
m("TB a flaky pair records the later run's outcome", "internal/report/testbending.go",
  "FlakyPair{Seqs: pair, FirstFailed: prev.failed}",
  "FlakyPair{Seqs: pair, FirstFailed: failed}",
  "TestTestBending_FlakyKeepsTheOrder|TestTestRuns_")
m("TB a flaky pair is always passed then failed", "internal/report/testbending.go",
  "\tif p.FirstFailed {\n\t\treturn \"failed\", \"passed\"\n\t}",
  "\tif false {\n\t\treturn \"failed\", \"passed\"\n\t}",
  "TestTestBending_FlakyKeepsTheOrder|TestTestRuns_|TestLineTestBending")
m("TB the session block prints a flaky pair without its order", "internal/report/text.go",
  "between: %d %s, %d %s\\n\",\n\t\t\tp.Seqs[0], first, p.Seqs[1], second)",
  "between: %d, %d\\n\",\n\t\t\tp.Seqs[0], p.Seqs[1])\n\t\t_, _ = first, second",
  "TestTestRuns_")
m("TB the end-of-turn line prints a flaky pair without its order", "internal/recap/recap.go",
  "\t\tfirst = fmt.Sprintf(\"#%d %s, #%d %s\", p.Seqs[0], a, p.Seqs[1], b)",
  "\t\tfirst = fmt.Sprintf(\"#%d, #%d\", p.Seqs[0], p.Seqs[1])\n\t\t_, _ = a, b",
  "TestLineTestBending|TestTestBending_SameCommandBothOutcomesIsFlagged")

m("TB go and pytest suffixes are folded", "internal/shape/label.go",
  "\t\t\t\"_spec.rb\",\n",
  "\t\t\t\"_spec.rb\", \"_test.go\", \"_test.py\",\n",
  "TestLabelTestFile")
m("TB test_*.py is folded", "internal/shape/label.go",
  "\t\t\tif strings.HasPrefix(written, ps[0]) && strings.HasSuffix(written, ps[1]) {",
  "\t\t\tif strings.HasPrefix(base, ps[0]) && strings.HasSuffix(base, ps[1]) {",
  "TestLabelTestFile")
m("TB conftest.py is folded", "internal/shape/label.go",
  "\t\t\tif written == e {",
  "\t\t\tif base == e {",
  "TestLabelTestFile")
m("TB conftest.py is not a test file", "internal/shape/label.go",
  "casedExact: []string{\"conftest.py\"},",
  "casedExact: []string{\"conftest.pyx\"},",
  "TestLabelTestFile")
m("TB *Test.php is not a test file", "internal/shape/label.go",
  ", \"Tests.cs\", \"Test.php\",\n",
  ", \"Tests.cs\",\n",
  "TestLabelTestFile")

m("TB the session block drops the cd-failure limit", "internal/report/text.go",
  "\t\tfmt.Fprintln(b, \"    and a runner behind `cd DIR &&` is a test run, so a cd that failed reads as a failed run\")\n",
  "",
  "TestTestRuns_")

m("TB a task or todo update between two runs is an edit", "internal/report/testbending.go",
  "\tif noWrite[d.ToolName] {\n",
  "\tif false && noWrite[d.ToolName] {\n",
  "TestTestBending")

m("TB a flag's =value form is not refused", "internal/shape/shape.go",
  "\t\t} else if hasValue && strings.HasPrefix(w, \"-\") && name == w && !isFalse(value) {",
  "\t\t} else if false && hasValue && strings.HasPrefix(w, \"-\") && name == w && !isFalse(value) {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a flag set =false is refused", "internal/shape/shape.go",
  "\t\t} else if hasValue && strings.HasPrefix(w, \"-\") && name == w && !isFalse(value) {",
  "\t\t} else if hasValue && strings.HasPrefix(w, \"-\") && name == w && (!isFalse(value) || true) {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB make test -n is a test run", "internal/shape/shape.go",
  "\"make\":    {\"-n\", \"--just-print\", \"--dry-run\", \"--recon\", \"-q\", \"--question\", ",
  "\"make\":    {\"--recon\", ",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB make test -v is a test run", "internal/shape/shape.go",
  "\"-t\", \"--touch\", \"-v\"},",
  "\"-t\", \"--touch\"},",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB gradle continuous mode is a test run", "internal/shape/shape.go",
  "\"gradle\":  {\"--dry-run\", \"-m\", \"-v\", \"-t\", \"--continuous\"},",
  "\"gradle\":  {\"--dry-run\", \"-m\", \"-v\"},",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB mvn test -v is a test run", "internal/shape/shape.go",
  "\t\"mvn\":     {\"-v\"},\n",
  "\t\"mvn\":     {},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB tox --notest is a test run", "internal/shape/shape.go",
  "\"--help-ini\", \"--notest\",",
  "\"--help-ini\",",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB tox devenv is a test run", "internal/shape/shape.go",
  ", \"devenv\", \"d\"},",
  "},",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB nox --install-only is a test run", "internal/shape/shape.go",
  ", \"--install-only\"},",
  "},",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB PYTEST_ADDOPTS on the line is not read", "internal/shape/shape.go",
  "\t\t\tif strings.HasPrefix(t.text, \"PYTEST_ADDOPTS=\") {",
  "\t\t\tif false && strings.HasPrefix(t.text, \"PYTEST_ADDOPTS=\") {",
  "TestTestClassRefusesWhatDoesNotRunTests")
# Re-anchored after the masked-status refactor: the refusal is in runnerOn.
m("TB the words before the runner are not read", "internal/shape/shape.go",
  "\t\treturn !refusesRun(refusalsOf(c), toks[:i], toks[i+len(c):end])",
  "\t\treturn !refusesRun(refusalsOf(c), toks[i:i], toks[i+len(c):end])",
  "TestTestClassRefusesWhatDoesNotRunTests")

m("TB *Test.cs and *Tests.kt are not test files", "internal/shape/label.go",
  "\"Test.kt\", \"Tests.kt\", \"Test.cs\", \"Tests.cs\",",
  "\"Test.kt\", \"Tests.cs\",",
  "TestLabelTestFile")
m("TB GoogleTest files are not test files", "internal/shape/label.go",
  "\"_test.py\", \"_test.cc\", \"_unittest.cc\",\n",
  "\"_test.py\",\n",
  "TestLabelTestFile")
m("TB the .mts and .cts infixes are not test files", "internal/shape/label.go",
  ", \".test.mts\", \".test.cts\",\n",
  ",\n",
  "TestLabelTestFile")
m("TB GoogleTest suffixes are folded", "internal/shape/label.go",
  "\t\t\t\"_spec.rb\",\n",
  "\t\t\t\"_spec.rb\", \"_test.cc\",\n",
  "TestLabelTestFile")

m("TB the report's timeline is built without the patterns", "internal/report/report.go",
  "\t\tsess.Timeline = timelineFrom(run, executed, denied, tb)\n",
  "\t\tsess.Timeline = timelineFrom(run, executed, denied, TestBending{})\n",
  "TestTestBending_|TestTimeline")

m("TB the test runs drop the patterns they were given", "internal/report/testbending.go",
  "\tout := &TestRuns{Undeclared: undeclaredCalls(run, denied), TestBending: tb}\n",
  "\tout := &TestRuns{Undeclared: undeclaredCalls(run, denied), TestBending: TestBending{TestsOnlyThenGreen: []SeqPair{}, Flaky: []FlakyPair{}}}\n\t_ = tb\n",
  "TestTestRuns_|TestTestBending_")

m("TB the session block drops the timeline hint", "internal/report/text.go",
  "\t\tfmt.Fprintf(b, \"    the numbers are call seqs, and `rashomon report --session %s --timeline` shows these rows;\\n\", pasteArg(sessionID))\n",
  "",
  "TestTestRuns_")

m("TB Derive never produces the test class", "internal/shape/shape.go",
  "\t\t\tif runsTests(pshaped, i, prog, perr == nil) && !backgrounded(toolInput) {",
  "\t\t\tif false && runsTests(pshaped, i, prog, perr == nil) && !backgrounded(toolInput) {",
  "TestTestBending_ShapesFromDerive")

m("TB the redaction walk skips an embedded struct", "internal/report/redact_enumerate_test.go",
  "\t\t\tif flattened(f, name) {\n\t\t\t\twalkStringPaths(",
  "\t\t\tif false && flattened(f, name) {\n\t\t\t\twalkStringPaths(",
  "TestRedact_")
m("TB the redaction plant skips an embedded struct", "internal/report/redact_enumerate_test.go",
  "\t\t\tif flattened(f, name) {\n\t\t\t\tplantAt(",
  "\t\t\tif false && flattened(f, name) {\n\t\t\t\tplantAt(",
  "TestRedact_")

m("TB the session block drops the unlisted-option limit", "internal/report/text.go",
  "\t\tfmt.Fprintln(b, \"    but one that writes through an option not on that list (find -fprint, curl -D or -c) is not counted;\")\n",
  "",
  "TestTestRuns_")

# Re-anchored after the masked-status refactor: the writer moved to schema 4,
# and the mutation is still "the writer stays at the version before".
m("TB the writer stays at schema 3", "internal/store/record.go",
  "const SchemaVersion = 4",
  "const SchemaVersion = 3",
  "TestSchema3_")
m("TB test runs are counted over records that predate the test class", "internal/report/testbending.go",
  "\tif run == nil || !measuresTests(run) {\n",
  "\tif run == nil {\n",
  "TestTestRuns_")

m("TB a backgrounded execution reads as its recorded ok", "internal/report/chains.go",
  "\t\tcase e.Backgrounded:\n",
  "\t\tcase false && e.Backgrounded:\n",
  "TestTestBending|TestTestRuns_|TestTimeline_ABackgrounded")
m("TB the post path never reads the background keys", "internal/hook/post.go",
  "\t\trec.Backgrounded = backgroundedCall(payload)\n",
  "\t\trec.Backgrounded = false && backgroundedCall(payload)\n",
  "TestTestBending_ARunMovedToTheBackground|TestH20_Backgrounded")
m("TB the background bit is read for every tool", "internal/hook/post.go",
  "\tif pl.ToolName == \"Bash\" && pl.HookEventName != FailureEvent {\n\t\trec.Backgrounded",
  "\tif pl.HookEventName != FailureEvent {\n\t\trec.Backgrounded",
  "TestH20_BackgroundedIsReadForBashOnly")
m("TB a failure event can be moved to the background", "internal/hook/post.go",
  "\tif pl.ToolName == \"Bash\" && pl.HookEventName != FailureEvent {\n\t\trec.Backgrounded",
  "\tif pl.ToolName == \"Bash\" {\n\t\trec.Backgrounded",
  "TestTestBending_AFailureIsNeverMovedToTheBackground")
m("TB a false backgroundedByUser sets the bit", "internal/hook/post.go",
  "\treturn bits[0].Set() || bits[1].Set()\n",
  "\treturn bits[0].Set() || bits[1].Present()\n",
  "TestH20_BackgroundedIsReadForBashOnly")

m("TB runs pair across directories", "internal/report/testbending.go",
  "\t\tkey := runKey{d.Shape.Digest, d.CWDDigest}\n",
  "\t\tkey := runKey{d.Shape.Digest, \"\"}\n",
  "TestTestBending")
m("TB the declaration carries no cwd digest", "internal/hook/handle.go",
  "\t\tCWDDigest:      shape.CWDDigest(h.st.Key(), startDirectory(p.CWD, p.ToolName, p.ToolInput)),\n",
  "",
  "TestTestBending_ARepeatedRelativeCd|TestSchema3_|TestH13_")
m("TB the cwd digest is the plain path", "internal/shape/shape.go",
  "\treturn digest(key, \"\\x00cwd\", cwd)\n",
  "\treturn cwd\n",
  "TestTestBending_ARepeatedRelativeCd")

# Re-anchored after the masked-status refactor: mayEdit's check also counts a
# masked line; only may_write is dropped.
m("TB mayEdit ignores may_write", "internal/report/testbending.go",
  "\tif d.Shape.MayWrite || statusMasked(d) {\n\t\treturn true\n\t}\n",
  "\tif statusMasked(d) {\n\t\treturn true\n\t}\n",
  "TestTestBending_WriteCapable")
m("TB Derive never sets may_write for a named program", "internal/shape/shape.go",
  "\t\t\ts.MayWrite = perr != nil || mayWrite(pshaped, i)\n",
  "\t\t\ts.MayWrite = false\n",
  "TestMayWrite|TestTestBending_WriteCapable")
m("TB a line whose program cannot be named may not write", "internal/shape/shape.go",
  "\ts.MayWrite = true\n\tif i, ok := programToken",
  "\ts.MayWrite = false\n\tif i, ok := programToken",
  "TestMayWrite")
m("TB find -delete and -exec do not write", "internal/shape/shape.go",
  "\t\tcase \"-delete\", \"-exec\", \"-execdir\", \"-ok\", \"-okdir\":\n",
  "\t\tcase \"-okdir\":\n",
  "TestMayWrite|TestTestBending_WriteCapable")
m("TB xargs, tee, rsync and scp do not write", "internal/shape/shape.go",
  "\t\tcase \"xargs\", \"tee\", \"wget\", \"rsync\", \"scp\":\n",
  "\t\tcase \"wget\":\n",
  "TestMayWrite|TestTestBending_WriteCapable")
m("TB curl with an output flag does not write", "internal/shape/shape.go",
  "\tif curl && curlOut {\n",
  "\tif false && curl && curlOut {\n",
  "TestMayWrite|TestTestBending_WriteCapable")
m("TB a later stage is not looked at", "internal/shape/shape.go",
  "\t\tif !ok || verbForProgram(path.Base(toks[start+k].text)) != VerbRead {\n",
  "\t\tif false && (!ok || verbForProgram(path.Base(toks[start+k].text)) != VerbRead) {\n",
  "TestMayWrite")
m("TB a descriptor duplication is a write", "internal/shape/shape.go",
  "\tif dup && (target == \"-\" || isFDPrefix(toks[end+1])) {\n",
  "\tif false && dup && (target == \"-\" || isFDPrefix(toks[end+1])) {\n",
  "TestMayWrite")
m("TB /dev/null is a file written", "internal/shape/shape.go",
  "\tcase \"/dev/null\", \"/dev/stdout\", \"/dev/stderr\", \"/dev/tty\":\n",
  "\tcase \"/dev/stdout\", \"/dev/stderr\", \"/dev/tty\":\n",
  "TestMayWrite")
m("TB the & of 2>&1 separates stages", "internal/shape/shape.go",
  "(toks[j-1].text == \">\" || toks[j-1].text == \">>\" || toks[j-1].text == \"|\")",
  "(toks[j-1].text == \"|\")",
  "TestMayWrite")
m("TB the session block drops what makes a read an edit", "internal/report/text.go",
  "\t\tfmt.Fprintln(b, \"    a shell read or fetch counts when its line may write: a redirect to a file, a download (curl -o, attached or not), a command or process substitution, find -delete or -exec, xargs, tee, rsync or scp, or a later stage that is not a read,\")\n",
  "",
  "TestTestRuns_")

m("TB a runner prefix is not stepped past", "internal/shape/shape.go",
  "\ti, prog, ok := runnerPrefix(toks, i, prog)\n",
  "\t_, _, ok := runnerPrefix(toks, i, prog)\n",
  "TestTestRunnerIsRecognised")
m("TB time is not stepped past", "internal/shape/shape.go",
  "\tif prog == \"time\" {\n",
  "\tif false && prog == \"time\" {\n",
  "TestTestRunnerIsRecognised")
m("TB timeout takes any word for its duration", "internal/shape/shape.go",
  "\t\tif !plain(i+1) || !isDuration(toks[i+1].text) || !word(i+2) {\n",
  "\t\tif !plain(i+1) || !word(i+2) {\n",
  "TestTestRunnerIsRecognised")
m("TB a wrapped runner is refused on the wrapper's list", "internal/shape/shape.go",
  "\tswitch c[0] {\n\tcase \"npx\", \"uv\", \"poetry\", \"bundle\":\n",
  "\tswitch \"\" {\n\tcase \"npx\", \"uv\", \"poetry\", \"bundle\":\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB the npx, uv, poetry and bundle forms are off the list", "internal/shape/shape.go",
  "\t{\"npx\", \"jest\"}, {\"npx\", \"vitest\"}, {\"uv\", \"run\", \"pytest\"}, {\"poetry\", \"run\", \"pytest\"},\n\t{\"bundle\", \"exec\", \"rspec\"},\n",
  "",
  "TestTestRunnerIsRecognised")
m("TB gradlew and mvnw are off the list", "internal/shape/shape.go",
  "{\"mvnw\", \"test\"}, {\"gradle\", \"test\"}, {\"gradlew\", \"test\"}, ",
  "{\"gradle\", \"test\"}, ",
  "TestTestRunnerIsRecognised")
m("TB npm t is off the list", "internal/shape/shape.go",
  "{\"npm\", \"t\"}, ",
  "",
  "TestTestRunnerIsRecognised")
m("TB timeout's own 124 is a failed run", "internal/report/testbending.go",
  "c != nil && *c == timeoutFired {",
  "false && c != nil && *c == timeoutFired {",
  "TestTestBending|TestTestRuns_")
m("TB a runner's own 124 is no result", "internal/report/testbending.go",
  "\tif o != store.ExecFailed || d.Shape.Program == nil || *d.Shape.Program != \"timeout\" {\n",
  "\tif o != store.ExecFailed {\n",
  "TestTestBending")

m("TB curl's attached output file hides the flag", "internal/shape/shape.go",
  "\t\t\tcase c == 'o' || c == 'O':\n\t\t\t\treturn true\n\t\t\tcase strings.ContainsRune(curlArgumentOptions, c):\n\t\t\t\treturn false\n\t\t\tcase 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '#', c == ':':\n\t\t\tdefault:\n\t\t\t\treturn false\n\t\t\t}\n\t\t}\n\t}\n\treturn false\n}\n",
  "\t\t\tcase c == 'o' || c == 'O':\n\t\t\tcase strings.ContainsRune(curlArgumentOptions, c):\n\t\t\t\treturn false\n\t\t\tcase 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '#', c == ':':\n\t\t\tdefault:\n\t\t\t\treturn false\n\t\t\t}\n\t\t}\n\t\treturn strings.ContainsAny(w[1:], \"oO\")\n\t}\n\treturn false\n}\n",
  "TestMayWrite|TestTestBending_AnAttachedCurl")
m("TB curl's digits, # and : end the short-flag walk", "internal/shape/shape.go",
  "\t\t\tcase 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '#', c == ':':\n",
  "\t\t\tcase 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':\n",
  "TestMayWrite|TestTestBending_ACurlProgressBar")
m("TB curl's argument-taking options are read as more flags", "internal/shape/shape.go",
  "\t\t\tcase strings.ContainsRune(curlArgumentOptions, c):\n\t\t\t\treturn false\n",
  "",
  "TestMayWrite")
m("TB a command or process substitution is not a write", "internal/shape/shape.go",
  "\t\tif opensSubstitution(toks, j) {\n",
  "\t\tif false && opensSubstitution(toks, j) {\n",
  "TestMayWrite")
m("TB backticks are not a write", "internal/shape/shape.go",
  "\t\tif t.ticks > 0 || t.opaque && holdsSubstitution(t.text) {\n",
  "\t\tif t.opaque && holdsSubstitution(t.text) {\n",
  "TestMayWrite")
m("TB a quoted $( is not a write", "internal/shape/shape.go",
  "\t\tif t.ticks > 0 || t.opaque && holdsSubstitution(t.text) {\n",
  "\t\tif t.ticks > 0 {\n",
  "TestMayWrite")
m("TB a single-quoted $( is a write", "internal/shape/shape.go",
  "\t\tif t.ticks > 0 || t.opaque && holdsSubstitution(t.text) {\n",
  "\t\tif t.ticks > 0 || holdsSubstitution(t.text) {\n",
  "TestMayWrite")
m("TB arithmetic is a command substitution", "internal/shape/shape.go",
  "\treturn strings.HasSuffix(t.text, \"$\") && !(gluedParen(toks, j+2) && arithmeticAt(parenText(toks[j+3:])))\n",
  "\treturn strings.HasSuffix(t.text, \"$\")\n",
  "TestMayWrite")
m("TB a $(( that does not close as )) is arithmetic", "internal/shape/shape.go",
  "\t\t\t\treturn strings.HasPrefix(s[k+1:], \")\")\n",
  "\t\t\t\treturn true\n",
  "TestMayWrite")
m("TB quoted arithmetic is a command substitution", "internal/shape/shape.go",
  "\t\tif !strings.HasPrefix(s[k+2:], \"(\") || !arithmeticAt(s[k+3:]) {\n",
  "\t\tif true {\n",
  "TestMayWrite")
m("TB a ( inside arithmetic opens no group", "internal/shape/shape.go",
  "\t\tcase '(':\n\t\t\tdepth++\n",
  "",
  "TestMayWrite")
m("TB a quoted ) inside $(( closes it", "internal/shape/shape.go",
  "\t\t\tb.WriteByte('w')\n",
  "\t\t\tb.WriteString(t.text)\n",
  "TestMayWrite")

m("TB a backgrounded call is not counted as unobserved", "internal/report/account.go",
  "\t\tif x.Backgrounded {\n\t\t\t// Recorded ok",
  "\t\tif false && x.Backgrounded {\n\t\t\t// Recorded ok",
  "TestSilentFailures_")

m("TB a leading cd's target is not the run's directory", "internal/hook/handle.go",
  "shape.CWDDigest(h.st.Key(), startDirectory(p.CWD, p.ToolName, p.ToolInput)),",
  "shape.CWDDigest(h.st.Key(), p.CWD),",
  "TestTestBending_ALeadingAbsoluteCd")
m("TB a relative leading cd keeps the shell's directory", "internal/hook/handle.go",
  "\t\t\tcwd = filepath.Join(cwd, dir)\n",
  "\t\t\treturn cwd\n",
  "TestStartDirectory|TestTestBending_ALeadingRelativeCd")
m("TB a leading run stops at a step it cannot resolve", "internal/shape/shape.go",
  "\t\tif !ok {\n\t\t\treturn nil, false\n\t\t}\n\t\tdirs = append(dirs, dir)\n",
  "\t\tif !ok {\n\t\t\treturn dirs, len(dirs) > 0\n\t\t}\n\t\tdirs = append(dirs, dir)\n",
  "TestLeadingDirectory|TestTestBending_ACdBack")
m("TB a leading pushd or popd is not a directory change", "internal/shape/shape.go",
  "\t\tcase \"cd\", \"pushd\", \"popd\":\n",
  "\t\tcase \"cd\":\n",
  "TestLeadingDirectory")
m("TB an assignment in front of a later cd is not a directory change", "internal/shape/shape.go",
  "\t\tswitch toks[i+k].text {\n",
  "\t\t_ = k\n\t\tswitch toks[i].text {\n",
  "TestLeadingDirectory|TestTestBending_AnAssignedCdBack")

m("TB a backgrounded call between two runs is not an edit", "internal/report/testbending.go",
  "\t\tif outcome == LinkOutcomeDenied {\n\t\t\tcontinue\n\t\t}\n",
  "\t\tif outcome == LinkOutcomeDenied || outcome == LinkOutcomeBackgrounded {\n\t\t\tcontinue\n\t\t}\n",
  "TestTestBending")
m("TB a prefixed run records its runner as the program", "internal/shape/shape.go",
  "\t\t\t\ts.VerbClass = VerbTest\n",
  "\t\t\t\ts.VerbClass = VerbTest\n\t\t\t\t_, runner, _ := runnerPrefix(pshaped, i, prog)\n\t\t\t\ts.Program = &runner\n",
  "TestTestRunnerIsRecognised|TestTestBending")

m("TB Maven's skip properties are not refused", "internal/shape/shape.go",
  " || runner == \"mvn\" && mavenSkip(t.text) {\n",
  " {\n",
  "TestTestClassRefusesWhatDoesNotRunTests|TestTestRunnerIsRecognised")
m("TB a Maven skip property refuses its =false form", "internal/shape/shape.go",
  "\t\treturn !hasValue || strings.EqualFold(value, \"true\")\n",
  "\t\treturn !hasValue || len(value) >= 0\n",
  "TestTestRunnerIsRecognised")
m("TB a Maven skip property is read as true only in lower case", "internal/shape/shape.go",
  "\t\treturn !hasValue || strings.EqualFold(value, \"true\")\n",
  "\t\treturn !hasValue || value == \"true\"\n",
  "TestTestRunnerIsRecognised")
m("TB a prefix entry takes isFalse's exception", "internal/shape/shape.go",
  "\t\t\tif strings.HasPrefix(word, p) {\n",
  "\t\t\tif v, ok := strings.CutPrefix(strings.TrimPrefix(word, p), \"=\"); strings.HasPrefix(word, p) && !(ok && isFalse(v)) {\n",
  "TestTestRunnerIsRecognised")

m("TB the .cpp and .cxx GoogleTest spellings are not test files", "internal/shape/label.go",
  "\t\t\t\"_test.cpp\", \"_unittest.cpp\", \"_test.cxx\",\n",
  "",
  "TestLabelTestFile")

m("TB a test call's outcome is linkOutcome's, not the timeline's", "internal/report/testbending.go",
  "\to := timelineOutcome(d.ToolUseID, rec, executed, denied)\n",
  "\to, _, _ := linkOutcome(d.ToolUseID, executed, denied)\n",
  "TestDetectTestBending_|TestTestBending")

m("TB the --timeline hint names a placeholder, not the session", "internal/report/text.go",
  "\twriteTestRuns(b, sess.TestRuns, sess.SessionID)\n",
  "\twriteTestRuns(b, sess.TestRuns, \"<id>\")\n",
  "TestTestBending_OnlyTestFilesEditedThenGreen")
m("TB the --timeline hint pastes the id unquoted", "internal/report/text.go",
  "\t\t\treturn \"'\" + strings.ReplaceAll(s, \"'\", `'\\''`) + \"'\"\n",
  "\t\t\treturn s\n",
  "TestTestRuns_CountsAndText")

m("TB curl's : takes an argument", "internal/shape/shape.go",
  "'0' <= c && c <= '9', c == '#', c == ':':\n",
  "'0' <= c && c <= '9', c == '#':\n",
  "TestMayWrite")
m("TB -Dmaven.test.skip runs the tests", "internal/shape/shape.go",
  "\tcase \"-DskipTests\", \"-Dmaven.test.skip\", \"-Dmaven.test.skip.exec\":\n",
  "\tcase \"-DskipTests\", \"-Dmaven.test.skip.exec\":\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB an empty session id pastes as nothing", "internal/report/text.go",
  "\tif s == \"\" {\n\t\treturn \"''\"\n\t}\n",
  "",
  "TestTestRuns_CountsAndText")

m("TB the unobserved line says backgrounded calls ended", "internal/report/text.go",
  "recorded no ending or were moved to the background before they ended\\n",
  "recorded no ending or ended in the background\\n",
  "TestSilentFailures_")

m("TB the Cursor report command drops a flag", ".cursor/commands/rashomon-report.md",
  "`--chain` (each call",
  "chain (each call",
  "TestH76_TheReportSkillCopiesListOneFlagSet")

m("TB a call whose declaration was lost does not stop a pair", "internal/report/testbending.go",
  "\tif undeclaredCalls(run, denied) > 0 {\n\t\treturn out\n\t}\n",
  "",
  "TestTestBending")
m("TB a lost call with only a terminal does not stop a pair", "internal/report/testbending.go",
  "\tfor _, t := range run.Terminals {\n\t\tif t.FlushFailed() && t.ToolUseID == \"\" {\n\t\t\tcontinue\n\t\t}\n\t\tif !declared[t.ToolUseID] && !denied[t.ToolUseID] {\n\t\t\tlost[t.ToolUseID] = true\n\t\t}\n\t}\n",
  "",
  "TestTestBending")
m("TB a lost call's terminal is counted apart from its execution record", "internal/report/testbending.go",
  "\t\t\tlost[t.ToolUseID] = true\n",
  "\t\t\tlost[\"terminal \"+t.ToolUseID] = true\n",
  "TestTestRuns_")
m("TB the test runs never count the lost calls", "internal/report/testbending.go",
  "\tout := &TestRuns{Undeclared: undeclaredCalls(run, denied), TestBending: tb}\n",
  "\tout := &TestRuns{TestBending: tb}\n",
  "TestTestRuns_")
m("TB the session block does not say why no pair is looked for", "internal/report/text.go",
  "\tif t.Undeclared > 0 {\n\t\tfmt.Fprintf(b, \"    no pair is looked for",
  "\tif false {\n\t\tfmt.Fprintf(b, \"    no pair is looked for",
  "TestTestRuns_")

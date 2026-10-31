m("TB a test runner is never recognised", "internal/shape/shape.go",
  "\t\t\tif runsTests(pshaped, i, prog, perr == nil) && !backgrounded(toolInput) {",
  "\t\t\tif false && runsTests(pshaped, i, prog, perr == nil) && !backgrounded(toolInput) {",
  "TestTestRunnerIsRecognised|TestTestClassCarriesNoContent|TestTestBending_")
m("TB a line the lexer could not finish is vouched for", "internal/shape/verb.go",
  "\tif !whole {\n\t\treturn false\n\t}", "\t_ = whole", "TestTestRunnerIsRecognised")
m("TB a quoted argument is compared as plain", "internal/shape/runner.go",
  "\t\tif t.quotedAt >= 0 || t.text != want || runsOn(toks, k) {",
  "\t\tif t.text != want || runsOn(toks, k) {", "TestTestRunnerIsRecognised")
m("TB an argument running into a process substitution is plain", "internal/shape/runner.go",
  "\t\tif t.quotedAt >= 0 || t.text != want || runsOn(toks, k) {",
  "\t\tif t.quotedAt >= 0 || t.text != want {", "TestTestRunnerIsRecognised")
m("TB every word of a runner is compared against the first argument", "internal/shape/runner.go",
  "\t\tk := from + n\n", "\t\tk := from + n*0\n", "TestTestRunnerIsRecognised")
# Re-anchored after the masked-status refactor: the row match moved into
# runnerOn, which compares one command's words up to its end; and again when
# the comparison at one position moved into rowAt.
m("TB a runner missing its argument still matches", "internal/shape/runner.go",
  "\t\tif k >= end {\n\t\t\treturn false\n", "\t\tif k >= end {\n\t\t\tbreak\n", "TestTestRunnerIsRecognised")
m("TB go test is not on the list", "internal/shape/runner.go",
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
m("TB a run at an unknown location is marked", "internal/report/testbending.go",
  "\t\tif isRun && d.CWDDigest != \"\" {\n\t\t\tlast[key] = mark{", "\t\tif isRun {\n\t\t\tlast[key] = mark{",
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
m("TB the report never builds the test runs", "internal/report/assemble.go",
  "\tsess.TestRuns = buildTestRuns(run, executed, denied, tb)\n",
  "\t_ = buildTestRuns\n",
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
  "\t\tfirst, len(tb.TestsOnlyThenGreen)+tb.TestsOnlyThenGreenOmitted); ok && d.Fired(report.DetectorTestsOnlyThenGreen) {\n\t\tsentences = append(sentences, s)",
  "\t\tfirst, len(tb.TestsOnlyThenGreen)+tb.TestsOnlyThenGreenOmitted); ok && d.Fired(report.DetectorTestsOnlyThenGreen) {\n\t\t_ = s",
  "TestLineTestBending|TestTestBending_")
m("TB the end-of-turn line drops the flaky sentence", "internal/recap/recap.go",
  "\t\tfirst, len(tb.Flaky)+tb.FlakyOmitted); ok && d.Fired(report.DetectorFlakyPair) {\n\t\tsentences = append(sentences, s)",
  "\t\tfirst, len(tb.Flaky)+tb.FlakyOmitted); ok && d.Fired(report.DetectorFlakyPair) {\n\t\t_ = s",
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
m("TB a runner piped or listed into another command is a test run", "internal/shape/verb.go",
  "return on && !hides && wholeCommand(toks, i)\n",
  "return on && !hides && wholeCommand(toks[:i+1], i)\n",
  "TestTestRunnerIsRecognised|TestTestBending_ShapesFromDerive")
m("TB a command on the runner's next line is not seen", "internal/shape/verb.go",
  "\t\tif t.nlBefore {\n\t\t\treturn false\n\t\t}\n\t}\n\treturn closesGroups(toks, i)\n}",
  "\t\tif t.nlBefore && false {\n\t\t\treturn false\n\t\t}\n\t}\n\treturn closesGroups(toks, i)\n}",
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

m("TB make check is a test run", "internal/shape/runner.go",
  "{\"gradlew\", \"test\"}, {\"make\", \"test\"},\n",
  "{\"gradlew\", \"test\"}, {\"make\", \"test\"}, {\"make\", \"check\"},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
# Re-anchored after the masked-status refactor: the refusal moved into runnerOn,
# over the runner's own command (toks up to end); the next two as well.
m("TB an argument that does not run the tests is ignored", "internal/shape/runner.go",
  "\t\tif refusesRun(refusalsOf(c), toks[:i], args) {",
  "\t\tif false && refusesRun(refusalsOf(c), toks[:i], args) {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB the refusal reads only the first argument", "internal/shape/runner.go",
  "\t\tif refusesRun(refusalsOf(c), toks[:i], args) {",
  "\t\tif refusesRun(refusalsOf(c), toks[:i], args[:min(1, len(args))]) {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a prefix entry is compared whole", "internal/shape/verb.go",
  "\t\treturn strings.HasPrefix(word, p)\n",
  "\t\treturn word == p\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB the per-runner list is not read", "internal/shape/verb.go",
  "\tlist := notARun[strings.TrimSuffix(runner, \" build\")]\n",
  "\tlist := []string(nil)\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB help, version and watch refuse nothing", "internal/shape/verb.go",
  "|| onList(t.text, notARunAny)",
  "|| onList(t.text, nil)",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB tox with a target is a test run", "internal/shape/verb.go",
  "\"tox\": {\"-e*\", ",
  "\"tox\": {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB nox with a session is a test run", "internal/shape/verb.go",
  "\"nox\": {\"-s*\", ",
  "\"nox\": {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB go test -c is a test run", "internal/shape/verb.go",
  "\"go\":      {\"-c\", ",
  "\"go\":      {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB pytest --collect-only is a test run", "internal/shape/verb.go",
  "\t\"--collect-only\", \"--co\", ",
  "\t",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB cargo test --no-run is a test run", "internal/shape/verb.go",
  "\"cargo\":   {\"--no-run\", ",
  "\"cargo\":   {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB jest --listTests is a test run", "internal/shape/verb.go",
  "\"jest\":    {\"--listTests\", ",
  "\"jest\":    {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB watch mode is a test run", "internal/shape/verb.go",
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

m("TB a flag's =value form is not refused", "internal/shape/verb.go",
  "\treturn word == w || hasValue && strings.HasPrefix(w, \"-\") && name == w && !isFalse(value)\n",
  "\treturn word == w || false && hasValue && strings.HasPrefix(w, \"-\") && name == w && !isFalse(value)\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a flag set =false is refused", "internal/shape/verb.go",
  "\treturn word == w || hasValue && strings.HasPrefix(w, \"-\") && name == w && !isFalse(value)\n",
  "\treturn word == w || hasValue && strings.HasPrefix(w, \"-\") && name == w && (!isFalse(value) || true)\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB make test -n is a test run", "internal/shape/verb.go",
  "\"make\":    {\"-n\", \"--just-print\", \"--dry-run\", \"--recon\", \"-q\", \"--question\", ",
  "\"make\":    {\"--recon\", ",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB make test -v is a test run", "internal/shape/verb.go",
  "\"-t\", \"--touch\", \"-v\"},\n",
  "\"-t\", \"--touch\"},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB gradle continuous mode is a test run", "internal/shape/verb.go",
  "\"gradle\":  {\"--dry-run\", \"-m\", \"-v\", \"-t\", \"--continuous\"},",
  "\"gradle\":  {\"--dry-run\", \"-m\", \"-v\"},",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB mvn test -v is a test run", "internal/shape/verb.go",
  "\t\"mvn\":     {\"-v\"},\n",
  "\t\"mvn\":     {},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB tox --notest is a test run", "internal/shape/verb.go",
  "\"--help-ini\", \"--notest\",",
  "\"--help-ini\",",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB tox devenv is a test run", "internal/shape/verb.go",
  ", \"devenv\", \"d\"},",
  "},",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB nox --install-only is a test run", "internal/shape/verb.go",
  ", \"--install-only\"},",
  "},",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB PYTEST_ADDOPTS on the line is not read", "internal/shape/verb.go",
  "\t\t\tif strings.HasPrefix(t.text, \"PYTEST_ADDOPTS=\") {",
  "\t\t\tif false && strings.HasPrefix(t.text, \"PYTEST_ADDOPTS=\") {",
  "TestTestClassRefusesWhatDoesNotRunTests")
# Re-anchored after the masked-status refactor: the refusal is in runnerOn.
m("TB the words before the runner are not read", "internal/shape/runner.go",
  "\t\tif refusesRun(refusalsOf(c), toks[:i], args) {",
  "\t\tif refusesRun(refusalsOf(c), toks[i:i], args) {",
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

m("TB the report's timeline is built without the patterns", "internal/report/assemble.go",
  "\tsess.Timeline = timelineFrom(run, executed, denied, tb)\n",
  "\tsess.Timeline = timelineFrom(run, executed, denied, TestBending{})\n",
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

# Re-anchored when the call records moved to schema 5: the constant is now
# CallSchemaVersion's name, and the mutation is still "the writer stays at an
# older version". REC-1 owns the one-version-back case.
m("TB the writer stays at schema 3", "internal/store/record.go",
  "const SchemaVersion = CallSchemaVersion",
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
  "\tdecl.Shape, decl.CWDDigest, decl.LocationUnread = locate(h.st.Key(), p.ToolName, p.ToolInput, p.CWD, physical)\n",
  "\tdecl.Shape, _, decl.LocationUnread = locate(h.st.Key(), p.ToolName, p.ToolInput, p.CWD, physical)\n",
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
m("TB find -delete and -exec do not write", "internal/shape/verb.go",
  "var findWriteWords = []string{\"-delete\", \"-exec\", \"-execdir\", \"-ok\", \"-okdir\"}\n",
  "var findWriteWords = []string{\"-okdir\"}\n",
  "TestMayWrite|TestTestBending_WriteCapable")
m("TB xargs, tee, rsync and scp do not write", "internal/shape/verb.go",
  "var inlineWriters = []string{\"xargs\", \"tee\", \"wget\", \"rsync\", \"scp\"}\n",
  "var inlineWriters = []string{\"wget\"}\n",
  "TestMayWrite|TestTestBending_WriteCapable")
m("TB curl with an output flag does not write", "internal/shape/verb.go",
  "\tif curl && curlOut {\n",
  "\tif false && curl && curlOut {\n",
  "TestMayWrite|TestTestBending_WriteCapable")
m("TB a later stage is not looked at", "internal/shape/verb.go",
  "\t\tif !ok || verbForProgram(path.Base(toks[start+k].text)) != VerbRead {\n",
  "\t\tif false && (!ok || verbForProgram(path.Base(toks[start+k].text)) != VerbRead) {\n",
  "TestMayWrite")
m("TB a descriptor duplication is a write", "internal/shape/verb.go",
  "\tif dup && (target == \"-\" || isFDPrefix(toks[end+1])) {\n",
  "\tif false && dup && (target == \"-\" || isFDPrefix(toks[end+1])) {\n",
  "TestMayWrite")
m("TB /dev/null is a file written", "internal/shape/verb.go",
  "\tcase \"/dev/null\", \"/dev/stdout\", \"/dev/stderr\", \"/dev/tty\":\n",
  "\tcase \"/dev/stdout\", \"/dev/stderr\", \"/dev/tty\":\n",
  "TestMayWrite")
m("TB the & of 2>&1 separates stages", "internal/shape/verb.go",
  "(toks[j-1].text == \">\" || toks[j-1].text == \">>\" || toks[j-1].text == \"|\")",
  "(toks[j-1].text == \"|\")",
  "TestMayWrite")
m("TB the session block drops what makes a read an edit", "internal/report/text.go",
  "\t\tfmt.Fprintln(b, \"    a shell read or fetch counts when its line may write: a redirect to a file, a download (curl -o, attached or not), a command or process substitution, find -delete or -exec, xargs, tee, rsync or scp, or a later stage that is not a read,\")\n",
  "",
  "TestTestRuns_")

m("TB a runner prefix is not stepped past", "internal/shape/verb.go",
  "\ti, prog, ok := runnerPrefix(toks, i, prog)\n",
  "\t_, _, ok := runnerPrefix(toks, i, prog)\n",
  "TestTestRunnerIsRecognised")
m("TB time is not stepped past", "internal/shape/verb.go",
  "\tif prog == \"time\" {\n",
  "\tif false && prog == \"time\" {\n",
  "TestTestRunnerIsRecognised")
m("TB timeout takes any word for its duration", "internal/shape/verb.go",
  "\t\tif !plain(i+1) || !isDuration(toks[i+1].text) || !word(i+2) {\n",
  "\t\tif !plain(i+1) || !word(i+2) {\n",
  "TestTestRunnerIsRecognised")
m("TB a wrapped runner is refused on the wrapper's list", "internal/shape/verb.go",
  "\tswitch c[0] {\n\tcase \"npx\", \"uv\", \"poetry\", \"bundle\":\n",
  "\tswitch \"\" {\n\tcase \"npx\", \"uv\", \"poetry\", \"bundle\":\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB the npx, uv, poetry and bundle forms are off the list", "internal/shape/runner.go",
  "\t{\"npx\", \"jest\"}, {\"npx\", \"vitest\"}, {\"uv\", \"run\", \"pytest\"}, {\"poetry\", \"run\", \"pytest\"},\n\t{\"bundle\", \"exec\", \"rspec\"},\n",
  "",
  "TestTestRunnerIsRecognised")
m("TB gradlew and mvnw are off the list", "internal/shape/runner.go",
  "{\"mvnw\", \"test\"}, {\"gradle\", \"test\"}, {\"gradlew\", \"test\"}, ",
  "{\"gradle\", \"test\"}, ",
  "TestTestRunnerIsRecognised")
m("TB npm t is off the list", "internal/shape/runner.go",
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

m("TB curl's attached output file hides the flag", "internal/shape/verb.go",
  "\t\t\tcase c == 'o' || c == 'O':\n\t\t\t\treturn true\n\t\t\tcase strings.ContainsRune(curlArgumentOptions, c):\n\t\t\t\treturn false\n\t\t\tcase 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '#', c == ':':\n\t\t\tdefault:\n\t\t\t\treturn false\n\t\t\t}\n\t\t}\n\t}\n\treturn false\n}\n",
  "\t\t\tcase c == 'o' || c == 'O':\n\t\t\tcase strings.ContainsRune(curlArgumentOptions, c):\n\t\t\t\treturn false\n\t\t\tcase 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '#', c == ':':\n\t\t\tdefault:\n\t\t\t\treturn false\n\t\t\t}\n\t\t}\n\t\treturn strings.ContainsAny(w[1:], \"oO\")\n\t}\n\treturn false\n}\n",
  "TestMayWrite|TestTestBending_AnAttachedCurl")
m("TB curl's digits, # and : end the short-flag walk", "internal/shape/verb.go",
  "\t\t\tcase 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '#', c == ':':\n",
  "\t\t\tcase 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':\n",
  "TestMayWrite|TestTestBending_ACurlProgressBar")
m("TB curl's argument-taking options are read as more flags", "internal/shape/verb.go",
  "\t\t\tcase strings.ContainsRune(curlArgumentOptions, c):\n\t\t\t\treturn false\n",
  "",
  "TestMayWrite")
m("TB a command or process substitution is not a write", "internal/shape/verb.go",
  "\t\tif opensSubstitution(toks, j) {\n",
  "\t\tif false && opensSubstitution(toks, j) {\n",
  "TestMayWrite")
m("TB backticks are not a write", "internal/shape/verb.go",
  "\t\tif t.ticks > 0 || t.opaque && holdsSubstitution(t.text) {\n",
  "\t\tif t.opaque && holdsSubstitution(t.text) {\n",
  "TestMayWrite")
m("TB a quoted $( is not a write", "internal/shape/verb.go",
  "\t\tif t.ticks > 0 || t.opaque && holdsSubstitution(t.text) {\n",
  "\t\tif t.ticks > 0 {\n",
  "TestMayWrite")
m("TB a single-quoted $( is a write", "internal/shape/verb.go",
  "\t\tif t.ticks > 0 || t.opaque && holdsSubstitution(t.text) {\n",
  "\t\tif t.ticks > 0 || holdsSubstitution(t.text) {\n",
  "TestMayWrite")
m("TB arithmetic is a command substitution", "internal/shape/verb.go",
  "\treturn strings.HasSuffix(t.text, \"$\") && !(gluedParen(toks, j+2) && arithmeticAt(parenText(toks[j+3:])))\n",
  "\treturn strings.HasSuffix(t.text, \"$\")\n",
  "TestMayWrite")
m("TB a $(( that does not close as )) is arithmetic", "internal/shape/verb.go",
  "\t\t\t\treturn strings.HasPrefix(s[k+1:], \")\")\n",
  "\t\t\t\treturn true\n",
  "TestMayWrite")
m("TB quoted arithmetic is a command substitution", "internal/shape/verb.go",
  "\t\tif !strings.HasPrefix(s[k+2:], \"(\") || !arithmeticAt(s[k+3:]) {\n",
  "\t\tif true {\n",
  "TestMayWrite")
m("TB a ( inside arithmetic opens no group", "internal/shape/verb.go",
  "\t\tcase '(':\n\t\t\tdepth++\n",
  "",
  "TestMayWrite")
m("TB a quoted ) inside $(( closes it", "internal/shape/verb.go",
  "\t\t\tb.WriteByte('w')\n",
  "\t\t\tb.WriteString(t.text)\n",
  "TestMayWrite")

m("TB a backgrounded call is not counted as unobserved", "internal/report/account.go",
  "\t\tif x.Backgrounded {\n\t\t\t// Recorded ok",
  "\t\tif false && x.Backgrounded {\n\t\t\t// Recorded ok",
  "TestSilentFailures_")

m("TB a leading cd's target is not the run's directory", "internal/hook/handle.go",
  "\treturn s, shape.CWDDigest(key, physicalDir(startDirectory(cwd, dirs))), false\n",
  "\t_ = dirs\n\treturn s, shape.CWDDigest(key, physicalDir(cwd)), false\n",
  "TestTestBending_ALeadingAbsoluteCd")
m("TB a relative leading cd keeps the shell's directory", "internal/hook/handle.go",
  "\t\t\tcwd = filepath.Join(cwd, dir)\n",
  "\t\t\treturn cwd\n",
  "TestStartDirectory|TestTestBending_ALeadingRelativeCd")
m("TB a leading run stops at a step it cannot resolve", "internal/shape/leading.go",
  "\t\tif !ok {\n\t\t\treturn nil, false\n\t\t}\n\t\tdirs = append(dirs, dir)\n",
  "\t\tif !ok {\n\t\t\treturn dirs, len(dirs) > 0\n\t\t}\n\t\tdirs = append(dirs, dir)\n",
  "TestLeadingDirectory|TestTestBending_ACdBack")
m("TB a leading pushd or popd is not a directory change", "internal/shape/leading.go",
  "\t\tcase \"cd\", \"pushd\", \"popd\":\n",
  "\t\tcase \"cd\":\n",
  "TestLeadingDirectory")
m("TB an assignment in front of a later cd is not a directory change", "internal/shape/leading.go",
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

m("TB Maven's skip properties are not refused", "internal/shape/verb.go",
  " || runner == \"mvn\" && mavenSkip(t.text) {\n",
  " {\n",
  "TestTestClassRefusesWhatDoesNotRunTests|TestTestRunnerIsRecognised")
m("TB a Maven skip property refuses its =false form", "internal/shape/verb.go",
  "(!hasValue || strings.EqualFold(value, \"true\"))\n",
  "(!hasValue || len(value) >= 0)\n",
  "TestTestRunnerIsRecognised")
m("TB a Maven skip property is read as true only in lower case", "internal/shape/verb.go",
  "(!hasValue || strings.EqualFold(value, \"true\"))\n",
  "(!hasValue || value == \"true\")\n",
  "TestTestRunnerIsRecognised")
m("TB a prefix entry takes isFalse's exception", "internal/shape/verb.go",
  "\t\treturn strings.HasPrefix(word, p)\n",
  "\t\tv, ok := strings.CutPrefix(strings.TrimPrefix(word, p), \"=\")\n\t\treturn strings.HasPrefix(word, p) && !(ok && isFalse(v))\n",
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

m("TB curl's : takes an argument", "internal/shape/verb.go",
  "'0' <= c && c <= '9', c == '#', c == ':':\n",
  "'0' <= c && c <= '9', c == '#':\n",
  "TestMayWrite")
m("TB -Dmaven.test.skip runs the tests", "internal/shape/verb.go",
  "{\"-DskipTests\", \"-Dmaven.test.skip\", \"-Dmaven.test.skip.exec\"}\n",
  "{\"-DskipTests\", \"-Dmaven.test.skip.exec\"}\n",
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

# A line holding a control byte names no leading directory to the hook,
# which reads them through DeriveWithDirs, as to LeadingDirectory: the shell
# does not split it where the tokenizer does. Folding its cds is the
# regression.
m("TB DeriveWithDirs folds the leading cds of a control-byte line", "internal/shape/shape.go",
  "\tif !ctl {\n\t\tdirs, _ = leadingRun(pshaped, perr)\n\t}\n",
  "\tdirs, _ = leadingRun(pshaped, perr)\n",
  "TestLeadingDirectory|TestCaptureKeepsTheCWDOfAControlByteLine")
m("TB LeadingDirectory reads the cds of a control-byte line", "internal/shape/leading.go",
  "\tif !ok || controlByte(cmd) {\n\t\treturn nil, false\n\t}\n\treturn leadingDirectories(cmd)\n",
  "\tif !ok {\n\t\treturn nil, false\n\t}\n\treturn leadingDirectories(cmd)\n",
  "TestLeadingDirectory")

# The in-place letter inside a cluster of short options, and the words that
# clear a formatter. Each table mutant runs TestRC3b_ alone, so the row it is
# written for kills it; TestRulesVersion_WriteTables would also kill each,
# by the digest, and is the RulesVersion mutant's own killer. The emptied
# table and the scan never called are equivalent for these rows (both leave
# every cluster unread); both stay, one per line they break.
m("TB no editor's short options are scanned as a cluster", "internal/shape/verb.go",
  "var shortClusters = map[string]shortCluster{\n\t\"sed\":  {letters: \"iI\", consumes: \"ef\"},\n\t\"perl\": {letters: \"i\", consumes: \"eEMmIFx\"},\n}\n",
  "var shortClusters = map[string]shortCluster{}\n", "TestRC3b_")
m("TB sed's -l takes the rest of the word again", "internal/shape/verb.go",
  "\t\"sed\":  {letters: \"iI\", consumes: \"ef\"},\n", "\t\"sed\":  {letters: \"iI\", consumes: \"efl\"},\n", "TestRC3b_")
m("TB BSD sed's I is not an in-place letter in a cluster", "internal/shape/verb.go",
  "\t\"sed\":  {letters: \"iI\", consumes: \"ef\"},\n", "\t\"sed\":  {letters: \"i\", consumes: \"ef\"},\n", "TestRC3b_")
m("TB BSD sed's -I is not an in-place word", "internal/shape/verb.go",
  "\t\"sed\":          {\"-i*\", \"-I*\", \"--in-place*\"},\n", "\t\"sed\":          {\"-i*\", \"--in-place*\"},\n", "TestRC3b_")
m("TB a consuming letter does not end the cluster scan", "internal/shape/verb.go",
  "\t\tcase strings.IndexByte(c.consumes, tok[i]) >= 0:\n\t\t\treturn false\n", "", "TestRC3b_")
m("TB a clustered editor's words are never scanned", "internal/shape/verb.go",
  "\t\tcase clustered && clusterHas(t.text, cluster):\n",
  "\t\tcase false && clustered && clusterHas(t.text, cluster):\n", "TestRC3b_")
m("TB --version does not clear an editor", "internal/shape/verb.go",
  "\"--help\", \"--version\", \"-h\"}", "\"--help\", \"-h\"}", "TestRC3b_")
m("TB --help does not clear an editor", "internal/shape/verb.go",
  "\"--help\", \"--version\", \"-h\"}", "\"--version\", \"-h\"}", "TestRC3b_")
m("TB -h does not clear an editor", "internal/shape/verb.go",
  "\"--help\", \"--version\", \"-h\"}", "\"--help\", \"--version\"}", "TestRC3b_")
m("TB the rules version stays at 2 when the in-place tables change", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 2\n", "TestRulesVersion_")

# The flags that may stand before a runner's row words (leadingFlags), and the
# spellings that make a failed run end 0 (notARun). The RulesVersion mutant
# is the matcher pin's own: no table the three table pins digest moved, so
# each of them still finds its version-3 digest under key 3.
m("TB every dash word may stand before the row's words", "internal/shape/runner.go",
  "\tt := toks[k]\n\tfor _, f := range leadingFlags[prog] {\n",
  "\tt := toks[k]\n\tif strings.HasPrefix(t.text, \"-\") {\n\t\treturn true\n\t}\n\tfor _, f := range leadingFlags[prog] {\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB no flag may stand before the row's words", "internal/shape/runner.go",
  "var leadingFlags = map[string][]string{\n",
  "var leadingFlags = map[string][]string{}\n\nvar _ = map[string][]string{\n",
  "TestTestRunnerIsRecognised|TestLeadingOptionsOneMatcher")
m("TB make's -n may stand before the row's words", "internal/shape/runner.go",
  "\t\"make\":    {\"-j#\", \"-i!\", \"--ignore-errors!\"},\n",
  "\t\"make\":    {\"-j#\", \"-n\", \"-i!\", \"--ignore-errors!\"},\n",
  "TestLeadingFlagsAreNotRefusals|TestTestClassRefusesWhatDoesNotRunTests")
m("TB a Maven property prefix may stand before the row's words", "internal/shape/runner.go",
  "\tmavenLeadingFlags  = []string{\"-B\", \"-q\", \"-fn!\", \"--fail-never!\", \"-Dmaven.test.failure.ignore!\"}\n",
  "\tmavenLeadingFlags  = []string{\"-B\", \"-q\", \"-fn!\", \"--fail-never!\", \"-Dmaven.test.failure.ignore!\", \"-D*\"}\n",
  "TestLeadingFlagsAreNotRefusals")
m("TB python3 has no leading flags of its own", "internal/shape/runner.go",
  "\t\"python3\": pythonLeadingFlags,\n", "",
  "TestTestRunnerIsRecognised|TestLeadingOptionsOneMatcher")
m("TB a toolchain word may stand before any program's row", "internal/shape/runner.go",
  "\tt := toks[k]\n\tfor _, f := range leadingFlags[prog] {\n",
  "\tt := toks[k]\n\tfor _, f := range append(leadingFlags[prog], \"+*\") {\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a quoted word is read as a listed flag", "internal/shape/runner.go",
  "\treturn !(t.meta || t.quotedAt >= 0 || t.opaque || t.ticks > 0 || runsOn(toks, k) ||\n",
  "\treturn !(t.meta || t.opaque || t.ticks > 0 || runsOn(toks, k) ||\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB an expansion is read as a listed flag", "internal/shape/runner.go",
  "\t\tstrings.ContainsAny(t.text, \"$`*?[{\\\\\"))\n}\n",
  "\t\tfalse)\n}\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB make's -i hides nothing", "internal/shape/runner.go",
  "\t\"make\":    {\"-j#\", \"-i!\", \"--ignore-errors!\"},\n",
  "\t\"make\":    {\"-j#\", \"--ignore-errors!\"},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB npm's --if-present hides nothing", "internal/shape/runner.go",
  "\t\"npm\":     {\"--if-present!\"},\n",
  "\t\"npm\":     {},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB mvn's -fn hides nothing", "internal/shape/runner.go",
  "\"-B\", \"-q\", \"-fn!\", \"--fail-never!\",",
  "\"-B\", \"-q\", \"--fail-never!\",",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB -Dmaven.test.failure.ignore hides nothing", "internal/shape/runner.go",
  "\"--fail-never!\", \"-Dmaven.test.failure.ignore!\"}\n",
  "\"--fail-never!\"}\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB the rules version stays at 3 when a matcher changes", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 3\n", "TestRulesVersion_Matchers")

# Version 5: the masked parser passes over the leading cd steps the program
# search steps over, `~` is read, and a fold that is not read has a location
# of its own (shape.UnreadCWDDigest, hook.Locate). Leaving the version at 4,
# or giving an unread fold a read location, is the regression.
m("TB the rules version stays at 4 when the fold and the location rule change", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 4\n", "TestRulesVersion_Matchers|TestRulesVersion_Locations")
m("TB an unread fold's location is the payload cwd's", "internal/shape/shape.go",
  "\treturn digest(key, \"\\x00cwd-unread\", cwd+\"\\x00\"+fold)\n", "\treturn CWDDigest(key, cwd)\n",
  "TestLocation|TestUnreadLocationDomain")
m("TB an unread fold's location drops its quoting", "internal/shape/leading.go",
  "\t\tif t.quotedAt >= 0 {\n\t\t\tq = 'q'\n\t\t}\n", "",
  "TestUnreadLocationDomain|TestDeriveWithDirsUnread")
m("TB an unread fold is counted to the fold's bound", "internal/shape/leading.go",
  "\tafter, _, _, _ := directoryChanges(toks, i, uncertain)\n\tif after == 0 {\n",
  "\t_ = i\n\tafter := leadingFold(toks, uncertain)\n\tif after == 0 {\n",
  "TestLocation|TestDeriveWithDirsUnread")
m("TB a leading cd before ; is read as a location", "internal/shape/leading.go",
  "\tif !and.meta || and.text != \"&&\" || and.nlBefore {\n",
  "\tif !and.meta || and.text != \"&&\" && and.text != \";\" || and.nlBefore {\n",
  "TestLocation|TestLeadingDirectory")
m("TB an unread fold with no payload cwd is keyed on the fold alone", "internal/hook/handle.go",
  "\t\tif cwd == \"\" {\n\t\t\treturn s, \"\", true\n\t\t}\n\t\treturn s, shape.UnreadCWDDigest(key, cwd, unread), true\n",
  "\t\treturn s, shape.UnreadCWDDigest(key, cwd, unread), true\n", "TestLocation")
m("TB ~ is not expanded with the hook's home", "internal/hook/handle.go",
  "\t\tcase dir == \"~\" || strings.HasPrefix(dir, \"~/\"):\n", "\t\tcase false:\n",
  "TestLocation|TestStartDirectory")

# Version 6: -l leaves checkOnly (gofmt's -l lists files and does not stop a
# -w beside it; for sed, perl, black and isort it is not a check at all),
# make's -j admits digits only, and the flag tables are under a pin. Each
# comment names the test that kills the mutant and the assertion that fails.
#
# -l put back: TestRC3b_InPlaceEditorsSetMayWrite, `"gofmt -l -w .": may_write
# false, want true` (and the sed, perl, black and isort rows beside it).
# `gofmt -l .` stays a read in the same test: no word arms the stage.
m("TB -l clears an editor again", "internal/shape/verb.go",
  "\"--diff\", \"--list-different\"", "\"--diff\", \"-l\", \"--list-different\"",
  "TestRC3b_")
m("TB the rules version stays at 5 when -l leaves checkOnly", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 5\n", "TestRulesVersion_")

# The -j# entry: TestTestClassRefusesWhatDoesNotRunTests, whose rows `make
# -j4x test` and `make -j=4 test` read verb_class test where execute is
# wanted (the glob restored), `make -j test` and `make -j 4 test` read execute
# where test is wanted (a count required; the separate count not passed over).
m("TB make's -j admits whatever follows it", "internal/shape/runner.go",
  "\t\"make\":    {\"-j#\", \"-i!\", \"--ignore-errors!\"},\n",
  "\t\"make\":    {\"-j*\", \"-i!\", \"--ignore-errors!\"},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB make's -j needs a count", "internal/shape/runner.go",
  "ok && allDigits(n) {", "ok && n != \"\" && allDigits(n) {",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a digits entry admits a letter after the digits", "internal/shape/runner.go",
  "\t\tif s[i] < '0' || '9' < s[i] {\n", "\t\tif s[i] < '0' || 'z' < s[i] {\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB make's separate job count stops the match", "internal/shape/runner.go",
  "\t\t\tif from < end && countFollows(toks, from-1, prog) {\n",
  "\t\t\tif false && from < end && countFollows(toks, from-1, prog) {\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB any word after make's -j is its count", "internal/shape/runner.go",
  "\treturn literalWord(toks, k+1) && n != \"\" && allDigits(n)\n", "\treturn literalWord(toks, k+1) && n != \"\"\n",
  "TestTestClassRefusesWhatDoesNotRunTests")

# A flag table edited with RulesVersion left as it is: TestRulesVersion_FlagTables,
# `the flag tables digest to ..., and RulesVersion 7 pins ...: a table changed
# without a bump`. No behaviour row reads the words these drop, so the pin
# alone kills each.
m("TB the flag tables stay at the same pin when notARunAny drops --watchAll", "internal/shape/verb.go",
  "\"--version\", \"--watch\", \"--watchAll\"}", "\"--version\", \"--watch\"}",
  "TestRulesVersion_FlagTables")
m("TB the flag tables stay at the same pin when make leaves --just-print out", "internal/shape/verb.go",
  "\"-n\", \"--just-print\", \"--dry-run\", \"--recon\"", "\"-n\", \"--dry-run\", \"--recon\"",
  "TestRulesVersion_FlagTables")
m("TB the flag tables stay at the same pin when curl's argument options change", "internal/shape/verb.go",
  "const curlArgumentOptions = \"AbcCdDeEFhHKmPQrtTuUwxXyYz\"", "const curlArgumentOptions = \"AbcCdDeEFhHKmPQrtTuUwxXyY\"",
  "TestRulesVersion_FlagTables")
m("TB the flag tables stay at the same pin when make's -j entry changes", "internal/shape/runner.go",
  "\t\"make\":    {\"-j#\", \"-i!\", \"--ignore-errors!\"},\n",
  "\t\"make\":    {\"-j#\", \"-i!\", \"--ignore-errors!\", \"-s\"},\n",
  "TestRulesVersion_FlagTables")

# 5.9 and 5.10 (card S1). The digit run: TestS1_EditorWordsAndPerlDigitRun,
# `"perl -0777pi -e s/x/y/ a.go": may_write false, want true` (and -00pi).
# The group count lives in wholeCommand's closesGroups: each rule's rows in
# TestS1_WholeCommandCountsGroupsAndComments read test where package is
# wanted, or package where test is. TestS1_CommandEndReadersUnmoved kills the
# last two: the count moved into commandEnd moves the cd step search's reading
# (program <nil>), and a `)` before the runner that closes a group it never
# opened refuses `cd /a) && go test ./...` (class package).
m("TB perl's -0 digit run hides the in-place letter after it", "internal/shape/verb.go",
  "tok[1] <= 'Z' || tok[1] == '0') ||", "tok[1] <= 'Z') ||",
  "TestS1_EditorWordsAndPerlDigitRun")
m("TB a ( opened before the runner need not close", "internal/shape/program.go",
  "\t\t\treturn sep < 0 && parens == 0 && braces == 0\n", "\t\t\treturn sep < 0 && braces == 0\n",
  "TestS1_WholeCommandCountsGroupsAndComments")
m("TB a ) after the runner that closes nothing is passed over", "internal/shape/program.go",
  "\t\tcase t.text == \")\" && parens == 0:\n\t\t\treturn 0, false\n\t\tcase t.text == \")\":\n\t\t\tparens--\n",
  "\t\tcase t.text == \")\" && parens > 0:\n\t\t\tparens--\n",
  "TestS1_WholeCommandCountsGroupsAndComments")
# Removed by 5.15 (RulesVersion 9): the program stream holds no comment token (lex), so the branch it broke was
# dead and is deleted: "TB a paren after a comment is counted" (closedParens; S1's `#` rule).
m("TB the ; of a brace group's ; } ends the runner's line", "internal/shape/program.go",
  "\t\tif toks[sep].text != \";\" || braces == 0 ||", "\t\tif true || braces == 0 ||",
  "TestS1_WholeCommandCountsGroupsAndComments")
m("TB a ) before the runner closes a group it never opened", "internal/shape/program.go",
  "\t\tcase t.meta && t.text == \")\" && parens > 0:\n", "\t\tcase t.meta && t.text == \")\":\n",
  "TestS1_CommandEndReadersUnmoved")
m("TB the paren count moved into commandEnd", "internal/shape/program.go",
  "\t\tcase t.text == \";\" || t.text == \"&&\" || t.text == \"||\" || t.text == \"|\" || t.text == \"&\":\n\t\t\treturn j, true\n",
  "\t\tcase t.text == \")\":\n\t\t\treturn 0, false\n\t\tcase t.text == \";\" || t.text == \"&&\" || t.text == \"||\" || t.text == \"|\" || t.text == \"&\":\n\t\t\treturn j, true\n",
  "TestS1_CommandEndReadersUnmoved")
m("TB the rules version stays at 6 when wholeCommand counts groups", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 6\n", "TestRulesVersion_Matchers")

# 5.11 and 5.12 (card S2). A hiding flag (leadingFlags, ! entries) keeps the
# runner, masked: TestTestClassRefusesWhatDoesNotRunTests's rows read none or test where
# masked test or build is wanted. pushd folds as cd in every reader:
# TestS2_PushdFoldsAsCd, whose end-to-end row's digest fails when one reading
# lags, and whose popd and stack-form rows fail when either is read.
m("TB make's --ignore-errors hides nothing", "internal/shape/runner.go",
  "\t\"make\":    {\"-j#\", \"-i!\", \"--ignore-errors!\"},\n", "\t\"make\":    {\"-j#\", \"-i!\"},\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB mvn's --fail-never hides nothing", "internal/shape/runner.go",
  "\"-fn!\", \"--fail-never!\", \"-Dmaven", "\"-fn!\", \"-Dmaven", "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a hiding flag after the row's words is not read", "internal/shape/runner.go",
  "\t\tfor _, t := range args {\n\t\t\thides = hides || hidingFlag(t.text, prog)\n\t\t}\n", "",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a hiding flag reads Maven's =1 as true", "internal/shape/verb.go",
  "\treturn name == p && (!hasValue || strings.EqualFold(value, \"true\"))\n",
  "\treturn name == p && (!hasValue || !isFalse(value))\n", "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a hiding runner is a test run", "internal/shape/verb.go",
  "return on && !hides && wholeCommand(toks, i)\n", "return on && (!hides || true) && wholeCommand(toks, i)\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB the masked parser drops the hiding bit", "internal/shape/masked_simple.go",
  "\tc.op, c.runner, c.hides = opRunner, g, hides\n", "\tc.op, c.runner, c.hides = opRunner, g, hides && false\n",
  "TestTestClassRefusesWhatDoesNotRunTests")
m("TB a hiding runner's failure reaches the line", "internal/shape/masked_eval.go",
  "\t\t\tif ev.fail && !c.hides {\n", "\t\t\tif ev.fail {\n", "TestTestClassRefusesWhatDoesNotRunTests")
m("TB popd is stepped over as cd is", "internal/shape/program.go",
  "\tfor (toks[i].text == \"cd\" || toks[i].text == \"pushd\") && toks[i].quotedAt < 0 {\n",
  "\tfor (toks[i].text == \"cd\" || toks[i].text == \"pushd\" || toks[i].text == \"popd\") && toks[i].quotedAt < 0 {\n",
  "TestS2_PushdFoldsAsCd")
m("TB popd DIR is read as a location", "internal/shape/leading.go",
  "\tif cd.text != \"cd\" && cd.text != \"pushd\" || cd.quotedAt >= 0 || cd.opaque {\n",
  "\tif cd.text != \"cd\" && cd.text != \"pushd\" && cd.text != \"popd\" || cd.quotedAt >= 0 || cd.opaque {\n",
  "TestS2_PushdFoldsAsCd")
m("TB pushd is folded by the location reader only", "internal/shape/program.go",
  "\tfor (toks[i].text == \"cd\" || toks[i].text == \"pushd\") && toks[i].quotedAt < 0 {\n",
  "\tfor toks[i].text == \"cd\" && toks[i].quotedAt < 0 {\n", "TestS2_PushdFoldsAsCd")
m("TB pushd is folded by the program search only", "internal/shape/program.go",
  "\tif !slices.Equal(toks[:after], fold) {\n",
  "\tif !slices.Equal(toks[:after], fold) || slices.ContainsFunc(fold, func(t token) bool { return t.text == \"pushd\" }) {\n",
  "TestS2_PushdFoldsAsCd")
m("TB pushd's stack forms are read as a location", "internal/shape/leading.go",
  " || cd.text == \"pushd\" && strings.HasPrefix(dir.text, \"+\") ||", " ||", "TestS2_PushdFoldsAsCd")
m("TB the rules version stays at 7 when hiding flags and pushd change", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 7\n", "TestRulesVersion_Matchers|TestRulesVersion_FlagTables")
# S3 (5.15, 5.13): the program stream reads a comment as the list stream does, and the here-document skip
# stays the list stream's; `make test -C sub` is pinned at the payload's cwd.
m("TB the program stream joins a comment's backslash-newline", "internal/shape/tokenize.go",
  "\treturn bounded(lex(joinLines(s, true), true, false))\n", "\treturn bounded(lex(joinLines(s, false), true, false))\n",
  "TestS3_TK15_|TestS3_TK15b_|TestFoldKeepsRunnerIdentity")
m("TB the program stream reads no comment", "internal/shape/tokenize.go",
  "\t\tcase l.program && c == '#' && !l.w.started:", "\t\tcase l.docs && c == '#' && !l.w.started:",
  "TestS3_TK15_|TestS3_TK15b_|TestFoldKeepsRunnerIdentity")
m("TB the program stream skips a here-document's body", "internal/shape/tokenize.go",
  "\treturn bounded(lex(joinLines(s, true), true, false))\n", "\treturn bounded(lex(joinLines(s, true), true, true))\n",
  "TestS3_TK15c_")
m("TB make test -C sub is located in its directory option", "internal/hook/handle.go",
  "\treturn s, shape.CWDDigest(key, physicalDir(startDirectory(cwd, dirs))), false\n",
  "\treturn s, shape.CWDDigest(key, physicalDir(startDirectory(cwd, append(dirs, \"sub\")))), false\n", "TestS3_13_")
m("TB the rules version stays at 8 when comments end at their newline", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 8\n", "TestRulesVersion_Matchers|TestRulesVersion_Locations")

# 1.2w: WrittenPaths, the write targets of a Bash line (writetargets.go), and
# stageMayWrite, factored out of mayWrite. Version 10 names them, so the
# RulesVersion anchors above moved from 9 to 10, each still setting an older
# version.
m("TB the rules version stays at 9 when the write targets change", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 9\n", "TestRulesVersion_WriteTargets")
# MASKED-OUTCOME1 (D229): version 11 names the runner (RunnerName) and the stage
# of a pipe that hid its status (MaskedBy), pinned over a corpus of its own
# (maskedStagePins). Every anchor above moved from 10 to 11, each still setting
# an older version; version 10 has no masked-stage pin.
m("TB the rules version stays at 10 when the runner and the masking stage are named", "internal/shape/masked.go",
  "const RulesVersion = 11\n", "const RulesVersion = 10\n", "TestRulesVersion_MaskedStage")
m("TB stageMayWrite answers true for a stage no word of which writes", "internal/shape/verb.go",
  "\treturn w.edit || w.redirect || w.substitution || w.writer\n", "\t_ = w\n\treturn true\n",
  "TestWT10_|TestMayWrite")
m("WT a redirection's word is not named", "internal/shape/writetargets.go",
  "\t\t\t\tif writes {\n\t\t\t\t\tw.name(toks[end+1], toks[end+1].text)\n\t\t\t\t}\n", "", "TestWT1_")
m("WT the word after < or >&2 is named", "internal/shape/writetargets.go",
  "\t\t\twrites := outputRedirect(toks, j) || end > j && outputRedirect(toks, j+1)\n",
  "\t\t\twrites := true\n", "TestWT1_")
m("WT sed's script is named", "internal/shape/writetargets.go",
  "\tcase ruleSed:\n\t\to.first = !sedScript(o.w.toks, at, o.e)\n", "\tcase ruleSed:\n\t\to.first = false\n", "TestWT2_")
m("WT sed's -e is never read", "internal/shape/writetargets.go",
  "\t\tif script, _ := sedOption(text); script && len(text) > 1 && text[0] == '-' {\n",
  "\t\tif script, _ := sedOption(text); false && script {\n", "TestWT2_")
m("WT sed's -e takes no next word", "internal/shape/writetargets.go",
  "\t\tcase 'e', 'f':\n\t\t\treturn true, k == len(text)-1\n", "\t\tcase 'e', 'f':\n\t\t\treturn true, false\n", "TestWT2_")
m("WT BSD sed's empty suffix is the script", "internal/shape/writetargets.go",
  "o.w.toks[k].text == \"\" {\n\t\t\t\to.value = true\n", "o.w.toks[k].text == \"\" {\n\t\t\t\to.value = false\n", "TestWT2c_")
m("WT perl's program file is named", "internal/shape/writetargets.go",
  "\tcase rulePerl:\n\t\to.first = true\n", "\tcase rulePerl:\n\t\to.first = false\n", "TestWT2_")
m("WT perl's -e takes no next word", "internal/shape/writetargets.go",
  "\t\tcase c == 'e' || c == 'E':\n\t\t\treturn true, k == len(text)-1\n", "\t\tcase c == 'e' || c == 'E':\n\t\t\treturn true, false\n", "TestWT2_")
m("WT perl reads options after its first operand", "internal/shape/writetargets.go",
  "\t\to.ended = true // perl reads no option after its first operand\n", "", "TestWT2_")
m("WT the arming word is a target", "internal/shape/writetargets.go",
  "\tif o.rule == ruleEvery && onList(text, inPlaceEditors[o.name]) {\n",
  "\tif false && o.rule == ruleEvery && onList(text, inPlaceEditors[o.name]) {\n", "TestWT2_")
m("WT an unarmed editor's stage is classed by its program", "internal/shape/writetargets.go",
  "\tif _, editor := inPlaceEditors[name]; editor {\n", "\tif _, editor := inPlaceEditors[name]; false && editor {\n", "TestWT2b_")
m("WT no program writes by itself", "internal/shape/writetargets.go",
  "\treturn verbForProgram(name) != VerbRead\n}\n", "\treturn false\n}\n", "TestWT6_")
m("WT tee names nothing", "internal/shape/writetargets.go",
  "\tcase \"rm\", \"mkdir\", \"touch\", \"tee\", \"truncate\":\n", "\tcase \"rm\", \"mkdir\", \"touch\", \"truncate\":\n", "TestWT3_")
m("WT rm names nothing", "internal/shape/writetargets.go",
  "\tcase \"rm\", \"mkdir\", \"touch\", \"tee\", \"truncate\":\n", "\tcase \"mkdir\", \"touch\", \"tee\", \"truncate\":\n", "TestWT5_")
m("WT truncate's size is a file", "internal/shape/writetargets.go",
  "\t\to.value = o.name == \"truncate\" && text == \"-s\"\n", "\t\to.value = false\n", "TestWT5_")
m("WT cp's destination is its first operand", "internal/shape/writetargets.go",
  "\t\t\tlast = j\n", "\t\t\tif last < 0 {\n\t\t\t\tlast = j\n\t\t\t}\n", "TestWT4_")
m("WT cp's -t is not read", "internal/shape/writetargets.go",
  "\t\tcase text == \"-t\" || text == \"--target-directory\":\n", "\t\tcase false:\n", "TestWT4_")
m("WT an option is a target", "internal/shape/writetargets.go",
  "\tcase !o.ended && len(text) > 1 && text[0] == '-':\n", "\tcase false && !o.ended && len(text) > 1 && text[0] == '-':\n",
  "TestWT5_")
m("WT -- is a target", "internal/shape/writetargets.go",
  "\tcase !o.ended && text == \"--\":\n\t\to.ended = true\n\t\treturn\n",
  "\tcase !o.ended && text == \"--\":\n\t\to.ended = true\n\t\to.target(t, text)\n\t\treturn\n", "TestWT7_")
m("WT an fd prefix is an operand", "internal/shape/writetargets.go",
  "\t\tcase t.meta, j <= at, at < 0, fdPrefix(toks, j, e):\n", "\t\tcase t.meta, j <= at, at < 0:\n", "TestWT5_")
m("WT a word holding $ is named", "internal/shape/writetargets.go",
  "\t\tstrings.ContainsAny(text, \"$`*?[{\") ||\n", "\t\tstrings.ContainsAny(text, \"`*?[{\") ||\n", "TestWT7_")
m("WT a word holding a quoted backtick is named", "internal/shape/writetargets.go",
  "\t\tstrings.ContainsAny(text, \"$`*?[{\") ||\n", "\t\tstrings.ContainsAny(text, \"$*?[{\") ||\n", "TestWT7_")
m("WT a glob is named", "internal/shape/writetargets.go",
  "\t\tstrings.ContainsAny(text, \"$`*?[{\") ||\n", "\t\tstrings.ContainsAny(text, \"$`?[{\") ||\n", "TestWT7_")
m("WT a ? glob is named", "internal/shape/writetargets.go",
  "\t\tstrings.ContainsAny(text, \"$`*?[{\") ||\n", "\t\tstrings.ContainsAny(text, \"$`*[{\") ||\n", "TestWT7_")
m("WT a bracket glob is named", "internal/shape/writetargets.go",
  "\t\tstrings.ContainsAny(text, \"$`*?[{\") ||\n", "\t\tstrings.ContainsAny(text, \"$`*?{\") ||\n", "TestWT7_")
m("WT a brace expansion is named", "internal/shape/writetargets.go",
  "\t\tstrings.ContainsAny(text, \"$`*?[{\") ||\n", "\t\tstrings.ContainsAny(text, \"$`*?[\") ||\n", "TestWT7_")
m("WT a tilde is named", "internal/shape/writetargets.go",
  " || text == \"-\" || text[0] == '~' ||\n", " || text == \"-\" ||\n", "TestWT7_")
m("WT - is named", "internal/shape/writetargets.go",
  " || text == \"\" || text == \"-\" || text[0] == '~' ||\n", " || text == \"\" || text[0] == '~' ||\n", "TestWT7_")
m("WT the directory-like rule is dropped", "internal/shape/writetargets.go",
  "\t\tstrings.HasSuffix(text, \"/\") || strings.HasSuffix(text, \"...\") || base == \".\" || base == \"..\":\n",
  "\t\tbase == \"\\x00\":\n", "TestWT7_|TestWT4_")
m("WT a device is a file", "internal/shape/writetargets.go",
  "\tcase devFile(text):\n\t\treturn false\n", "\tcase false && devFile(text):\n\t\treturn false\n", "TestWT7_")
m("WT the stages are one at a separator", "internal/shape/writetargets.go",
  "\t\tcase stageSeparator(toks, j):\n\t\t\treturn j, j + 1\n", "\t\tcase false && stageSeparator(toks, j):\n\t\t\treturn j, j + 1\n",
  "TestWT8_")
m("WT a newline ends no stage", "internal/shape/writetargets.go",
  "\t\tcase j > s && t.nlBefore:\n", "\t\tcase false && j > s && t.nlBefore:\n", "TestWT8_")
m("WT complete is forced true", "internal/shape/writetargets.go",
  "\treturn w.targets, w.complete\n}\n", "\treturn w.targets, true\n}\n", "TestWT")
m("WT complete is forced false", "internal/shape/writetargets.go",
  "\treturn w.targets, w.complete\n}\n", "\treturn w.targets, false\n}\n", "TestWT")
m("WT a stage that may write and names nothing is complete", "internal/shape/writetargets.go",
  "\tif runs && !o.named {\n", "\tif false && runs && !o.named {\n", "TestWT6_")
m("WT the may_write guard is dropped", "internal/shape/writetargets.go",
  "\tif w.targets == nil && w.complete && mayWrite(toks, i) {\n", "\tif false && w.targets == nil && w.complete && mayWrite(toks, i) {\n",
  "TestWT12_")
m("WT a relative target past a cd is recorded as written", "internal/shape/writetargets.go",
  "\t\tif dir != \"\" {\n\t\t\ttext = path.Join(dir, text)\n\t\t}\n", "\t\t_ = dir\n", "TestWT13_")
m("WT an unread fold resolves", "internal/shape/writetargets.go",
  "\tif len(dirs) == 0 || len(dirs) > MaxLeadingCd {\n\t\treturn \"\", false\n",
  "\tif len(dirs) == 0 || len(dirs) > MaxLeadingCd {\n\t\treturn \"\", true\n", "TestWT13_")
m("WT an absolute cd step is joined on", "internal/shape/writetargets.go",
  "\t\tcase path.IsAbs(d):\n\t\t\tdir = d\n", "\t\tcase path.IsAbs(d):\n\t\t\tdir = path.Join(dir, d)\n", "TestWT13_")
m("WT a step under ~ is folded", "internal/shape/writetargets.go",
  "\t\tcase d[0] == '~':\n\t\t\treturn \"\", false\n", "\t\tcase false:\n\t\t\treturn \"\", false\n", "TestWT13_")
m("WT a cd later on the line moves nothing", "internal/shape/writetargets.go",
  "\t\tw.moved = true\n", "\t\tw.moved = false\n", "TestWT13_")
m("WT a tool that is not a shell is incomplete", "internal/shape/writetargets.go",
  "\tif verbForTool(toolName) != VerbExecute {\n\t\treturn nil, true\n", "\tif verbForTool(toolName) != VerbExecute {\n\t\treturn nil, false\n",
  "TestWT9_")
m("WT the hook leaves written_paths null", "internal/hook/handle.go",
  "\tdecl.WrittenPaths, decl.WrittenPathsComplete = written, &complete\n", "\t_, _ = written, complete\n", "TestWT11_")

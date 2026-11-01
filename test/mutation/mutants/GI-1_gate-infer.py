# GI-1 (card GATE-INFER1, R46 as W1-W4, W2', W3', W6): with no spec test
# gate, the project's own test command, read CI first by a closed grammar
# through bounded reads, is a required gate the Stop names by its runner,
# labelled "(inferred)" until the index has seen a test run, and the brief
# quotes with its file.
G = "internal/gateinfer/gateinfer.go"
D = "internal/verify/declared.go"
E = "internal/verify/evaluate.go"
S = "internal/verify/stop.go"
O = "internal/verify/obligations.go"
V = "internal/recap/value.go"
B = "internal/brief/brief.go"
C = "cmd/rashomon/gateinfer_stop.go"
R = "cmd/rashomon/recap_stop.go"
L = "cmd/rashomon/laststop.go"

# gateinfer: the order, the grammar and the bounds.
m("GI-1 the CI is not read first", G,
  "\tif in, ok := fromCI(root); ok {\n", "\tif in, ok := fromCI(root); false && ok {\n", "TestGI1_CIFirst")
m("GI-1 a pipe is read as a test command", G,
  "case strings.ContainsRune(\"&|<>()`\", c)", "case strings.ContainsRune(\"&<>()`\", c)", "TestGI1_CILinesReadByAClosedGrammar|TestGI1_CIFirst")
m("GI-1 a cd outside the checkout is followed", G,
  "path.IsAbs(dir) || strings.HasPrefix(dir, \"..\")", "path.IsAbs(dir)", "TestGI1_CILinesReadByAClosedGrammar")
m("GI-1 npm's placeholder test is a test script", G,
  "s != npmPlaceholder", "true", "TestGI1_EachEcosystemItsOwnTestCommand")
m("GI-1 pnpm's lockfile is not read", G,
  "\tcase exists(root, \"pnpm-lock.yaml\"):\n", "\tcase false:\n", "TestGI1_EachEcosystemItsOwnTestCommand")
m("GI-1 a Makefile variable named test is a target", G,
  " && !strings.HasPrefix(rest, \"=\"))", ")", "TestGI1_AMakefileVariableIsNoTarget")
m("GI-1 pytest's own config is a convention", G,
  "{file: \"pytest.ini\", argv: []string{\"pytest\"}, declared: true},", "{file: \"pytest.ini\", argv: []string{\"pytest\"}},",
  "TestGI1_DeclaredOnlyWhereTheProjectSaysSo")
m("GI-1 a line's buffer grows past MaxLine", G,
  "sc.Buffer(make([]byte, 0, 4<<10), MaxLine)", "sc.Buffer(make([]byte, 0, 4<<10), MaxRead)", "TestGI1_BoundedReadsMakeOversizedFilesUnknown")
m("GI-1 a JSON manifest is read whole", G,
  "\tb, err := io.ReadAll(io.LimitReader(f, MaxJSON+1))\n\treturn b, err == nil && len(b) <= MaxJSON\n",
  "\tb, err := io.ReadAll(io.LimitReader(f, MaxRead))\n\treturn b, err == nil\n", "TestGI1_BoundedReadsMakeOversizedFilesUnknown")

# verify: the row, its key, its standing and its name.
m("GI-1 the inferred gate is never judged", E,
  "if g, ok := inferredGate(inf, key, snap.ProjectRoot); ok {", "if g, ok := inferredGate(inf, key, snap.ProjectRoot); false && ok {",
  "TestGI1v_|TestGI1a_")
m("GI-1 the inferred gate is advisory", D,
  "source: SourceInferred, trust: SourceInferred, required: true,", "source: SourceInferred, trust: SourceInferred, required: false,",
  "TestGI1v_")
m("GI-1 the label outlives the index's test run", D,
  "\tif inf.Learned {\n\t\tg.trust = trustBuiltin\n\t}\n", "", "TestGI1v_RequiredInEverySessionLabelledUntilLearned|TestGI1a2_")
m("GI-1 the inferred gate is keyed by its words", D,
  "\tif k, ok := shape.RunnerKey(nameOf(s.RunnerName), s.RunnerDigest); ok {\n",
  "\tif k, ok := \"\", s.RunnerDigest != nil; ok {\n\t\tk = *s.RunnerDigest\n", "TestGI1v_ARunOfItsRunnerBindsIt|TestGI1a_")
m("GI-1 the inferred gate is named by its kind", S,
  "if name := nameOf(lineShape(inf.Argv, key).RunnerName); name != \"\" {",
  "if name := nameOf(lineShape(inf.Argv, key).RunnerName); false && name != \"\" {", "TestGI1v_AnInferredGateNobodyRanIsAbsent|TestGI1a_")
m("GI-1 the inferred gate's rows read unknown", O,
  "\tcase sourceLearned, SourceInferred:\n", "\tcase sourceLearned:\n", "TestGI1o_")

# recap: the line.
m("GI-1 the inferred gate is not labelled", V,
  "\t\tn += inferredLabel\n", "", "TestGI1r_|TestGI1a_")
m("GI-1 no run names no inferred gate", V,
  "\tcase g != nil && g.Source == verify.SourceInferred:\n", "\tcase false:\n", "TestGI1r_|TestGI1a_")
m("GI-1 the label pushes the line past 80 columns", V,
  "&& g.Trust == verify.SourceInferred && !t.short {", "&& g.Trust == verify.SourceInferred {", "TestGI1r_")

# brief and cmd: the wiring.
m("GI-1 the brief never infers", B,
  "\t\tif in, ok := gateinfer.Infer(root); ok {\n", "\t\tif in, ok := gateinfer.Infer(root); false && ok {\n", "TestGI1b_")
m("GI-1 a spec test gate does not win", C,
  "if root == \"\" || sp != nil && slices.ContainsFunc(sp.Gates", "if root == \"\" || false && sp != nil && slices.ContainsFunc(sp.Gates",
  "TestGI1c_")
m("GI-1 the index's test family is not seen", C,
  "\t\tg.Learned = slices.ContainsFunc(idx.Entries", "\t\t_ = slices.ContainsFunc(idx.Entries", "TestGI1c_|TestGI1a2_")
m("GI-1 the replay builds what the checkpoint never had", C,
  "required != nil && !ok {", "false && required != nil && !ok {", "TestGI1c_")
m("GI-1 the Stop is not wired", R,
  "\tin.Inferred = inferredGate(key, sp, idx)\n", "", "TestGI1a_")
m("GI-1 the replay is not wired", L,
  "\t\tin.Inferred = replayInferred(root, r.Spec, idx, in.Required)\n", "", "TestGI1l_")

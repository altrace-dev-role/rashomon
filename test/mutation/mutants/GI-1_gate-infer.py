# GI-1 (card GATE-INFER1, R46 as W1-W6, W2', W3'): with no spec test
# gate, the project's own test command, read CI first by a closed grammar
# through bounded reads, is a required gate the Stop names by its runner,
# labelled "(inferred)" until the index has seen a test run, and the brief
# quotes with its file; a declared or learned one a turn left unrun after
# its edit is the next prompt's note, once per turn end (W5).
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
N = "cmd/rashomon/gateinfer_note.go"
T = "cmd/rashomon/recap_turn.go"
P = "cmd/rashomon/preflight.go"
Q = "internal/recap/state.go"

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

# W5: the next prompt's note, what the Stop owes, recap.json and the wiring.
m("GI-1 a convention alone is owed", N,
  "if inf == nil || !inf.Declared && !inf.Learned || v.Turn.Writes == 0", "if inf == nil || v.Turn.Writes == 0",
  "TestGI1n_TheStopOwes|TestGI1na_")
m("GI-1 a turn that wrote nothing is owed", N,
  "!inf.Learned || v.Turn.Writes == 0 || name", "!inf.Learned || name", "TestGI1n_TheStopOwes")
m("GI-1 a green or red row is owed", N,
  "(g.State == verify.Absent || g.State == verify.Stale) && g.WritesSinceGreen", "true && g.WritesSinceGreen", "TestGI1n_TheStopOwes|TestGI1na_")
m("GI-1 writes the gate ignores are owed", N,
  "verify.Stale) && g.WritesSinceGreen > 0 {", "verify.Stale) {", "TestGI1n_TheStopOwes")
m("GI-1 the Stop drops the stale kind", N,
  "Stale: g.State == verify.Stale,", "Stale: false,", "TestGI1n_TheStopOwes")
m("GI-1 the note drops the stale kind", N,
  "\tif o.Stale {\n\t\tf.Kind = \"stale\"\n\t}\n", "", "TestGI1n_TheNextPrompt")
m("GI-1 the note is not keyed by its turn end", N,
  "\"@\" + strconv.FormatInt(o.Seq, 10)", "\"@\" + strconv.FormatInt(0, 10)", "TestGI1n_TheNextPrompt")
m("GI-1 the note is given every prompt", N,
  "\tif owedGiven(lines, key) {\n", "\tif false && owedGiven(lines, key) {\n", "TestGI1n_TheNextPrompt|TestGI1na_")
m("GI-1 a superseded line counts as given", N,
  "(scanText(b, sp[1]) == string(steer.LineStateDelivered) ||", "(scanText(b, sp[1]) != \"\" ||", "TestGI1n_TheNextPrompt")
m("GI-1 an arm-off line is not given", N,
  " || scanText(b, sp[3]) == string(steer.SuppressedByArmOff))", ")", "TestGI1n_TheNextPrompt")
m("GI-1 a refused file drops the note", N,
  "\t\tm, err = render.New(pattern.ClaimVsRecord, render.ClaimVsRecordInferredUnnamedBinding{Gate: slot.Gate(o.Gate)})\n", "",
  "TestGI1n_TheNextPrompt")
m("GI-1 a seq past the decision's is noted", N,
  "if o.Seq <= 0 || o.Seq > afterSeq {", "if o.Seq <= 0 {", "TestGI1n_TheNextPrompt")
m("GI-1 the Stop owes nothing", R,
  ", owed: owedNote(v, in.Inferred)}", "}", "TestGI1na_")
m("GI-1 the prompt reads nothing owed", T,
  "\tpp.owed = owed\n", "\t_ = owed\n", "TestGI1na_")
m("GI-1 the prompt stops before the owed note", P,
  " && p.owed.Seq <= 0 {", " {", "TestGI1na_")
m("GI-1 the owed note is not routed", P,
  "\tif f, ok := owedFinding(p.owed, run.LastSeq(), lines); ok {\n", "\tif f, ok := owedFinding(p.owed, run.LastSeq(), lines); false && ok {\n",
  "TestGI1na_")
m("GI-1 recap.json drops the owed note", Q,
  "\t\ts.Owed[sessionID] = owed\n", "", "TestGI1s_|TestGI1na_")
m("GI-1 a turn end that owes nothing keeps the last", Q,
  "\tdefault:\n\t\tdelete(s.Owed, sessionID)\n", "\tdefault:\n", "TestGI1s_")
m("GI-1 the owed map is left out of the cap", Q,
  "\tcapSessions(s.Owed, current)\n", "", "TestGI1s_")
m("GI-1 Turn reads every prompt as pending", Q,
  "return promptID != \"\" && s.Checked[sessionID] != promptID, s.Owed[sessionID]", "return promptID != \"\", s.Owed[sessionID]", "TestGI1s_")
m("GI-1 the Stop's command is not declared", C,
  "From: in.From, Declared: in.Declared}", "From: in.From}", "TestGI1na_")
m("GI-1 the Stop's file is not carried", C,
  "From: in.From, Declared", "Declared", "TestGI1na_")

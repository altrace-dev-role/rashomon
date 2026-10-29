# VER-6.2: the lesson ledger (internal/learn, `rashomon learn`). One break per
# promise: a tier moves only by the Wilson bound over scored labels, a
# proposed lesson only by a person typing the code at a terminal, a shared
# lesson enters at proposed, active.json holds approved rows under its cap,
# dropping the oldest demoted first, and a write the learner did not make
# quarantines every lesson. The kill row is the named test's.
L = "internal/learn/learn.go"
D = "internal/learn/ledger.go"

# Tiers move by the bound alone.
m("VER-6.2 proven without the lower bound", L,
  "\tcase tally.Confirmed+tally.Refuted >= MinProven && lo >= Threshold:\n",
  "\tcase tally.Confirmed+tally.Refuted >= MinProven && lo >= 0:\n", "TestP6_TierMovesOnlyByTheBound")
m("VER-6.2 proven under 30 scored labels", L,
  "\tcase tally.Confirmed+tally.Refuted >= MinProven && lo >= Threshold:\n",
  "\tcase lo >= Threshold:\n", "TestP6_TierMovesOnlyByTheBound")
m("VER-6.2 never demoted", L, "\tcase hi < Threshold:\n", "\tcase false && hi < Threshold:\n",
  "TestP6_TierMovesOnlyByTheBound")
m("VER-6.2 a proposed lesson moves by its labels", L,
  "\tif t == TierProposed {\n\t\treturn TierProposed\n", "\tif false {\n\t\treturn TierProposed\n",
  "TestP6_TierMovesOnlyByTheBound")
m("VER-6.2 unresolved labels enter n", L,
  "\treturn Wilson95(t.Confirmed, t.Confirmed+t.Refuted)\n",
  "\treturn Wilson95(t.Confirmed, t.Confirmed+t.Refuted+t.Unresolved)\n", "TestP6_TierMovesOnlyByTheBound")
m("VER-6.2 a label outside the three is a fire", L,
  "\tdefault:\n\t\treturn\n\t}\n\tt.Fires++\n", "\tdefault:\n\t}\n\tt.Fires++\n", "TestP6_LabelRowIsLabelWriter1sContract")
m("VER-6.2 the candidate gate takes one day", L, "e.Days >= MinDays", "e.Days >= 1", "TestP6_CandidateGate")
m("VER-6.2 the candidate gate takes a contradiction", L,
  "\t\te.Consistency >= Threshold && e.Contradictions == 0\n", "\t\te.Consistency >= Threshold\n", "TestP6_CandidateGate")

# Entry and approval.
m("VER-6.2 an import keeps the tier it names", D,
  "Lesson{ID: ls.ID, Tier: TierProposed, Text: ls.Text,", "Lesson{ID: ls.ID, Tier: cmp.Or(ls.Tier, TierProposed), Text: ls.Text,",
  "TestP6_ImportLandsAtProposed|TestP6_LearnImportThenApprove")
m("VER-6.2 an import takes a text over the steer cap", D,
  " || len(ls.Text) > store.MaxSteerTextBytes || ", " || ", "TestP6_ImportLandsAtProposed")
m("VER-6.2 approval moves a lesson already approved", D,
  "\tif l.file.Lessons[i].Tier != TierProposed {\n", "\tif false {\n", "TestP6_ApprovalMovesOnlyAProposedLesson")

# active.json and quarantine.
m("VER-6.2 a proposed lesson reaches active.json", D,
  "\t\tif ls.Tier != TierProposed {\n\t\t\trows = append(rows, ls)\n", "\t\tif true {\n\t\t\trows = append(rows, ls)\n",
  "TestP6_ImportLandsAtProposed")
m("VER-6.2 the cap is not enforced", D,
  "\t\tif err != nil || len(b)+1 <= ActiveMaxBytes {\n", "\t\tif err != nil || true {\n",
  "TestP6_ActiveCapDropsTheOldestDemotedFirst")
m("VER-6.2 the cap drops an approved row before a demoted one", D,
  "\t\ti := slices.IndexFunc(rows, func(ls Lesson) bool { return ls.Tier == TierDemoted })\n",
  "\t\ti := slices.IndexFunc(rows, func(ls Lesson) bool { return ls.Tier != TierProposed })\n",
  "TestP6_ActiveCapDropsTheOldestDemotedFirst")
m("VER-6.2 quarantine not raised", D,
  "\tl.Quarantined = len(key) == 0 || !hmac.Equal(sum, []byte(sign(key, files)))\n",
  "\tl.Quarantined = false && (len(key) == 0 || !hmac.Equal(sum, []byte(sign(key, files))))\n",
  "TestP6_AgentWriteQuarantinesEveryFact")
m("VER-6.2 quarantine leaves lessons approved", D,
  "\tfor i := range l.file.Lessons {\n\t\tl.file.Lessons[i].Tier = TierProposed\n\t}\n",
  "\tfor i := range l.file.Lessons {\n\t\t_ = i\n\t}\n", "TestP6_AgentWriteQuarantinesEveryFact")
m("VER-6.2 the manifest leaves labels unsigned", D,
  "var stateFiles = []string{FileLessons, FileLabels, FileActive}\n",
  "var stateFiles = []string{FileLessons, FileActive}\n", "TestP6_AgentWriteQuarantinesEveryFact")
m("VER-6.2 the key is not private", D, "\tif err := store.ReplaceFile(path, key); err != nil {\n",
  "\tif err := os.WriteFile(path, key, 0o644); err != nil {\n", "TestP6_KeyIsPrivate|TestTRU23_LearnPaths")

# purge.
m("VER-6.2 purge leaves a project's manifest", D,
  "\t\tfiles = append(slices.Clone(stateFiles), FileSum)\n", "\t\tfiles = slices.Clone(stateFiles)\n",
  "TestP6_PurgeLeavesNoLearnerFile")

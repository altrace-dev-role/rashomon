# RP-1 (card REPORT1a, claims no requirement row): the session impact. One
# derivation feeds the Stop line's clause and the report's first screen and
# autosteer block, and each mutant breaks one promise it makes: a finding is
# resolved only by a later green; every count carries its seqs; the clause
# prints only when the counts changed; the summary switch is read; and
# `--session last` is the working directory's project's.
m("RP-1 a finding is resolved without a later green", "internal/recap/impact.go",
  "if n != 1 || match.LastGreen == nil || match.LastGreen.Seq <= f.Seq || match.State != verify.Green {",
  "if n != 1 || match.LastGreen == nil || match.State != verify.Green {",
  "TestRP1_ResolvedOnlyByALaterGreen")
m("RP-1 a resolved finding loses the seq of its green", "internal/recap/impact.go",
  "return FindingResolved, match.LastGreen.Seq",
  "return FindingResolved, 0",
  "TestRP1_ResolvedOnlyByALaterGreen")
m("RP-1 the report's finding is counted without its seqs", "internal/report/text_autosteer.go",
  "return s + \", resolved by #\" + strconv.FormatInt(f.ResolvedBy, 10) + \" green\"",
  "return s + \", resolved\"",
  "TestRP1d_ReportBlockMatchesTheClause")
m("RP-1 the clause's count carries no seq", "internal/recap/clause.go",
  "\t\tat = append(at, \"#\"+strconv.FormatInt(f.ResolvedBy, 10))",
  "\t\tat = append(at, \"#0\")",
  "TestRP1_|TestRP1_ClauseCountsDeliveredFindingsResolvedChecks")
m("RP-1 the clause repeats when the counts did not change", "cmd/rashomon/recap_stop.go",
  "if fp := im.Fingerprint(); fp == s.shown || im.Empty() {",
  "if fp := im.Fingerprint(); im.Empty() {",
  "TestRP1b_ClausePrintsOnlyOnChange")
m("RP-1 the summary switch is ignored", "cmd/rashomon/recap_stop.go",
  "\tif !s.summary {",
  "\tif false {",
  "TestRP1c_SummaryOffSilencesTheClauseNotTheDoneFlag")
m("RP-1 session last reads another directory's project", "cmd/rashomon/report_impact.go",
  "receipt.Latest(st, baseline.Key(cwd))",
  "receipt.Latest(st, baseline.Key(os.TempDir()))",
  "TestRP1h_SessionLast")
m("RP-1 an unknown check is counted met", "internal/recap/impact.go",
  "\t\tcase verify.ObligationMet:\n\t\t\tmet++",
  "\t\tcase verify.ObligationMet, verify.ObligationUnknown:\n\t\t\tmet++",
  "TestRP1g_UnknownIsNotMet")
# The review's blocker: a finding is settled by its own gate, never by a
# program or a kind another gate shares.
m("RP-1 a finding is resolved by any gate of its program", "internal/recap/impact.go",
  "\t\tif key != \"\" && v.Snap.Gates[i].Gate == key {",
  "\t\tif false {",
  "TestRP1i_AGreenBuildDoesNotResolveATestFinding|TestRP1i_TheSameGateGoingGreenResolves")
m("RP-1 a slot two gates share resolves its finding", "internal/recap/impact.go",
  "if n != 1 || match.LastGreen == nil",
  "if n == 0 || match.LastGreen == nil",
  "TestRP1i_ASlotTwoGatesShareReadsOpen|TestRP1i_AGreenBuildDoesNotResolveATestFinding")
m("RP-1 the line's gate id is not read, so the true resolution is lost", "internal/recap/impact.go",
  "\tcase string(pattern.RegressionLocation), string(pattern.StaleEvidence), string(pattern.SubtaskGate):\n\t\treturn key",
  "\tcase string(pattern.RegressionLocation), string(pattern.StaleEvidence), string(pattern.SubtaskGate):\n\t\treturn \"\"",
  "TestRP1i_TheSameGateGoingGreenResolves")

# The Stop view names a gate by its runner's whole name ("go test"), the slot
# by its program word ("go"): the fallback compares first words, and a word two
# gates share settles nothing.
m("RP-1 the runner name is not split into words, so go test never matches go", "internal/recap/impact.go",
  "slices.Contains(strings.Fields(v.Programs[g.Gate]), f.Gate)",
  "v.Programs[g.Gate] == f.Gate",
  "TestRP1i_ARunnerNameIsComparedByItsProgramWord")
m("RP-1 a first word two gates share resolves its finding", "internal/recap/impact.go",
  "\tif n != 1 || match.LastGreen == nil || match.LastGreen.Seq <= f.Seq",
  "\tif n == 0 || match.LastGreen == nil || match.LastGreen.Seq <= f.Seq",
  "TestRP1i_ARunnerNameIsComparedByItsProgramWord")

# A trusted spec's gate id is not a host-safe word (RP1zl; census g3 W1, STEP-15).
D = "internal/report/redact_autosteer.go"
DR = "internal/report/redact.go"
m("RP-1 Redact skips the autosteer pass", DR, "\t\ts.Autosteer = redactAutosteer(sess.Autosteer, key)\n", "\t\ts.Autosteer = sess.Autosteer\n", "TestRP2zl_")
m("RP-1 Redact keeps a gate's name", D, "\t\tg.Name = redactWord(g.Name, key)\n", "", "TestRP2zl_")
m("RP-1 Redact keeps the Stop's sentence", D, "\t\tst.Sentence = redactWord(st.Sentence, key)\n", "", "TestRP2zl_")
m("RP-1 Redact keeps a finding's gate", D, "\t\tf.Gate = redactWord(f.Gate, key)\n", "", "TestRP2zm_")
m("RP-1 Redact keeps a check's gate key", D, "\t\t\tc.Gate = redactGateKey(c.Gate, key)\n", "", "TestRP2zm_")
m("RP-1 Redact keeps a check's reason", D, "\t\t\tc.Reason = strings.ReplaceAll(c.Reason, c.Gate, redactGateKey(c.Gate, key))\n", "", "TestRP2zm_")
m("RP-1 a spec gate key loses its prefix", D, '\t\treturn "spec:" + redactWord(id, key)\n', "\t\treturn redactWord(id, key)\n", "TestRP2zm_")
m("RP-1 a learned gate key is digested", D, '\tif strings.HasPrefix(k, "learned:") || strings.HasPrefix(k, "ci:") {\n', "\tif false {\n", "TestRP2zm_")

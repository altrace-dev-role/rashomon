# RP-1 (card REPORT1a, claims no requirement row): the session impact. One
# derivation feeds the Stop line's clause and the report's first screen and
# autosteer block, and each mutant breaks one promise it makes: a finding is
# resolved only by a later green; every count carries its seqs; the clause
# prints only when the counts changed; the summary switch is read; and
# `--session last` is the working directory's project's.
m("RP-1 a finding is resolved without a later green", "internal/recap/impact.go",
  "if g.LastGreen != nil && g.LastGreen.Seq > f.Seq && g.State == verify.Green {",
  "if g.LastGreen != nil && g.State == verify.Green {",
  "TestRP1_ResolvedOnlyByALaterGreen")
m("RP-1 a resolved finding loses the seq of its green", "internal/recap/impact.go",
  "state, by = FindingResolved, g.LastGreen.Seq",
  "state, by = FindingResolved, 0",
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

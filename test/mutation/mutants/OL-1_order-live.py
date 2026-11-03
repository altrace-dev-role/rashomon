# OL-1: an order rule's Stop row reads live (card ORDER-LIVE1): every Bash
# call the post reads under the rule leaves a then_check (yes, no or maybe),
# kept whether or not the call raised anything; the Stop reads the Then of a
# line the record holds no command for from it, and the same line's After
# from a hit at that call; a call with no check reads maybe, never no. Each
# break removes one rule and names the tests that hold it.
VR = "internal/verify/rules_raise.go"
VO = "internal/verify/rules_order.go"
HK = "internal/hook/steerstep.go"
m("OL-1 the post keeps no no", VR,
  "\t\t\tif read {\n\t\t\t\tview.ThenChecks = append(",
  "\t\t\tif read && lineMatch(dr.Rule.Then, &ordLine) != triNo {\n\t\t\t\tview.ThenChecks = append(", "TestOL1a_|TestOL1b_")
m("OL-1 the post writes maybe for a line that is no Then", VO,
  "\tcase triNo:\n\t\treturn store.ThenNo\n", "", "TestOL1b_")
m("OL-1 a call with no then_check reads no", VO,
  "\t\tif t, ok := checks[l.seq]; ok && !l.whole {\n",
  "\t\tif t, ok := checks[l.seq]; ok || !l.whole {\n", "TestOL1c_")
m("OL-1 a line not read keeps its unread after", VO,
  "\t\t\tthen, after = t, triOf(false, hasHit(hits, l.seq))\n", "\t\t\tthen = t\n", "TestOL1a_")
m("OL-1 a check of an earlier spec digest counts", VO,
  "\t\tif c.RuleID != id || c.SourceDigest != digest || c.LandedSeq <= 0 || c.LandedSeq > at {\n",
  "\t\tif c.RuleID != id || c.LandedSeq <= 0 || c.LandedSeq > at {\n", "TestOL1e_")
m("OL-1 a check past the point counts", VO,
  "\t\tif c.RuleID != id || c.SourceDigest != digest || c.LandedSeq <= 0 || c.LandedSeq > at {\n",
  "\t\tif c.RuleID != id || c.SourceDigest != digest || c.LandedSeq <= 0 {\n", "TestOL1e_")
m("OL-1 two checks that disagree keep the last", VO,
  "\t\tif old, ok := out[seq]; ok && old != t {\n",
  "\t\tif old, ok := out[seq]; ok && old != t && false {\n", "TestOL1e_")
m("OL-1 an unknown word reads no", VO,
  "\t\tt := triMaybe\n\t\tswitch c.Then {\n", "\t\tt := triNo\n\t\tswitch c.Then {\n", "TestOL1e_")
m("OL-1 a check keys its landed seq", VO,
  "\t\t\tseq = d\n", "\t\t\t_ = d\n", "TestOL1e_|TestOL1a_")
m("OL-1 a call that raised nothing keeps no check", HK,
  "\t\tif len(view.ThenChecks) > 0 {\n", "\t\tif false {\n", "TestOL1b_")
m("OL-1 the early return drops the checks", HK,
  "verify.SteerView{ThenChecks: rules.ThenChecks}, false", "verify.SteerView{}, false", "TestOL1b_")
m("OL-1 a stepped post drops the checks", HK,
  "\tview.Hits, view.AfterHits, view.ThenChecks = rules.Hits, rules.AfterHits, rules.ThenChecks\n",
  "\tview.Hits, view.AfterHits = rules.Hits, rules.AfterHits\n", "TestOL1a_|TestOL1f_")
m("OL-1 the hook writes no then_check", HK,
  "\tfor _, c := range checks {\n", "\tfor _, c := range checks[:0] {\n", "TestOL1a_|TestOL1b_")
m("OL-1 the then_check kind is not in the version table", "internal/store/kinds.go",
  "\tTypeThenCheck:     EventSchemaVersion,\n", "", "TestKindVersionsAreTheFrozenTable|TestOL1w_|TestOL1b_")
m("OL-1 the reader does not file a then_check", "internal/store/read.go",
  "\tcase TypeThenCheck:\n\t\treturn readThenCheck(run, line, kind, version)\n", "", "TestOL1w_|TestOL1a_")
m("OL-1 the then_check is stamped not critical", "internal/store/thencheck.go",
  "SchemaVersion: EventSchemaVersion, Critical: true,", "SchemaVersion: EventSchemaVersion, Critical: false,",
  "TestOL1w_|TestOL1b_")
m("OL-1 the records row omits the then_check it holds", "internal/disclose/disclose.go",
  "; where a Bash call's line was read for an order rule of that spec, a then_check with the rule's id, the content digest of the spec file that declared it, the seq the call's execution landed at and one word for whether the line runs the rule's then (yes, no or maybe), no command text;",
  ";", "TestOL1d_")

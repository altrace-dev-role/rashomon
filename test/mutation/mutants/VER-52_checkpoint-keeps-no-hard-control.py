# VER-52: the checkpoint stores a rule verdict as rule, kind, verdict and
# seqs (archive 20.2.14). The spec's hard_control text stays on the
# snapshot's RuleVerdict, which only prints; the checkpoint storing the
# snapshot's entry, or its own entry gaining the field, is the regression.
m("VER-52 the checkpoint stores the snapshot's rule verdict again", "internal/verify/snapshot.go",
  "\tSpecVerdicts        []CheckpointRuleVerdict `json:\"spec_verdicts\"`\n",
  "\tSpecVerdicts        []RuleVerdict `json:\"spec_verdicts\"`\n", "TestVER52e_")
m("VER-52 the checkpoint's rule verdict carries hard_control", "internal/verify/snapshot.go",
  "\tSeqs    []int64          `json:\"seqs\"`\n}\n\n// IntegrityKind",
  "\tSeqs    []int64          `json:\"seqs\"`\n\tHardControl string `json:\"hard_control\"`\n}\n\n// IntegrityKind", "TestVER52e_")

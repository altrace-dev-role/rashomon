# VER-52: the checkpoint stores a rule verdict as rule, kind, verdict and
# seqs (archive 20.2.14). The spec's hard_control text stays on the
# snapshot's RuleVerdict, which only prints; the checkpoint storing the
# snapshot's entry, or its own entry gaining the field, is the regression.
# The break aliases the checkpoint's verdict to the snapshot's, so every site
# that builds or cuts the checkpoint's list still compiles (a mutant is one
# file) and the record stores the snapshot's entry, hard_control and all.
m("VER-52 the checkpoint stores the snapshot's rule verdict again", "internal/verify/snapshot.go",
  "type CheckpointRuleVerdict struct {\n\tRule    SlotID           `json:\"rule\"`\n\tKind    spec.RuleKind    `json:\"kind\"`\n\tVerdict RuleVerdictValue `json:\"verdict\"`\n\tSeqs    []int64          `json:\"seqs\"`\n}\n",
  "type CheckpointRuleVerdict = RuleVerdict\n", "TestVER52e_")
m("VER-52 the checkpoint's rule verdict carries hard_control", "internal/verify/snapshot.go",
  "\tSeqs    []int64          `json:\"seqs\"`\n}\n\n// IntegrityKind",
  "\tSeqs    []int64          `json:\"seqs\"`\n\tHardControl string `json:\"hard_control\"`\n}\n\n// IntegrityKind", "TestVER52e_")

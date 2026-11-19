# CONFLICTFOLD-1: the Stop's fold of the run's conflict checks is bounded on
# each check's landed seq, the seq its call's execution landed at, and skips
# a check landed at 0 or less at every bound, bound 0 included. A post
# appends its check one record seq after a lone call's execution, so only
# calls in parallel tell a bound on the record seq less one from the landed
# bound. Each row is one change to the final condition.
F = "cmd/rashomon/recap_conflicts.go"
m("CONFLICTFOLD-1 the fold is bounded on the check's record seq", F,
  "(seq > 0 && rec.LandedSeq > seq)",
  "(seq > 0 && rec.Seq != nil && *rec.Seq > seq)",
  "TestConflictsSeen_FoldFromRecord")
m("CONFLICTFOLD-1 the fold is bounded on the record seq less one", F,
  "(seq > 0 && rec.LandedSeq > seq)",
  "(seq > 0 && rec.Seq != nil && *rec.Seq > seq+1)",
  "TestConflictsSeen_InterleavedCallsFoldAtTheirLandedSeqs")
m("CONFLICTFOLD-1 a check landed at 0 or less is folded", F,
  "rec.LandedSeq <= 0 || (",
  "(",
  "TestConflictsSeen_ACheckLandedAtZeroIsSkipped")

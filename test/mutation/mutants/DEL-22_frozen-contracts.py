# DEL-22: P-07's contracts are frozen by test. The steer.PatternID set and the
# gate Reason vocabulary are the two lists the row names; a dropped id, a
# dropped reason or two reasons sharing a string is the whole regression.
m("DEL-22 the frozen pattern set loses masked-status", "internal/steer/pattern/pattern.go",
  "\t\tClaimVsRecord, MaskedStatus, SequenceRisk,\n",
  "\t\tClaimVsRecord, SequenceRisk,\n", "TestDEL22_")
m("DEL-22 a gate reason is dropped from the vocabulary", "internal/verify/state.go",
  "\tBudget, OK,\n}", "\tOK,\n}", "TestDEL22_")
m("DEL-22 two gate reasons share a string", "internal/verify/state.go",
  'MaskedNoSuccessLine   Reason = "masked_no_success_line"',
  'MaskedNoSuccessLine   Reason = "masked_with_success_line"', "TestDEL22_")
m("DEL-22 the session record's resume key is respelled", "internal/store/record_v5.go",
  "`json:\"resume\"`", "`json:\"resumed\"`", "TestDEL22b_")
m("DEL-22 the target's dropped keys become a count again", "internal/store/record_v5.go",
  "\tDropped           []string          `json:\"dropped\"`\n", "\tDropped           int               `json:\"dropped\"`\n", "TestDEL22b_")

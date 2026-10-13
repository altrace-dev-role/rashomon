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
# The pattern interface proposal, the nullable steer-line prompt_id and
# Finding.Check (cross F7, F8(d); P07N-2, P07N-14).
m("DEL-22 the pattern interface's Revalidate is renamed", "internal/steer/pattern_iface.go",
  "\tRevalidate(ctx *Context, f Finding) bool\n", "\tRecheck(ctx *Context, f Finding) bool\n", "TestDEL22c_")
m("DEL-22 a steer line drops prompt_id when it has none", "internal/steer/line.go",
  "\tPromptID *string `json:\"prompt_id\"`\n", "\tPromptID *string `json:\"prompt_id,omitempty\"`\n", "TestDEL22d_")
m("DEL-22 any kind passes as a frozen kind", "internal/steer/pattern/pattern.go",
  "\treturn slices.Contains(kinds[id], kind)\n}\n\n// classForm", "\treturn true\n}\n\n// classForm", "TestDEL22e_")
m("DEL-22 the masked claim may be model-visible", "internal/steer/finding.go",
  '\treturn f.Pattern == pattern.ClaimVsRecord && f.Kind == "masked"\n', "\treturn false\n", "TestDEL22e_")
m("DEL-22 a message rides another pattern's finding", "internal/steer/finding.go",
  "\tcase f.Message.Text() != \"\" && f.Message.Pattern() != f.Pattern:\n", "\tcase false:\n", "TestDEL22e_")
m("DEL-22 evidence may repeat or follow after_seq", "internal/steer/finding.go",
  "\t\tif seq > f.AfterSeq || i > 0 && seq <= f.Evidence[i-1] {\n", "\t\tif i > 0 && seq < f.Evidence[i-1] {\n", "TestDEL22e_")
m("DEL-22 a stop form rides below the stop level", "internal/steer/finding.go",
  "\tcase f.StopForm != StopFormNone && f.MaxLevel != LevelStop:\n", "\tcase false:\n", "TestDEL22e_")

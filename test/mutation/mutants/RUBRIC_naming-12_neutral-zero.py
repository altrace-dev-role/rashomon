# Rubric naming-12: an enum's zero value is a neutral member, never a
# meaningful one, so a value nobody set cannot read as a decision.
m("RUBRIC naming-12 the zero LineKind is a blank line again", "internal/report/transcript.go",
  "\t_ LineKind = iota // the zero value: no kind\n\t// LineBlank is an empty or whitespace-only line: no line at all.\n\tLineBlank\n",
  "\t// LineBlank is an empty or whitespace-only line: no line at all.\n\tLineBlank LineKind = iota\n",
  "TestRubricNAMING12_TheZeroLineKindIsNoKind")
m("RUBRIC naming-12 the zero Phase is Verify again", "internal/budget/budget.go",
  "\tphaseUnset Phase = iota\n\tVerify\n", "\tVerify Phase = iota\n\tphaseUnset\n",
  "TestRubricNAMING12_TheZeroPhaseIsNoPhase")
m("RUBRIC naming-12 the zero CaptureMode copies the head again", "internal/jsonscan/stream.go",
  "\t_ CaptureMode = iota // the zero value: no mode, refused\n\t// CaptureHead keeps the first Max bytes.\n\tCaptureHead\n",
  "\t// CaptureHead keeps the first Max bytes.\n\tCaptureHead CaptureMode = iota\n",
  "TestRubricNAMING12_AnUnsetCaptureModeIsRefused")

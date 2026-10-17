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
m("RUBRIC naming-12 an unset or unknown CaptureMode gets ErrPaths' own message",
  "internal/jsonscan/capture.go",
  "\tdefault:\n\t\treturn fmt.Errorf(\"%w: capture mode %d is not set or not a mode Stream knows\", ErrPaths, p.Mode)\n",
  "\tdefault:\n\t\treturn ErrPaths\n",
  "TestRubricNAMING12_AnUnsetCaptureModeIsRefused")
m("RUBRIC naming-12 a CaptureLines Max too small gets ErrPaths' own message",
  "internal/jsonscan/capture.go",
  "\t\tif p.Max < utf8.UTFMax {\n\t\t\treturn fmt.Errorf(\"%w: a CaptureLines path's Max (%d) must be at least %d, to hold one rune\", ErrPaths, p.Max, utf8.UTFMax)\n\t\t}\n",
  "\t\tif p.Max < utf8.UTFMax {\n\t\t\treturn ErrPaths\n\t\t}\n",
  "TestRubricNAMING12_ACaptureLinesBelowOneRuneIsRefused")

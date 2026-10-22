# STR-9: a continuity briefing is mirrored byte for byte like every other
# note, its lines inside the one quote. MirrorBriefing checking a briefing
# with the single-line Validate, which refuses every briefing, or wrapping
# it unchecked, is the regression.
m("STR-9 a briefing mirror runs the single-line check", "internal/steer/render/validate.go",
  "\tif err := ValidateBriefing(modelText); err != nil {\n\t\treturn \"\", err\n\t}\n\treturn MirrorPrefix + modelText + MirrorSuffix, nil\n",
  "\tif err := Validate(modelText); err != nil {\n\t\treturn \"\", err\n\t}\n\treturn MirrorPrefix + modelText + MirrorSuffix, nil\n", "TestSTR9c_")
m("STR-9 a briefing mirror wraps unchecked text", "internal/steer/render/validate.go",
  "\tif err := ValidateBriefing(modelText); err != nil {\n\t\treturn \"\", err\n",
  "\tif err := ValidateBriefing(modelText); err != nil && false {\n\t\treturn \"\", err\n", "TestSTR9c_")

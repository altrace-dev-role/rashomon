# BRD-4: Claude entry points stand down on foreign or unknown harnesses --
# StandsDown's own comparison, and familyOf's refusal to call Unknown a
# member of any installed-for family.

m("BRD-4 StandsDown never fires: every entry matches every harness",
  "internal/hostguard/hostguard.go",
  "\treturn string(installedFor) != familyOf(id.ID)",
  "\treturn false",
  "TestBRD4_")

m("BRD-4 Unknown is folded into the claude family, so a Claude entry never stands down under an unidentified caller",
  "internal/hostguard/hostguard.go",
  '\tdefault:\n\t\treturn "unknown"\n\t}',
  '\tdefault:\n\t\treturn string(FamilyClaude)\n\t}',
  "TestBRD4_")

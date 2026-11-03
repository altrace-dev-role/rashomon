# PERF-EDGE1: a post reads one capability with harness.Cap, the rule Caps'
# table is built from, and builds no table for it. The rules' own rows are
# BRD-1's and PF-1's (re-anchored onto Cap); these are Cap's.
C = "internal/harness/caps.go"

m("PE-1 a capability not in the table reads as one", C,
  "\tif !slices.Contains(allCapabilities, c) {\n",
  "\tif !slices.Contains(allCapabilities, c) && false {\n", "TestPE1_CapIsCapsWithoutTheTable")
m("PE-1 the unknown surface's mirror is verified", C,
  "(s == SurfaceSDK || s == SurfaceUnknown)", "(s == SurfaceSDK)", "TestBRD1b_|TestPF1z_|TestPE1_")
m("PE-1 a person reads an unknown surface's prompt", C,
  "\tcase c == CapPersonAtPrompt && s == SurfaceUnknown:\n\t\treturn Unverified\n",
  "\tcase c == CapPersonAtPrompt && s == SurfaceUnknown:\n\t\treturn Yes\n", "TestBRD1b_|TestPF1z_|TestPE1_")

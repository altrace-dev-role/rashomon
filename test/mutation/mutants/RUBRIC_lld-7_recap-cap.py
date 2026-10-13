# lld-7: recap.json's session cap never drops the session Claim just wrote.
m("lld-7 the cap drops the session just claimed", "internal/recap/state.go",
  "\t\tif id != current {\n", "\t\tif true {\n", "TestRubricLLD7_")
m("lld-7 Claim does not name its session to the cap", "internal/recap/state.go",
  "\treturn speak, writeState(path, s, sessionID)\n", "\treturn speak, writeState(path, s, \"\")\n", "TestRubricLLD7_")

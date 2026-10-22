# H-144: without --harness, every hook path is Claude Code's, byte for byte.
# Each break lets the Codex side of the harness branch reach a command line
# that names no harness.
m("H-144 a command line naming no harness is read as Codex", "internal/hook/harness.go",
  "\t\treturn \"\", false\n\t}\n\treturn ClaudeCode, true\n",
  "\t\treturn \"\", false\n\t}\n\treturn Codex, true\n", "TestH144_")
m("H-144 recap is silent under Claude Code too", "cmd/rashomon/harness.go",
  "\treturn h, true\n}", "\treturn h, h == hook.Codex\n}", "TestH144_")
m("H-144 coverage skips Claude Code's configuration for every harness", "internal/hook/coverage.go",
  "\tif harness != ClaudeCode {\n\t\treturn resolveUnread(st, sessionID)\n\t}\n",
  "\tif true {\n\t\treturn resolveUnread(st, sessionID)\n\t}\n", "TestH144_")
m("H-144 every execution is recorded unobserved", "internal/hook/post.go",
  "\tif p.Harness == Codex {\n\t\tunobserved(&rec)\n\t}\n", "\tunobserved(&rec)\n", "TestH144_")
m("H-144 every declaration takes its prompt_id from turn_id", "internal/hook/handle.go",
  "\tif h.Harness == Codex {\n\t\tdecl.PromptID = codexTurn(", "\tif true {\n\t\tdecl.PromptID = codexTurn(", "TestH144_")

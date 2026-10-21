# H-145: Codex's captured payloads record what Codex said and nothing it did
# not. Most breaks are judged by the replay; the few it cannot see -- a list
# entry no fixture exercises, a lookup the fixtures' order happens to
# satisfy -- are judged by the unit test that pins them.
m("H-145 a Codex execution is recorded as Claude Code reads it", "internal/hook/post.go",
  "\tif p.Harness == Codex {\n\t\tunobserved(&rec)\n\t}\n", "", "TestH145_|TestPost_CodexOutcomeIsUnobserved")
m("H-145 a Codex execution keeps the outcome its event implies", "internal/hook/harness.go",
  "\trec.Outcome = \"\"\n", "", "TestH145_|TestPost_CodexOutcomeIsUnobserved")
m("H-145 --harness codex is ignored", "internal/hook/harness.go",
  "\t\tif i+1 < len(args) && Harness(args[i+1]) == Codex {\n\t\t\treturn Codex, true\n\t\t}\n\t\treturn \"\", false\n",
  "\t\t_ = i\n\t\treturn ClaudeCode, true\n", "TestH145_|TestHarnessArg")
m("H-145 an unknown harness is read as Claude Code", "internal/hook/harness.go",
  "\t\treturn \"\", false\n\t}\n\treturn ClaudeCode, true\n",
  "\t\treturn ClaudeCode, true\n\t}\n\treturn ClaudeCode, true\n", "TestH145_|TestHarnessArg")
m("H-145 a Codex declaration carries no turn", "internal/hook/harness.go",
  "\treturn &t.TurnID\n}", "\treturn nil\n}", "TestH145_|TestCodexTurn")
m("H-145 a Codex subagent's call keeps its own turn", "internal/hook/harness.go",
  "\tif agentID != \"\" {\n\t\tif parent, ok := st.LatestMainPromptID(sessionID); ok {",
  "\tif false {\n\t\tif parent, ok := st.LatestMainPromptID(sessionID); ok {", "TestH145_|TestCodexTurn")
m("H-145 the main agent's latest turn is read from a subagent's call", "internal/store/read.go",
  "\t\tif d.AgentID != nil && *d.AgentID != \"\" {\n\t\t\tcontinue\n\t\t}\n", "", "TestLatestMainPromptID")
m("H-145 Codex coverage reads Claude Code's configuration", "internal/hook/coverage.go",
  "\tif harness != ClaudeCode {\n\t\treturn resolveUnread(st, sessionID)\n\t}\n", "",
  "TestH145_|TestResolve_CodexReadsNoClaudeCodeConfiguration")
m("H-145 apply_patch is not a write", "internal/shape/shape.go",
  "\tcase \"apply_patch\":\n\t\treturn VerbWrite\n", "", "TestCodexToolClasses")
m("H-145 Codex's subagent tools are not agent calls", "internal/shape/shape.go",
  "\tcase \"collaborationspawn_agent\", \"collaborationwait_agent\":\n\t\treturn VerbAgent\n", "", "TestCodexToolClasses")

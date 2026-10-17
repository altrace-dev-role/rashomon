# BRD-1: harness.Detect returns cursor, codex, claude_code,
# claude_code_presumed or unknown per Technical spec 6.2.2, from key
# presence and env only.

m("BRD-1 cursor markers never fire",
  "internal/harness/detect.go",
  "\tif hasCursorVersion || hasConvGen {\n\t\treturn finish(Cursor, SurfaceUnknown, versionPtr, evidence)\n\t}",
  "\tif false {\n\t\treturn finish(Cursor, SurfaceUnknown, versionPtr, evidence)\n\t}",
  "TestBRD1_")

m("BRD-1 turn_id is never a Codex marker, on any event",
  "internal/harness/detect.go",
  "\tturnIDMatch := hasTurnID && codexTurnIDEvents[hookEventName]",
  "\tturnIDMatch := hasTurnID && false",
  "TestBRD1_")

m("BRD-1 model is a Codex marker even on SessionStart",
  "internal/harness/detect.go",
  "\tmodelMatch := hasModel && hookEventName != \"SessionStart\"",
  "\tmodelMatch := hasModel",
  "TestBRD1_")

m("BRD-1 PreToolUse is not in the Codex turn_id event set",
  "internal/harness/detect.go",
  "\t\"PreToolUse\":        true,\n\t\"PostToolUse\":       true,",
  "\t\"PreToolUse\":        false,\n\t\"PostToolUse\":       true,",
  "TestBRD1_")

m("BRD-1 claude-vscode maps to the cli surface, not ide",
  "internal/harness/detect.go",
  "\t\"claude-vscode\":             SurfaceIDE,",
  "\t\"claude-vscode\":             SurfaceCLI,",
  "TestBRD1_")

m("BRD-1 remote* is matched by exact string, not prefix",
  "internal/harness/detect.go",
  "\tif strings.HasPrefix(entrypoint, \"remote\") {",
  "\tif entrypoint == \"remote\" {",
  "TestBRD1_")

m("BRD-1 the Claude Code override needs only the entrypoint, not the transcript too",
  "internal/harness/detect.go",
  "\tclaudeOverride := isAllowedEntrypoint && transcriptUnderClaude",
  "\tclaudeOverride := isAllowedEntrypoint",
  "TestBRD1_")

m("BRD-1 a nil Payload is read as Claude Code, not Unknown",
  "internal/harness/detect.go",
  "\tif p == nil {\n\t\treturn unknown(nil)\n\t}",
  "\tif p == nil {\n\t\treturn finish(ClaudeCode, SurfaceCLI, nil, nil)\n\t}",
  "TestDetect_NilPayloadIsUnknown")

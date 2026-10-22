# BRD-1: harness.Detect returns cursor, codex, claude_code,
# claude_code_presumed or unknown per Technical spec 6.2.2, from key
# presence and env only.

m("BRD-1 cursor markers never fire",
  "internal/harness/detect.go",
  "\tif (hasCursorVersion || hasConvGen) && !claudeOverride {\n\t\treturn finish(Cursor, SurfaceUnknown, versionPtr, evidence)\n\t}",
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

m("BRD-1 the step-3 override does not reach the Cursor step, only Codex's",
  "internal/harness/detect.go",
  "\tif (hasCursorVersion || hasConvGen) && !claudeOverride {",
  "\tif (hasCursorVersion || hasConvGen) {",
  "TestDetect_ClaudeOverrideWinsOverCursorMarkers")

# The decision points below back no single numbered row by themselves (Caps,
# the version charset gate, the transcript-containment guard, evidence
# de-duplication and sorting). They are folded into this file -- rather than
# a numbered H-item -- because H-110 is on the never-reuse list (9.2.6) and
# no unclaimed row names them; their pinning tests carry the BRD1 suffix
# letters above TestBRD1_DetectsEachHarnessFromDocumentedPayloads to satisfy
# the per-row test-naming convention (9.2.6) while staying distinguishable
# from that table's own cases.

m("BRD-1 Caps grants mirror_in_band on the sdk surface",
  "internal/harness/caps.go",
  "\t\tif s == SurfaceSDK || s == SurfaceUnknown {\n\t\t\tout[CapMirrorInBand] = Unverified\n\t\t}",
  "\t\tif false {\n\t\t\tout[CapMirrorInBand] = Unverified\n\t\t}",
  "TestBRD1b_")

m("BRD-1 Cursor and Codex get every capability by default",
  "internal/harness/caps.go",
  "\tdefault: // Cursor, Codex, Unknown, and any future value\n\t\tfor _, c := range allCapabilities {\n\t\t\tout[c] = No\n\t\t}",
  "\tdefault: // Cursor, Codex, Unknown, and any future value\n\t\tfor _, c := range allCapabilities {\n\t\t\tout[c] = Yes\n\t\t}",
  "TestBRD1b_")

m("BRD-1 a too-long cursor_version is still stored",
  "internal/harness/detect.go",
  "\tif !present || len(v) == 0 || len(v) > 32 {",
  "\tif !present || len(v) == 0 {",
  "TestBRD1c_")

m("BRD-1 a space passes the cursor_version charset",
  "internal/harness/detect.go",
  "\tcase c == '.' || c == '+' || c == '-':",
  "\tcase c == '.' || c == '+' || c == '-' || c == ' ':",
  "TestBRD1c_")

m("BRD-1 transcript containment is a bare prefix, not a path relation",
  "internal/harness/detect.go",
  "\trel, err := filepath.Rel(projects, filepath.Clean(transcriptPath))\n\tif err != nil {\n\t\treturn false\n\t}\n\tif rel == \".\" {\n\t\treturn false // the transcript path names the directory itself, not a file in it\n\t}\n\treturn rel != \"..\" && !strings.HasPrefix(rel, \"..\"+string(filepath.Separator))",
  "\treturn strings.HasPrefix(filepath.Clean(transcriptPath), projects)",
  "TestBRD1d_")

m("BRD-1 evidence is never de-duplicated",
  "internal/harness/detect.go",
  "\t\tif !seen[e] {\n\t\t\tseen[e] = true\n\t\t\tout = append(out, e)\n\t\t}",
  "\t\t_ = seen\n\t\tout = append(out, e)",
  "TestBRD1e_")

m("BRD-1 evidence is never sorted",
  "internal/harness/detect.go",
  "\tsort.Slice(out, func(i, j int) bool { return out[i] < out[j] })",
  "\t_ = sort.Slice",
  "TestDetect_CursorEvidenceIsSortedWhenBothMarkersFire")

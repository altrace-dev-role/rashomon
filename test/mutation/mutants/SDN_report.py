# SDN -- a call Claude Code refuses while checking its input: a Read, Edit or
# Write a settings deny rule covers, an old_string not in the file, a file not
# read first. The check runs before PreToolUse, so no hook fires; each read as
# executed-but-unrecorded AND missing-from-store, and a session that did the
# right thing went unverified. Matched by the <tool_use_error> wrapper and
# excused only for an id no hook left any record of.
m("SDN a path-denied Edit is read as an execution", "internal/report/transcript.go",
  "\tpathDeniedPrefix,\n}", "}",
  "TestTranscript_RefusalsOutside|TestH30_AnEditDeniedByAPathRule")
m("SDN an Edit or Write under a Read deny rule is not a denial", "internal/report/transcript.go",
  "\treadDeniedPrefix,\n\tpathDeniedPrefix,\n}", "\tpathDeniedPrefix,\n}",
  "TestTranscript_RefusalsOutside|TestH30_AnEditOrWriteCoveredByAReadDenyRule")
m("SDN an input-check refusal is not kept as refused before any hook", "internal/report/transcript.go",
  "\t\t\t\tif b.IsError && strings.HasPrefix(text, toolUseErrorTag) {", "\t\t\t\tif false {",
  "TestTranscript_EveryInputCheckRefusalIsUnhooked|TestH30_AnEditDeniedByAPathRule|TestH30_AnEditRefusedWhileItsInput")
m("SDN every error result is kept as refused before any hook", "internal/report/transcript.go",
  "\t\t\t\tif b.IsError && strings.HasPrefix(text, toolUseErrorTag) {", "\t\t\t\tif b.IsError {",
  "TestTranscript_OnlyThePathRefusalIsUnhooked|TestH30_AnUndeclaredPromptDenial")
m("SDN a result that is not an error is kept as refused before any hook", "internal/report/transcript.go",
  "\t\t\t\tif b.IsError && strings.HasPrefix(text, toolUseErrorTag) {",
  "\t\t\t\tif strings.HasPrefix(text, toolUseErrorTag) {",
  "TestTranscript_TheToolUseErrorTagMustOpenAnErrorResult")
m("SDN the tag is matched anywhere in an error result", "internal/report/transcript.go",
  "\t\t\t\tif b.IsError && strings.HasPrefix(text, toolUseErrorTag) {",
  "\t\t\t\tif b.IsError && strings.Contains(text, toolUseErrorTag) {",
  "TestTranscript_TheToolUseErrorTagMustOpenAnErrorResult")
m("SDN a call refused before any hook is still missing from the store", "internal/report/report.go",
  "\t\tif !recorded[id] && !refused[id] {", "\t\tif !recorded[id] && len(refused) >= 0 {",
  "TestH30_AnEditDeniedByAPathRule|TestH30_AnEditRefusedWhileItsInput|TestAccounting_ARefusalNoHookSaw")
m("SDN a call refused before any hook is still executed but unrecorded", "internal/report/report.go",
  "\t\tif !executed[id] && !refused[id] {", "\t\tif !executed[id] && len(refused) >= 0 {",
  "TestH30_AnEditRefusedWhileItsInput|TestAccounting_ARefusalNoHookSaw")
m("SDN every call no hook recorded is excused from the store", "internal/report/report.go",
  "\t\tif unhooked[id] && !hooked[id] {", "\t\tif !hooked[id] && len(unhooked) >= 0 {",
  "TestH30_AnUndeclaredPromptDenial")
m("SDN a call a hook recorded is excused from the store", "internal/report/report.go",
  "\t\tif unhooked[id] && !hooked[id] {", "\t\tif unhooked[id] && len(hooked) >= 0 {",
  "TestAccounting_AnyHookRecordTakesTheExemptionAway")
m("SDN a declaration is not a hook record", "internal/report/report.go",
  "\tfor _, d := range run.Declarations {\n\t\thooked[d.ToolUseID] = true\n\t}\n", "",
  "TestAccounting_AnyHookRecordTakesTheExemptionAway")
m("SDN a terminal is not a hook record", "internal/report/report.go",
  "\tfor _, x := range run.Terminals {\n\t\thooked[x.ToolUseID] = true\n\t}\n", "",
  "TestAccounting_AnyHookRecordTakesTheExemptionAway")
m("SDN an execution is not a hook record", "internal/report/report.go",
  "\tfor id := range executed {\n\t\thooked[id] = true\n\t}\n", "",
  "TestAccounting_AnyHookRecordTakesTheExemptionAway")
m("SDN a readable transcript's refused list renders as unknown", "internal/report/report.go",
  "\trefused := map[string]bool{}\n\tt.RefusedBeforeHooks = []string{}\n", "\trefused := map[string]bool{}\n",
  "TestAccounting_AReadableTranscriptRendersEveryEmptyListAsNone")
m("SDN the calls refused before any hook are not named", "internal/report/report.go",
  "\t\t\trefused[id] = true\n\t\t\tt.RefusedBeforeHooks = append(t.RefusedBeforeHooks, id)\n",
  "\t\t\trefused[id] = true\n",
  "TestH30_AnEditDeniedByAPathRule|TestAccounting_ARefusalNoHookSaw")
m("SDN the refused-before-any-hook line is not printed", "internal/report/text.go",
  "\t\tfmt.Fprintf(b, \"    refused before any hook: %s\\n\", set(t.RefusedBeforeHooks))\n", "",
  "TestH30_AnEditRefusedWhileItsInput|TestReport_TextRendersUnknownNeverZero")

m("H-20 PostToolUse is not installed", "internal/install/install.go",
  "\t\tEventPostToolUse,\n\t\tEventPostToolUseFailure,\n", "", "TestH3_")
m("H-20 the post entry is installed without a matcher", "internal/install/install.go",
  "\treturn event == EventPreToolUse ||\n\t\tevent == EventPostToolUse ||\n\t\tevent == EventPostToolUseFailure",
  "\treturn event == EventPreToolUse", "TestH3_")
m("H-20 the execution record is never written", "internal/hook/post.go",
  "\tseq, err := p.st.LandExecution(rec)\n\tp.landedSeq, p.landedRunner, p.landedStore = seq, rec.RunnerDigest != nil, "
  "storeNamed(pl.ToolInput, p.st.Root())\n\tp.landedRed = redRead(&rec)\n\treturn landed(err, &p.withheld)",
  "\treturn nil",
  "TestH20_PostRecordsTheExecution")
m("H-20 an execution the lock refuses is dropped", "internal/store/landexec.go",
  "\t\treturn 0, s.SpillExecution(rec)", "\t\treturn 0, err", "TestAppendExecutionSpillsWhenTheLockIsHeld")
m("H-20 the post path ignores the install it was told it belongs to", "cmd/rashomon/post.go",
  "\t\tif standsDown(args, st, stderr) {\n\t\t\treturn nil\n\t\t}\n\n\t\tp := hook.NewPost(st, time.Now)",
  "\t\tp := hook.NewPost(st, time.Now)",
  "TestH20_PostFromAnotherInstallStandsDown")
m("H-20 post coverage resolves the recorder's entry instead of its own", "internal/hook/coverage.go",
  "\tif phase == store.PhasePost {", "\tif false {", "TestH20_PostCoverageReadsItsOwnEntry")
m("H-20 the report ignores tool_result blocks", "internal/report/transcript.go",
  "\t\t\tcase b.Type == \"tool_result\" && b.ToolUseID != \"\":", "\t\t\tcase false:", "TestH20_ExecutionAccounting")

m("H-20 a result with no execution record is not a coverage failure", "internal/report/report.go",
  "\t\tif t.Readable && len(t.ExecutedButUnrecorded) > 0 {\n\t\t\tsess.Coverage.add(ReasonExecutionMismatch)\n\t\t}\n", "",
  "TestH20_ExecutionAccounting")
m("H-20 a declaration with no result is treated as a coverage failure", "internal/report/report.go",
  "\t\tif t.Readable && len(t.ExecutedButUnrecorded) > 0 {", "\t\tif t.Readable && len(t.DeclaredWithoutResult) > 0 {",
  "TestH20_ExecutionAccounting")
m("H-20 declarations with no execution are not named", "internal/store/read.go",
  "\t\tif !executed[d.ToolUseID] {", "\t\tif false {", "TestH20_DeclarationWithoutAnExecutionIsNamed")
m("H-20 the unexecuted list drops the permission mode", "internal/report/report.go",
  "Unexecuted{ToolUseID: id, PermissionMode: mode[id]})", "Unexecuted{ToolUseID: id})",
  "TestH20_DeclarationWithoutAnExecutionIsNamed")
m("H-20 the post path runs outside the panic barrier", "cmd/rashomon/post.go",
  "\t\tcaptureErr := safe.Guard(func() error {\n\t\t\tif payload != nil {\n\t\t\t\treturn p.CapturePayload(payload)\n\t\t\t}\n\t\t\treturn p.Capture(in())\n\t\t})",
  "\t\tvar captureErr error\n\t\tif payload != nil {\n\t\t\tcaptureErr = p.CapturePayload(payload)\n\t\t} else {\n\t\t\tcaptureErr = p.Capture(in())\n\t\t}",
  "TestH20_NoFaultOnThePostPathReachesTheAgent")

m("H-20 the post payload declares tool_response, and it reaches the debug log", "internal/hook/post.go",
  "\tvar pl PostPayload\n", "\tvar pl PostPayload\n\tdefer func() { fmt.Fprintln(os.Stderr, string(payload.Value(hookio.ToolResponse))) }()\n",
  "TestH20_ToolResponseNeverReachesDisk")
IMPORTS["H-20 the post payload declares tool_response, and it reaches the debug log"] = ("internal/hook/post.go", '\t"io"\n', '\t"fmt"\n\t"io"\n\t"os"\n')
m("H-20 the response is measured, and the measurement moves the record's width", "internal/hook/post.go",
  "\t\tToolName:      pl.ToolName,\n",
  "\t\tToolName:      pl.ToolName + strconv.Itoa(len(payload.Raw)),\n",
  "TestH20_ExecutionWidthIsIndependentOfTheResponse")

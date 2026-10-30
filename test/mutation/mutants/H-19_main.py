m("H-19 report re-reads today's config to judge a past run", "internal/report/assemble.go",
  "\tsess := build(run, in.Transcripts)\n", "\tsess := build(run, in.Transcripts)\n\tif p, err := settings.UserPath(); err == nil {\n\t\tif doc, err := settings.Load(p); err == nil {\n\t\t\tif ok, _ := install.Present(doc, sess.InstallID, install.EventPreToolUse); !ok {\n\t\t\t\tsess.Coverage.add(store.ReasonHookEntryAbsent)\n\t\t\t}\n\t\t}\n\t}\n", "TestH19_")
IMPORTS["H-19 report re-reads today's config to judge a past run"] = ("internal/report/assemble.go", '\t"github.com/altrace-dev-role/rashomon/internal/store"\n', '\t"github.com/altrace-dev-role/rashomon/internal/install"\n\t"github.com/altrace-dev-role/rashomon/internal/settings"\n\t"github.com/altrace-dev-role/rashomon/internal/store"\n')

m("report text renders a count it does not have as 0", "internal/report/text.go",
  "\tif p == nil {\n\t\treturn notRead\n\t}", "\tif p == nil {\n\t\treturn \"0\"\n\t}", "TestReport_")
m("report text renders a comparison it could not make as an empty list", "internal/report/text.go",
  "\tif ids == nil {\n\t\treturn unknown\n\t}", "\tif ids == nil {\n\t\treturn none\n\t}", "TestReport_")
m("report --json is accepted and ignored", "cmd/rashomon/report.go",
  "\t\tcase \"--json\":\n\t\t\tasJSON = true\n\t\tcase \"--redact\":", "\t\tcase \"--json\":\n\t\t\tasJSON = false\n\t\tcase \"--redact\":", "TestReport_")
m("status opens the store, which creates one", "cmd/rashomon/verbs_status.go",
  "\tif _, err := os.Stat(filepath.Join(root, installMetaFile)); err != nil {\n\t\treturn nil, nil\n",
  "\tif _, err := os.Stat(filepath.Join(root, installMetaFile)); false && err != nil {\n\t\treturn nil, nil\n",
  "TestH18_")
m("status calls recording active on a fresh machine", "cmd/rashomon/status_recording.go",
  "\tcase state == RecordingActive && !installed:",
  "\tcase state == RecordingActive && false:", "TestStatus_OnAFreshMachineSaysSo")
m("status calls every unpaused machine not installed", "cmd/rashomon/status_recording.go",
  "\tcase state == RecordingActive && !installed:",
  "\tcase state == RecordingActive && true:", "TestStatus_SaysWhetherRecordingIsPaused")
m("status calls an enabled plugin with no store yet not installed", "cmd/rashomon/status_recording.go",
  "\t\tinstalled, err = pluginEnabled()",
  "\t\tinstalled, err = false, nil", "TestStatus_AnEnabledPluginRecordsBeforeAnyStoreExists")
m("status reports an entry present whatever it finds", "cmd/rashomon/main.go",
  "\tcase present:\n\t\treturn \"present\"\n\tdefault:\n\t\treturn \"absent\"\n\t}",
  "\tcase present:\n\t\treturn \"present\"\n\tdefault:\n\t\treturn \"present\"\n\t}", "TestStatus_")
m("status does not name the other installs sharing the file", "cmd/rashomon/main.go",
  "\t\tfmt.Fprintf(stdout, \"  %s: %s\\n\", label, strings.Join(others, \", \"))",
  "\t\tfmt.Fprintf(stdout, \"  %s: none\\n\", label)", "TestStatus_")
m("status does not resolve the layer that disabled hooks", "cmd/rashomon/main.go",
  "\tcase decision.Disabled:\n\t\tfmt.Fprintf(stdout, \"hooks: disabled by the %s settings layer\\n\", decision.Layer)",
  "\tcase false:\n\t\tfmt.Fprintf(stdout, \"hooks: disabled by the %s settings layer\\n\", decision.Layer)", "TestStatus_")
# Commands that take no arguments. noArgs is the one refusal, and each call
# site also gets its own break -- back to the line that ignored what followed
# the command -- because dropping one site leaves the guard, and every other
# site's test, green.
m("a command that takes no arguments runs whatever follows it", "cmd/rashomon/main.go",
  "\t\tif len(args) > 0 {\n\t\t\treturn fmt.Errorf(\"unknown argument %q\", args[0])\n\t\t}\n\t\treturn cmd(stdout)",
  "\t\treturn cmd(stdout)", "TestArguments_")
m("the no-argument refusal is worded unlike every other command's", "cmd/rashomon/main.go",
  "\t\t\treturn fmt.Errorf(\"unknown argument %q\", args[0])",
  "\t\t\treturn fmt.Errorf(\"takes no arguments, got %q\", args[0])", "TestArguments_EveryListedCommandRefusesHelp")
m("watch --help installs the recorders", "cmd/rashomon/watch.go",
  "\t\tdefault:\n\t\t\treturn false, false, fmt.Errorf(\"unknown argument %q\", a)",
  "\t\tdefault:\n\t\t\tcontinue", "TestArguments_Watch")
m("pause --help pauses recording", "cmd/rashomon/main.go",
  "\t\treturn guarded(stderr, noArgs(rest, stdout, cmdPause))",
  "\t\treturn guarded(stderr, func() error { return cmdPause(stdout) })", "TestArguments_Pause")
m("resume --help lifts the pause", "cmd/rashomon/main.go",
  "\t\treturn guarded(stderr, noArgs(rest, stdout, cmdResume))",
  "\t\treturn guarded(stderr, func() error { return cmdResume(stdout) })", "TestArguments_Resume")
m("status --help prints the status", "cmd/rashomon/verbs_status.go",
  "\tasJSON, err := jsonFlag(args)\n\tif err != nil {\n\t\treturn err\n\t}\n\tif !asJSON {\n\t\treturn cmdStatus(stdout)",
  "\tasJSON, _ := jsonFlag(args)\n\tif !asJSON {\n\t\treturn cmdStatus(stdout)", "TestArguments_Status")
m("version --help prints the version", "cmd/rashomon/main.go",
  "\t\treturn guarded(stderr, noArgs(rest, stdout, printVersion))",
  "\t\treturn guarded(stderr, func() error { return printVersion(stdout) })", "TestArguments_Version")

# LA-1: a call that may have run a gate is one that launches, in any command
# of its line (card LAUNCH-ANY1): a script named by its extension and not
# asked to report, or a shell running a script, its standard input or a -c
# line that launches or names a runner, never a shell asked only to read.
# The hook records the bit on the declaration at call schema 10, omitted
# when false; the post hook reads a launch's output; verify and the report
# read the bit from schema 10 and the first program before it. Each break
# removes one rule and names the tests that hold it.
L = "internal/shape/launch.go"
H = "internal/hook/handle.go"
P = "internal/hook/post.go"
V = "internal/verify/declared.go"
R = "internal/report/testbending.go"
S = "internal/store/record.go"
m("LA-1 only a line's first command is read", L,
  "\tfor _, c := range cmds {\n", "\tfor _, c := range cmds[:min(len(cmds), 1)] {\n",
  "TestLA1_ALaunchIsReadInEveryCommand|TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun")
m("LA-1 a syntax check launches", L,
  "a[0] == '-' && strings.ContainsRune(letters, 'n')", "false",
  "TestLA1_ALaunchIsReadInEveryCommand|TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun|TestLA1_ALineNamesTheLaunchNotALaterCheck")
m("LA-1 noexec launches", L,
  "if a[0] == '-' && args[i] == \"noexec\" {", "if false {",
  "TestLA1_ALaunchIsReadInEveryCommand")
m("LA-1 a script asked to report launches", L,
  "\t\treturn len(args) == 0 || !reportWords[args[0]]\n", "\t\treturn true\n",
  "TestLA1_ALaunchIsReadInEveryCommand|TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun|TestLA1_ALineNamesTheLaunchNotALaterCheck")
m("LA-1 a shell's script asked to report launches", L,
  "\t\treturn i+1 >= len(args) || !reportWords[args[i+1]]\n", "\t\treturn true\n",
  "TestLA1_ALaunchIsReadInEveryCommand")
m("LA-1 a shell asked for its version launches", L,
  "\t\t\tif reportWords[a] {\n\t\t\t\treturn false\n", "\t\t\tif false {\n\t\t\t\treturn false\n",
  "TestLA1_ALaunchIsReadInEveryCommand")
m("LA-1 every -c line launches", L,
  "\t\t\treturn depth < maxLaunchDepth && (launchesIn(args[i], depth+1) || namesRunner(args[i]))\n", "\t\t\treturn true\n",
  "TestLA1_ALaunchIsReadInEveryCommand|TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun")
m("LA-1 a -c line is not looked into", L,
  "\t\t\treturn depth < maxLaunchDepth && (launchesIn(args[i], depth+1) || namesRunner(args[i]))\n", "\t\t\treturn false\n",
  "TestLA1_ALaunchIsReadInEveryCommand|TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun")
m("LA-1 a -c line's runner is no launch", L,
  "(launchesIn(args[i], depth+1) || namesRunner(args[i]))", "launchesIn(args[i], depth+1)",
  "TestLA1_ALaunchIsReadInEveryCommand")
m("LA-1 a nested -c line is not looked into", L,
  "depth < maxLaunchDepth && (launchesIn", "depth < 1 && (launchesIn",
  "TestLA1_ALaunchIsReadInEveryCommand")
m("LA-1 a shell reading its input launches nothing", L,
  "\treturn !cline\n}\n", "\treturn false\n}\n",
  "TestLA1_ALaunchIsReadInEveryCommand")
m("LA-1 a stage of assignments stops the reading", L,
  "\tcmds, _ := commands(line, true)\n", "\tcmds, _ := commands(line, false)\n",
  "TestLA1_ALaunchIsReadInEveryCommand|TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun")
m("LA-1 Commands reads past assignments for every caller", "internal/shape/commands.go",
  "\treturn commands(line, false)\n", "\treturn commands(line, true)\n",
  "TestLA1_ALaunchIsReadInEveryCommand")
m("LA-1 the byte scan drops a shell's name", L,
  "(j == len(line) || !wordByte(line[j])) {\n\t\t\t\treturn true\n", "(j == len(line) || !wordByte(line[j])) {\n\t\t\t\treturn false\n",
  "TestLA1_ALaunchIsReadInEveryCommand")
m("LA-1 an older record reads its bit", L,
  "\tif recorded {\n\t\treturn launch\n", "\tif true {\n\t\treturn launch\n",
  "TestLA1_ARecordBeforeTheBitIsReadByItsProgram|TestWR1_AWrapperRunStartsWithAShellOrAScript")
m("LA-1 a newer record reads its program", L,
  "\tif recorded {\n\t\treturn launch\n", "\tif false {\n\t\treturn launch\n",
  "TestLA1_ARecordBeforeTheBitIsReadByItsProgram|TestLA1_ALineNamesTheLaunchNotALaterCheck")
m("LA-1 the hook records no launch", H,
  "\tdecl.Launch = decl.Shape.Launch\n", "",
  "TestLA1_TheDeclarationRecordsALaunch|TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun")
m("LA-1 the post hook reads a line's first program only", P,
  "shape.WrapperRun(pl.ToolName, s.Launch,", "shape.WrapperRun(pl.ToolName, shape.RecordedLaunch(s.Program, false, false),",
  "TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun")
m("LA-1 verify reads every record by its program", V,
  "d.SchemaVersion >= store.LaunchSchemaVersion),\n\t\t\trunner, masked", "false),\n\t\t\trunner, masked",
  "TestLA1_ALineNamesTheLaunchNotALaterCheck|TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun")
m("LA-1 the report counts a run by its program", R,
  "d.SchemaVersion >= store.LaunchSchemaVersion),\n\t\t\td.Shape.RunnerDigest", "false),\n\t\t\td.Shape.RunnerDigest",
  "TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun")
m("LA-1 the bit is written false", S,
  "`json:\"launch,omitempty\"`", "`json:\"launch\"`",
  "TestLA1_TheLaunchBitIsOmittedWhenFalse|TestLA1_TheDeclarationRecordsALaunch")
m("LA-1 the bit is read from no record", S,
  "const LaunchSchemaVersion = 10\n", "const LaunchSchemaVersion = 11\n",
  "TestLA1_TheLaunchBitIsOmittedWhenFalse|TestLA1_ALineNamesTheLaunchNotALaterCheck")
m("LA-1 the schema names no launch key", "docs/store-schema.json",
  "        \"launch\": {\n", "        \"launched\": {\n",
  "TestStoreSchemaMatchesTheAllowlists")

# C8: the lint, typecheck and scan runner lists (PF-PG1). A live gate's kind is
# the list its runner is on: test, build, lint, typecheck or scan. Each break
# removes one row, one list, one rule of how the lists are read or one place
# the kind is spelled -- the schema's enum, gateout's accepted kinds, the rules
# version -- and names the test that holds it.
M = "internal/shape/masked.go"
K = "TestC8_RunnerKindIsTheListsWord"

# The rows.
m("C8 ruff, flake8 and pylint are not lint runners", M,
  "\t\t{\"ruff\", \"check\"}, {\"flake8\"}, {\"pylint\"},\n", "", K)
m("C8 golangci-lint run is not a lint runner", M,
  "{\"golangci-lint\", \"run\"}, ", "", K)
m("C8 go vet is a build again", M,
  "\t{\"go\", \"build\"}, {\"cargo\", \"build\"},\n", "\t{\"go\", \"build\"}, {\"go\", \"vet\"}, {\"cargo\", \"build\"},\n",
  "TestC8_TheRunnerListsAreDisjoint|" + K)
m("C8 tsc --noEmit is not a typecheck", M,
  "\t\t{\"tsc\", \"--noEmit\"}, {\"npx\", \"tsc\", \"--noEmit\"},\n", "", K)
m("C8 mypy is not a typecheck runner", M,
  "\t\t{\"mypy\"}, {\"python\", \"-m\", \"mypy\"}, {\"python3\", \"-m\", \"mypy\"}, {\"pyright\"},\n", "", K)
m("C8 semgrep and bandit are not scan runners", M,
  "\t\t{\"semgrep\"}, {\"bandit\"}, {\"gosec\"},", "\t\t{\"gosec\"},", K)
m("C8 npm audit is not a scan runner", M,
  "\t\t{\"pip-audit\"}, {\"npm\", \"audit\"}, {\"cargo\", \"audit\"},\n", "\t\t{\"pip-audit\"}, {\"cargo\", \"audit\"},\n", K)

# The lists and how they are read.
m("C8 the lint list is not read", M,
  "\t{RunnerLint, MaskedBuild, lintCommands, lintCommandNames},\n", "", K)
m("C8 the scan list is not read", M,
  "\t{RunnerScan, MaskedBuild, scanCommands, scanCommandNames},\n", "", K)
m("C8 the build list is read before the check lists", M,
  "\t{RunnerLint, MaskedBuild, lintCommands, lintCommandNames},\n",
  "\t{RunnerBuild, MaskedBuild, buildCommands, buildCommandNames},\n\t{RunnerLint, MaskedBuild, lintCommands, lintCommandNames},\n", K)
m("C8 a check runner's kind is its masked word", M,
  "\tk := p.runners[g].list\n", "\tk := p.runners[g].kind\n", K)
m("C8 a check runner hides its status as a test", "internal/shape/masked_simple.go",
  "kind: l.masked, list: l.kind,", "kind: MaskedTest, list: l.kind,", K)
m("C8 a lint line is a test run", "internal/shape/masked_simple.go",
  "kind: l.masked, list: l.kind,", "kind: l.masked, list: RunnerTest,", K)
m("C8 a runner name past the build list is not read back", "internal/shape/masked_name.go",
  "\t\ti -= len(l.names())\n", "\t\ti -= len(testCommandNames())\n", "TestMO1_TheIdsAreThePlacesInTheLists")
m("C8 RunnerNames leaves the scan list out", "internal/shape/masked_name.go",
  "\tfor _, l := range runnerLists {\n\t\tout = append(out, l.names()...)\n",
  "\tfor _, l := range runnerLists[:len(runnerLists)-2] {\n\t\tout = append(out, l.names()...)\n", "TestC8_RunnerNamesAreEveryRow|TestMO1_")

# The vocabulary and where it is spelled.
m("C8 RunnerKinds leaves out scan", M,
  "RunnerLint, RunnerTypecheck, RunnerScan}", "RunnerLint, RunnerTypecheck}", "TestC8_RunnerKindsAreTheFiveLists|TestStoreSchemaRunnerKindsAreTheCodeRunnerKinds")
m("C8 the schema offers no scan kind", "docs/store-schema.json",
  "                \"typecheck\",\n                \"scan\"\n", "                \"typecheck\"\n", "TestStoreSchemaRunnerKindsAreTheCodeRunnerKinds")
m("C8 gateout does not accept the lint kind", "internal/gateout/gateout.go",
  "\tcase \"test\", \"build\", \"lint\", \"typecheck\", \"scan\":\n", "\tcase \"test\", \"build\", \"typecheck\", \"scan\":\n", "TestC8_CheckKindsAreNoSummary|TestC8_ALintRunIsAGateOfKindLint")
m("C8 gateout reads a lint run's output as a test's", "internal/gateout/gateout.go",
  "\tif kind != \"test\" {\n", "\tif kind != \"test\" && kind != \"lint\" {\n", "TestC8_CheckKindsAreNoSummary")
m("C8 the runner lists change without a rules version bump", M,
  "{\"pip-audit\"}, {\"npm\", \"audit\"}, {\"cargo\", \"audit\"},\n", "{\"pip-audit\"}, {\"npm\", \"audit\"}, {\"cargo\", \"audit\"}, {\"trivy\"},\n", "TestRulesVersion_RunnerLists")
m("C8 the rules version is not bumped", M,
  "const RulesVersion = 12\n", "const RulesVersion = 11\n", "TestMO1_RulesVersionIsTwelve|TestRulesVersion_")

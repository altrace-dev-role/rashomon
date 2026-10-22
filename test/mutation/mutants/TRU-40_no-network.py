# TRU-40: no networking package anywhere in the binary. A blank import is the
# cheapest way in, and the one a health check or a telemetry client would use.
m("TRU-40 the report imports net/http", "internal/report/report.go",
  '\t"sort"\n', '\t_ "net/http"\n\t"sort"\n', "TestTRU40_BinaryHasNoNetworkingPackage")
m("TRU-40 the report imports os/exec with nothing sanctioning it", "internal/report/report.go",
  '\t"sort"\n', '\t_ "os/exec"\n\t"sort"\n', "TestTRU40_OsExecEntersOnlyThroughSanctionedImporters")
# Every release target, as .goreleaser.yml builds it. Each of these lists a
# build the release does not ship, where a _windows.go file or one built
# only without cgo can import net and pass.
m("TRU-40 the listing runs as the test machine's build", "test/acceptance/release_targets_test.go",
  "\t\tcmd.Env = releaseListEnv(target, m.env)",
  "\t\tcmd.Env = os.Environ()", "TestTRU40_")
m("TRU-40 the listing builds with cgo on", "test/acceptance/release_targets_test.go",
  "\t\tcmd.Env = releaseListEnv(target, m.env)",
  '\t\tcmd.Env = append(releaseListEnv(target, m.env), "CGO_ENABLED=1")',
  "TestTRU40_")
m("TRU-40 the matrix reader takes cgo off on trust", "test/acceptance/release_targets_test.go",
  '\treturn errors.New("the build\'s env does not say CGO_ENABLED=0")', "\treturn nil",
  "TestTRU40_ReleaseTargetsAreReadFromGoreleaser")
m("TRU-40 the union leaves a target out", "test/acceptance/release_targets_test.go",
  "\tfor _, pkgs := range byTarget {\n\t\tfor _, p := range pkgs {\n\t\t\tset[p.path] = true",
  "\tfor target, pkgs := range byTarget {\n\t\tif target.goos == \"windows\" {\n\t\t\tcontinue\n\t\t}\n"
  "\t\tfor _, p := range pkgs {\n\t\t\tset[p.path] = true",
  "TestTRU40_EveryTargetIsListedAsItShips")
m("TRU-40 a listing is never checked for the build it names", "test/acceptance/release_targets_test.go",
  "\tif windows != isWindows || unix == isWindows {",
  "\tif false && (windows != isWindows || unix == isWindows) {",
  "TestTRU40_EveryTargetIsListedAsItShips")
# The reader itself, round two. Each of these read a .goreleaser.yml shape
# that ships a build the listing never makes: a pair go list accepts and no
# toolchain builds, a key or flag that changes what links, a list item read
# as the build's own, a quoted entry the release unquotes, a target left
# out of the listing, and a listing whose module column is gone.
m("TRU-40 the matrix reader takes a pair no toolchain builds", "test/acceptance/release_targets_test.go",
  "\t\t\tif !known[t] {", "\t\t\tif false && !known[t] {", "TestTRU40_ReleaseTargetsAreReadFromGoreleaser")
m("TRU-40 a listing need not link its own syscall package", "test/acceptance/release_targets_test.go",
  "\tif windows != isWindows || unix == isWindows {", "\tif windows != isWindows {",
  "TestTRU40_EveryTargetIsListedAsItShips")
m("TRU-40 the matrix reader reads past a key it does not model", "test/acceptance/release_targets_test.go",
  "\t\tcase !ok || !modelled:", "\t\tcase !ok || false && !modelled:", "TestTRU40_ReleaseTargetsAreReadFromGoreleaser")
m("TRU-40 the matrix reader passes a flag it does not model", "test/acceptance/release_targets_test.go",
  "\t\tif !buildFlags[f] {", "\t\tif false && !buildFlags[f] {", "TestTRU40_ReleaseTargetsAreReadFromGoreleaser")
m("TRU-40 the matrix reader takes a nested item as the build's own", "test/acceptance/release_targets_test.go",
  '\t\tcase indent == 6 && key != "" && strings.HasPrefix(trimmed, "- "):',
  '\t\tcase indent >= 6 && key != "" && strings.HasPrefix(trimmed, "- "):',
  "TestTRU40_ReleaseTargetsAreReadFromGoreleaser")
m("TRU-40 the matrix reader takes a quoted entry as written", "test/acceptance/release_targets_test.go",
  '\t\tif strings.ContainsAny(trimmed, "\\"\'\\t") {', "\t\tif false {",
  "TestTRU40_ReleaseTargetsAreReadFromGoreleaser")
m("TRU-40 the listing leaves a target out", "test/acceptance/release_targets_test.go",
  "\tfor _, target := range m.targets {\n",
  "\tfor _, target := range m.targets {\n\t\tif target.goos == \"windows\" {\n\t\t\tcontinue\n\t\t}\n", "TestTRU40_")
m("TRU-40 the listing drops the module column", "test/acceptance/release_targets_test.go",
  '"{{with .Module}} {{.Path}} {{.Version}}{{end}}"', '""', "TestTRU40_")
# The file's root. A root env reaches every build and a before hook can write
# code ahead of one, and the listing applies neither, so the reader refuses
# any root key but the six it knows.
m("TRU-40 the matrix reader reads past a root key it does not model", "test/acceptance/release_targets_test.go",
  "\t\t\tif !rootKeys[name] {", "\t\t\tif false && !rootKeys[name] {", "TestTRU40_ReleaseTargetsAreReadFromGoreleaser")

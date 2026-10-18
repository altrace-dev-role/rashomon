# DEL-21: every setup-go pin across the workflows is one version. A bump that
# misses one job, by a patch version: the test compares the exact string.
m("DEL-21 the sweep job pins a different Go patch version", ".github/workflows/ci.yaml",
  "          go-version: \"1.27.2\"  # See the note in the test job.\n          cache: true\n          cache-dependency-path: go.sum  # See the test job.\n\n      # Same as in Test",
  "          go-version: \"1.27.1\"  # See the note in the test job.\n          cache: true\n          cache-dependency-path: go.sum  # See the test job.\n\n      # Same as in Test", "TestDEL21_")
# The same disagreement, but on the release side: nothing before this mutant
# broke if the file that drifted was release.yaml rather than ci.yaml.
m("DEL-21 release.yaml pins a different Go patch version than ci.yaml", ".github/workflows/release.yaml",
  "          go-version: \"1.27.2\"\n", "          go-version: \"1.27.1\"\n", "TestDEL21_")
# The parser must refuse a setup-go step with no pin, in any step form, by
# tying the pin to its own step rather than counting the two across the file.
m("DEL-21 the pin parser accepts a setup-go step without a pin", "test/mutation/sweep_test.go",
  "\t\tif cur != nil && cur.isSetupGo && !cur.pinSeen {\n", "\t\tif false {\n", "TestDEL21b_")
# A quoted go-version value that is not MAJOR.MINOR.PATCH -- "stable", a
# wildcard like "1.27.x" -- is a toolchain nobody chose here, same as an
# unquoted one; the parser must refuse it rather than accept any quoted string.
m("DEL-21 the pin parser accepts a non-version quoted string", "test/mutation/sweep_test.go",
  "\texactVersion := regexp.MustCompile(`^\\d+\\.\\d+\\.\\d+$`)\n",
  "\texactVersion := regexp.MustCompile(`.*`)\n", "TestDEL21b_")
# A quoted `uses:` value -- some actions pin it that way -- names the same
# action and must still be recognised as a setup-go step.
m("DEL-21 the pin parser misses a quoted uses: line", "test/mutation/sweep_test.go",
  '`^\\s*(-\\s+)?uses:\\s*"?([^"\\s]+)"?\\s*(#.*)?$`',
  '`^\\s*(-\\s+)?uses:\\s*([^"\\s]+)\\s*(#.*)?$`', "TestDEL21b_")
# A pin below go.mod's own `go` line builds the release with a toolchain
# older than the module declares it needs, even when every workflow agrees
# with every other -- the comparison must be numeric, not a string compare
# that sorts "1.9.0" above "1.10.0".
m("DEL-21 the go.mod floor comparison is inverted", "test/mutation/sweep_test.go",
  "\t\t\treturn v[i] > f[i], nil\n", "\t\t\treturn v[i] < f[i], nil\n", "TestDEL21e_")
# A trip-wire that cannot itself trip is not a trip-wire.
m("DEL-21 the pin review deadline is in the past", "test/mutation/sweep_test.go",
  "\treviewBy := time.Date(2027, time.July, 1, 0, 0, 0, 0, time.UTC)\n",
  "\treviewBy := time.Date(2020, time.July, 1, 0, 0, 0, 0, time.UTC)\n", "TestDEL21f_")
# The test job's govulncheck drifts from the release job's.
m("DEL-21 the test job runs an older govulncheck than the release", ".github/workflows/ci.yaml",
  "        run: go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...\n",
  "        run: go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...\n", "TestDEL21c_")

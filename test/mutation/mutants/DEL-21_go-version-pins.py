# DEL-21: every setup-go pin across the workflows is one version. A bump that
# misses one job, by a patch version: the test compares the exact string.
m("DEL-21 the sweep job pins a different Go patch version", ".github/workflows/ci.yaml",
  "          go-version: \"1.27.2\"  # See the note in the test job.\n          cache: true\n          cache-dependency-path: go.sum  # See the test job.\n\n      # Same as in Test",
  "          go-version: \"1.27.1\"  # See the note in the test job.\n          cache: true\n          cache-dependency-path: go.sum  # See the test job.\n\n      # Same as in Test", "TestDEL21_")
# The parser must refuse a setup-go step with no pin, in any step form.
m("DEL-21 the pin parser accepts a setup-go step without a pin", "test/mutation/sweep_test.go",
  "\tif steps != len(pins) {\n", "\tif false {\n", "TestDEL21b_")
# The test job's govulncheck drifts from the release job's.
m("DEL-21 the test job runs an older govulncheck than the release", ".github/workflows/ci.yaml",
  "        run: go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...\n",
  "        run: go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...\n", "TestDEL21c_")

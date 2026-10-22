# RES-36: the perf jobs run under the toolchain the rows were measured under,
# every piped step runs with pipefail (shell: bash), and the RSS steps
# require every owed gate to have run. The CI failure on #59 was the first.
# The test job carries the same pin and cache lines, so the anchor starts at
# the perf-linux job's own comment.
m("RES-36 the linux perf job builds with go1.25.8 again", ".github/workflows/ci.yaml",
  '          # toolchain, so this pin moves only with a remeasurement.\n          go-version: "1.27.2"\n          cache: true\n',
  '          # toolchain, so this pin moves only with a remeasurement.\n          go-version: "1.25.8"\n          cache: true\n', "TestRES36h_")
m("RES-36 the bench pipe runs without pipefail", ".github/workflows/ci.yaml",
  "      # midway would pass on whatever partial output reached the gate.\n      - name: Benchmark allocations against the budgets\n        shell: bash\n        run: |\n          set -o pipefail\n",
  "      # midway would pass on whatever partial output reached the gate.\n      - name: Benchmark allocations against the budgets\n        run: |\n", "TestRES36h_")
m("RES-36 the macOS RSS step passes when every gate skipped", ".github/workflows/ci.yaml",
  '        env:\n          RASHOMON_PERF_REQUIRE_ALL: "1"  # See the perf-linux job.\n', "", "TestRES36h_")
m("RES-36 budgets.json names a toolchain the perf jobs do not run", "test/perf/budgets.json",
  '"go_version": "go1.27.2"', '"go_version": "go1.25.8"', "TestRES36h_")

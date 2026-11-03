# test-1: the CI Fuzz job bounds minimization, its floor is the number of
# fuzz targets the tree holds (rubric fix list entry 23), it takes its
# targets from go test -list and counts a run only when it fuzzed, runs every
# target before it fails, and keeps a failure's inputs. The tests read
# ci.yaml by its lines.
m("test-1  fuzz minimization left at its one-minute default", ".github/workflows/ci.yaml",
  " -fuzztime 30s -fuzzminimizetime 10x 2>&1 | tee", " -fuzztime 30s 2>&1 | tee", "TestRubricTEST1_")
m("test-1  the fuzz job's floor is left behind the tree", ".github/workflows/ci.yaml",
  "          test \"$found\" -ge 35\n", "          test \"$found\" -ge 10\n", "TestRubricTEST1_")
m("test-1  the fuzz job finds its targets by grep", ".github/workflows/ci.yaml",
  "for name in $(go test \"$pkg\" -list '^Fuzz' | grep '^Fuzz' || true); do",
  "for name in $(grep -rho '^func Fuzz[A-Za-z0-9_]*' \"${pkg#github.com/altrace-dev-role/rashomon/}\" | cut -c6-); do", "TestRubricTEST1_")
m("test-1  a run that did not fuzz counts", ".github/workflows/ci.yaml",
  "                if ! grep -q '^fuzz: elapsed' \"$log\"; then", "                if false; then", "TestRubricTEST1_")
m("test-1  the first failing target stops the rest", ".github/workflows/ci.yaml",
  "                failed+=(\"${pkg} ${name}\")\n                corpus=",
  "                failed+=(\"${pkg} ${name}\")\n                exit 1\n                corpus=", "TestRubricTEST1_")
m("test-1  the corpus is listed whether or not it exists", ".github/workflows/ci.yaml",
  "                if [ -d \"$corpus\" ]; then", "                if true; then", "TestRubricTEST1_")
m("test-1  a failure's inputs are not kept", ".github/workflows/ci.yaml",
  "        if: failure()\n        uses: actions/upload-artifact@", "        if: success()\n        uses: actions/upload-artifact@", "TestRubricTEST1_")
# CI-PERF-SPLIT1: the job runs as shards 1 to n, each fuzzing the targets
# whose index falls to it and checking it ran its share, each keeping its
# failing inputs under its own artifact name.
m("test-1  the fuzz matrix runs one shard", ".github/workflows/ci.yaml",
  "        shard: [1, 2]\n", "        shard: [1]\n", "TestRubricTEST1_")
m("test-1  a shard fuzzes the other shard's targets", ".github/workflows/ci.yaml",
  "% SHARDS + 1 )) -ne \"$SHARD\" ]; then", "% SHARDS + 1 )) -eq \"$SHARD\" ]; then", "TestRubricTEST1_")
m("test-1  a shard that ran less than its share passes", ".github/workflows/ci.yaml",
  "          test \"$ran\" -ge $((found / SHARDS))\n", "", "TestRubricTEST1_")
m("test-1  the shards keep their failing inputs under one name", ".github/workflows/ci.yaml",
  "          name: fuzz-failing-inputs-${{ matrix.shard }}\n", "          name: fuzz-failing-inputs\n", "TestRubricTEST1_")
m("test-1  the shard count is written by hand", ".github/workflows/ci.yaml",
  "          SHARDS: ${{ strategy.job-total }}\n", "          SHARDS: 2\n", "TestRubricTEST1_")

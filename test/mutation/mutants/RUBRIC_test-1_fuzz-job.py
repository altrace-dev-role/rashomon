# test-1: the CI Fuzz job bounds minimization, its floor is the number of
# fuzz targets the tree holds (rubric fix list entry 23), it takes its
# targets from go test -list and counts a run only when it fuzzed, runs every
# target before it fails, and keeps a failure's inputs. The tests read
# ci.yaml by its lines.
m("test-1  fuzz minimization left at its one-minute default", ".github/workflows/ci.yaml",
  " -fuzztime 30s -fuzzminimizetime 10x 2>&1 | tee", " -fuzztime 30s 2>&1 | tee", "TestRubricTEST1_")
m("test-1  the fuzz job's floor is left behind the tree", ".github/workflows/ci.yaml",
  "          test \"$found\" -ge 30\n", "          test \"$found\" -ge 10\n", "TestRubricTEST1_")
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

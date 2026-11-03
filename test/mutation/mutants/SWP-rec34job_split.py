# CI-PERF-SPLIT1 (R26): the REC-34 group commit mutants run in a macOS job
# of their own beside Perf gates (macos), on every push and pull request,
# with no needs: and a bound of 40 minutes; as a step of that job they
# pushed it past its 25 and skipped the H-16 and TRU steps after them.
m("the REC-34 mutants step is back inside perf-macos", ".github/workflows/ci.yaml",
  "        run: go test -race -count=1 -run 'TestTRU(1|3|4|5)_' ./test/acceptance\n",
  "        run: go test -race -count=1 -run 'TestTRU(1|3|4|5)_' ./test/acceptance\n\n      - name: REC-34 group commit mutants (darwin)\n        run: python3 test/mutation/sweep.py --strict --files mutants/REC-34_group-commit.py\n",
  "TestSweepJobRunsTheREC34RowsBesidePerfGates")
m("the REC-34 mutants job waits on perf-macos", ".github/workflows/ci.yaml",
  "  rec34-darwin:\n    name: REC-34 group commit mutants (darwin)\n    runs-on: macos-latest\n",
  "  rec34-darwin:\n    name: REC-34 group commit mutants (darwin)\n    needs: perf-macos\n    runs-on: macos-latest\n",
  "TestSweepJobRunsTheREC34RowsBesidePerfGates")
m("the REC-34 mutants run on a manual dispatch only", ".github/workflows/ci.yaml",
  "  rec34-darwin:\n    name: REC-34 group commit mutants (darwin)\n    runs-on: macos-latest\n",
  "  rec34-darwin:\n    name: REC-34 group commit mutants (darwin)\n    if: github.event_name == 'workflow_dispatch'\n    runs-on: macos-latest\n",
  "TestSweepJobRunsTheREC34RowsBesidePerfGates")
m("the REC-34 mutants job keeps perf-macos's 25 minutes", ".github/workflows/ci.yaml",
  "    timeout-minutes: 40\n", "    timeout-minutes: 25\n",
  "TestSweepJobRunsTheREC34RowsBesidePerfGates")

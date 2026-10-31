# The sweep job runs only on a manual dispatch; two dispatches share no
# state, and cancelling an older one when a newer dispatch starts saves a
# runner's minutes for nothing.
m("the sweep job has no concurrency group", ".github/workflows/ci.yaml",
  "    concurrency:\n      group: sweep-${{ github.ref }}-${{ matrix.shard }}\n      cancel-in-progress: true\n",
  "",
  "TestSweepJobCancelsAnOlderRunOnRedispatch")

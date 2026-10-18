# Without a schedule trigger, govulncheck only re-runs when the tree itself
# changes; an advisory against an unchanged dependency would wait for the
# next push, pull request or tag to surface.
m("ci.yaml carries no weekly schedule trigger", ".github/workflows/ci.yaml",
  "  schedule:\n    - cron: \"17 6 * * 1\"\n", "",
  "TestCIRunsAWeeklyScheduledVulnerabilityCheck")

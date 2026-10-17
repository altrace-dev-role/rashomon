# BRD-2: no documented Claude Code payload reads as foreign, nor one with
# turn_id or model added when the entrypoint is allowlisted and the
# transcript is under <ConfigDir>/projects/.

m("BRD-2 the Claude Code override never fires: turn_id/model always wins",
  "internal/harness/detect.go",
  "\tif (turnIDMatch || modelMatch) && !claudeOverride {",
  "\tif (turnIDMatch || modelMatch) {",
  "TestBRD2_")


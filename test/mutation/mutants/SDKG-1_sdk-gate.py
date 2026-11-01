# SDK-GATE1 (B309): an Agent SDK program's calls are recorded only after a y
# to the consent screen's second question; no answer on file is not a y, and
# only the SDKs' own entrypoints close the gate.
H = "cmd/rashomon/hostguard.go"
R = "cmd/rashomon/recap_turn.go"
C = "internal/consent/harnesses.go"

m("SDKG-1 hook, post and probe record an SDK program after an n", H,
  "if !down && sdkDeclined() {", "if false && !down && sdkDeclined() {", "TestSDKG1_")
m("SDKG-1 recap records an SDK program after an n", R,
  "if !down && sdkDeclined() {", "if false && !down && sdkDeclined() {", "TestSDKG1_")
m("SDKG-1 no answer on file reads as a y", C,
  "== nil && state == SDKRecordOn", "!= nil || state == SDKRecordOn", "TestSDKG1_")
m("SDKG-1 the gate closes on any entrypoint", H,
  "if !agentSDKEntrypoints[claudeEnv().Entrypoint] {", "if false {", "TestSDKG1_")
m("SDKG-1 claude -p counts as an SDK program", H,
  "{\"sdk-ts\": true, \"sdk-py\": true}", "{\"sdk-ts\": true, \"sdk-py\": true, \"sdk-cli\": true}", "TestSDKG1_")

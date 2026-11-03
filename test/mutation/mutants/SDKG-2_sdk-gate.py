# SDK-GATE2: sdk-cli is an Agent SDK program's call only with
# CLAUDE_AGENT_SDK_VERSION set (a TypeScript SDK program under an inherited
# entrypoint); the variable alone, under cli, is a person's own session.
H = "cmd/rashomon/hostguard.go"

m("SDKG-2 sdk-cli is claude -p whatever its environment", H,
  "{\"sdk-ts\": false, \"sdk-py\": false, \"sdk-cli\": true}", "{\"sdk-ts\": false, \"sdk-py\": false}", "TestSDKG2_")
m("SDKG-2 the version variable alone makes an SDK program", H,
  "return ok && (!needsVersion || os.Getenv(\"CLAUDE_AGENT_SDK_VERSION\") != \"\")",
  "return ok && !needsVersion || os.Getenv(\"CLAUDE_AGENT_SDK_VERSION\") != \"\"", "TestSDKG2_")

# test-7: the live fixture carries no live message id or tool-use id either
# (same class as RUBRIC_test-7_fixture-scrub.py's requestId and signature).
# Each replacement is a made-up id of the same length and prefix as the
# capture's, not one of them.
m("test-7 a live message id back in the fixture", "test/fixtures/claude-code/2.1.280/transcript.jsonl",
  "000-8000-00000000000a\",\"isSidechain\":false,\"message\":{\"model\":\"claude-haiku-4-5-20251001\",\"id\":\"msg_fixture01\"",
  "000-8000-00000000000a\",\"isSidechain\":false,\"message\":{\"model\":\"claude-haiku-4-5-20251001\",\"id\":\"msg_011CmadeUpFakeId00000001\"",
  "TestRubricTEST7_")
m("test-7 a live tool-use id back in a hook payload", "test/fixtures/claude-code/2.1.280/payloads.ndjson",
  "\"tool_use_id\":\"toolu_fixture01\"}", "\"tool_use_id\":\"toolu_01madeUpFakeId0000000001\"}",
  "TestRubricTEST7_")

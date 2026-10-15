# test-7: the live fixture carries no live message id or tool-use id either
# (same class as RUBRIC_test-7_fixture-scrub.py's requestId and signature).
# The replacement is the shape every real id in the capture opened with, not
# one of them.
m("test-7 a live message id back in the fixture", "test/fixtures/claude-code/2.1.280/transcript.jsonl",
  "5ed3f4ea8d8\",\"isSidechain\":false,\"message\":{\"model\":\"claude-haiku-4-5-20251001\",\"id\":\"msg_fixture01\"",
  "5ed3f4ea8d8\",\"isSidechain\":false,\"message\":{\"model\":\"claude-haiku-4-5-20251001\",\"id\":\"msg_011Cfzintg93hAUACkTp9aQx\"",
  "TestRubricTest7_")
m("test-7 a live tool-use id back in a hook payload", "test/fixtures/claude-code/2.1.280/payloads.ndjson",
  "\"tool_use_id\":\"toolu_fixture01\"}", "\"tool_use_id\":\"toolu_019yJvZmdW1QvivkNnXtgAZn\"}",
  "TestRubricTest7_")

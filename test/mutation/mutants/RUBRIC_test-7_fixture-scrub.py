# test-7: the live fixture carries no live requestId or thinking signature
# (rubric fix list entry 37). The replacement is the shape every real
# signature in the capture opened with, not one of them.
m("test-7  a live thinking signature back in the fixture", "test/fixtures/claude-code/2.1.280/transcript.jsonl",
  "\"id\":\"msg_fixture01\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"thinking\",\"thinking\":\"\",\"signature\":\"[redacted]\"}]",
  "\"id\":\"msg_fixture01\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"thinking\",\"thinking\":\"\",\"signature\":\"EoYJCtABCBIYAipA\"}]",
  "TestRubricTEST7_")

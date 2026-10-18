# BRD-4 review-fix round: harness_counts.json's dedup key and lock
# contention. Three decision points this pass changed:
#   1. A session-less stand-down (no field its family keys sessions by)
#      must never enter or be checked against the Sessions dedup list --
#      digesting "" collided every such call onto one constant digest.
#   2. A session already in that list must return before the sidecar lock
#      is ever touched, not just before the write.
#   3. A Cursor-detected stand-down keys its dedup digest from
#      conversation_id, never session_id -- the field a real Cursor
#      payload does not send.

m("BRD-4 an empty sessionID is deduped like any other, undercounting every session-less stand-down after the first",
  "internal/store/harnesscounts.go",
  "\tdedup := sessionID != \"\"",
  "\tdedup := true",
  "TestRecordHarnessMismatch_EmptySessionIDNeverDedupedAgainstItself")

m("BRD-4 the lock-free pre-check never runs, so an already-counted session always waits on the sidecar lock",
  "internal/store/harnesscounts.go",
  "\tif dedup && containsSession(readHarnessCounts(path).Sessions, digest) {\n\t\treturn false, nil\n\t}",
  "\tif false {\n\t\treturn false, nil\n\t}",
  "TestRecordHarnessMismatch_AlreadyCountedSessionNeverLocks")

m("BRD-4 a Cursor-detected stand-down keys its dedup digest from session_id, the field Cursor never sends, instead of conversation_id",
  "cmd/rashomon/hostguard.go",
  "\tif id.ID == harness.Cursor {\n\t\tv, err := payload.Str(hookio.ConversationID)\n\t\tif err != nil {\n\t\t\treturn \"\"\n\t\t}\n\t\treturn v\n\t}",
  "\tif false {\n\t\treturn \"\"\n\t}",
  "TestHarnessSessionKey_CursorUsesConversationID")

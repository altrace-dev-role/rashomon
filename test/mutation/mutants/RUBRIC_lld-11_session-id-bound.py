# maxEntryBytes's own math assumes a session id of at most maxSessionIDBytes;
# nothing upstream enforces that, so foldOrigin must. Not clamping it is the
# regression.
m("a session id is not bounded before an entry stores it", "internal/baseline/fold.go",
  "\tsessionID = clampSessionID(sessionID)\n", "",
  "TestFold_SessionIDIsBoundedBeforeStorage")

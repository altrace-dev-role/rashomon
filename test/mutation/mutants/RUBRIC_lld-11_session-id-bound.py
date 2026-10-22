# maxEntryBytes's own math assumes a session id of at most maxSessionIDBytes;
# nothing upstream enforces that, so foldOrigin must. Not clamping it is the
# regression.
m("a session id is not bounded before an entry stores it", "internal/baseline/fold.go",
  "\tsessionID = clampSessionID(sessionID)\n", "",
  "TestRubricLLD11_FoldSessionIDIsBoundedBeforeStorage")

# A length cap alone does not bound what save writes: JSON writes `<`, a
# control byte or invalid UTF-8 as six bytes. Storing such an id as it is,
# rather than its digest, is the regression.
m("a session id JSON escapes is stored as it is", "internal/baseline/fold.go",
  "\tif len(id) <= maxSessionIDBytes && jsonPlain(id) {\n", "\tif len(id) <= maxSessionIDBytes {\n",
  "TestRubricLLD11_FoldASessionIDThatEscapesStillFitsAFullBaseline")

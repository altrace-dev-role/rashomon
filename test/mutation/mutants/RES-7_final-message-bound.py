# RES-7: recap and digest --stdin read at most the cap plus one byte, refuse
# a payload whose root is not an object, and read session_id from its last
# copy, an over-long one naming no session. Counting without limiting, taking
# the root's kind from a decoded byte, or letting an over-long copy fail the
# payload is each the regression.
m("RES-7 the final message is counted, not limited", "cmd/rashomon/intake.go",
  "\tcr := &countingReader{r: io.LimitReader(in, hookio.MaxPayloadBytes+1)}\n",
  "\tcr := &countingReader{r: in}\n",
  "TestRES7_")
m("RES-7 a top-level string passes the root check", "internal/hookio/stream.go",
  "\t\tcase '{', 'n':\n",
  "\t\tcase '{', 'n', '\"':\n",
  "TestRES7_")
m("RES-7 an over-long session_id fails the payload", "cmd/rashomon/intake.go",
  "\t\t\tif truncated {\n\t\t\t\tfm.SessionID = \"\"\n\t\t\t}\n",
  "\t\t\tif truncated {\n\t\t\t\tfm.SessionID, cut = \"\", true\n\t\t\t\treturn false\n\t\t\t}\n",
  "TestRES7_")
m("RES-7 a stopped walk reads as a whole payload", "internal/hookio/stream.go",
  "\tif err == nil && stopped {\n\t\treturn ErrStopped\n\t}\n",
  "\tif err == nil && stopped && false {\n\t\treturn ErrStopped\n\t}\n",
  "TestRES7_")

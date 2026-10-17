# test-9 / conc-3: the stdin readers' timeout, error and oversize branches,
# which a healthy pipe never takes (rubric fix list entry 31).
m("test-9  recap waits on stdin with no time budget", "cmd/rashomon/main.go",
  "\tselect {\n\tcase r := <-ch:\n\t\treturn r.payload\n\tcase <-time.After(timeout):\n\t\treturn recapPayload{}\n\t}\n",
  "\tr := <-ch\n\t_ = timeout\n\treturn r.payload\n", "TestRubricTest9_")
m("test-9  the final-message readers take a payload over the cap", "cmd/rashomon/intake.go",
  "\tif err != nil || long || cr.n == 0 || cr.n > hookio.MaxPayloadBytes {\n",
  "\tif err != nil || long || cr.n == 0 {\n", "TestRubricTest9_")
m("test-9  the final-message readers ignore a failed read", "cmd/rashomon/intake.go",
  "\tif err != nil || long || cr.n == 0 || cr.n > hookio.MaxPayloadBytes {\n",
  "\t_ = err\n\tif long || cr.n == 0 || cr.n > hookio.MaxPayloadBytes {\n", "TestRubricTest9_")
m("test-9  a failed hook read returns what it read so far", "internal/hookio/read.go",
  "\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tsize = min(",
  "\t\tif err != nil {\n\t\t\tbreak\n\t\t}\n\t\tsize = min(", "TestRubricTest9_")
m("test-9  a short last chunk reads as a failed hook read", "internal/hookio/read.go",
  "\t\tif err == io.EOF || err == io.ErrUnexpectedEOF {\n",
  "\t\tif err == io.EOF {\n", "TestRubricTest9_")
m("test-9  the abandoned recap reader blocks forever on an unbuffered send", "cmd/rashomon/main.go",
  "\tch := make(chan parsed, 1)\n",
  "\tch := make(chan parsed)\n", "TestRubricTest9_RecapReaderGivesUp")

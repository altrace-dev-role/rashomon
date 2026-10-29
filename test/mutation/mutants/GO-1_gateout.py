# GO-1 gate output: the post reads a gate run's output, bounded, classifies it
# and drops it (card GATEOUT-WIRE1; PF-PG2). Each break removes one bound or
# one rule: the cap on the buffer, the gate-run predicate, the clear before
# return, the stderr read where stdout gives no summary, the skip of a
# failure message's first line, and gateout's tables kept out of init now that
# the binary links them (RES-2); and names the test that holds it.
G = "internal/hook/gateoutput.go"
P = "internal/hook/post.go"
m("GO-1 the buffer is sized by the spans alone", G,
  "make([]byte, 0, min(n, gateout.MaxScanBytes))", "make([]byte, 0, n)", "TestGO1d_OverCapIsUnreadTooLarge")
m("GO-1 every call's output is read", P,
  "\tif shape.GateRun(s.RunnerDigest, s.StatusMasked, s.VerbClass) {\n", "\tif true {\n",
  "TestGO1c_NonGateCallHasNoGateOutput")
m("GO-1 the buffer is not cleared", G, "\tdefer clear(buf[:cap(buf)])\n", "", "TestGO1h_TheBufferIsClearedBeforeReturn")
m("GO-1 stderr is never read", G,
  "\t\tif i > 0 && out.Kind != \"no_summary\" {\n", "\t\tif i > 0 {\n", "TestGO1b_SummaryPassWhereTheRunnerAllowsIt")
m("GO-1 a failure message is read from line one", G,
  "_, text, _ = bytes.Cut(text, []byte{'\\n'})", "_, _, _ = bytes.Cut(text, []byte{'\\n'})",
  "TestGO1e_FailureEventReadsErrorAfterLineOne")
m("GO-1 gateout's tables are compiled at init", "internal/gateout/tables.go",
  "var rowsOnce struct {\n\tsync.Once\n\tv []table\n}\n",
  "var rowsOnce = struct {\n\tsync.Once\n\tv []table\n}{v: []table{{program: \"go\", pass: compile(`^ok\\s`)}}}\n",
  "TestRES2_NoPackageLevelRegexpCompile")

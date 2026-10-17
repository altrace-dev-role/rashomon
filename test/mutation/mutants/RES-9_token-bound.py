# RES-9: lexing stops at maxTokens, derived from the whole Derive's
# allocation per token, and the shape of a line read only to the bound says
# what it could not read. Lifting the bound, lexing on past it, handing back
# the token past it, reading the cut-off line as certain, or growing the
# token slice instead of sizing it is each the regression.
m("RES-9 the token bound is lifted", "internal/shape/tokenize.go",
  "\tmaxTokens     = shapeBudget / bytesPerToken\n", "\tmaxTokens     = 1 << 30\n", "TestRES9_")
m("RES-9 the lexer reads on past the bound", "internal/shape/tokenize.go",
  "\t\tif len(l.toks) > maxTokens {\n\t\t\t// Past the token bound: see bounded.\n",
  "\t\tif false {\n\t\t\t// Past the token bound: see bounded.\n", "TestRES9_TokenBoundCapsShapeMemory")
m("RES-9 the token past the bound is handed back", "internal/shape/tokenize.go",
  "\t\treturn toks[:maxTokens], errTokenBound\n", "\t\treturn toks, errTokenBound\n", "TestRES9_LexerStopsAtTheBound")
m("RES-9 a line read to the bound is read as certain", "internal/shape/shape.go",
  "\tuncertain := perr == errUncertain || perr == errTokenBound\n", "\tuncertain := perr == errUncertain\n",
  "TestRES9_TokenBoundCapsShapeMemory")
m("RES-9 the token slice is grown, not sized", "internal/shape/tokenize.go",
  "\tl.toks = make([]token, 0, tokenEstimate(s))\n", "\tl.toks = []token(nil)\n", "TestRES9_TokenBoundCapsShapeMemory")

# A line read only to the bound is reported as cut, by Derive and by the ssh
# destination reader, so a writer that can carry schema 5's shape.truncated
# never reads a short host or path list as a complete one. Not reporting a
# line Derive read to the bound, a line the ssh reader read to it, lines left
# past the ssh budget, or dropping the reader's cut in HostsCut is each the
# regression.
m("RES-9 Derive does not report a line read to the bound as cut", "internal/shape/shape.go",
  "\tcut := err == errTokenBound || perr == errTokenBound || len(dirs) > MaxLeadingCd\n",
  "\tcut := len(dirs) > MaxLeadingCd\n", "TestRES9_DeriveSaysALineWasCut")
m("RES-9 an ssh line read to the bound is not reported as cut", "internal/shape/hosts.go",
  "\treturn out, len(toks), err == errTokenBound\n", "\treturn out, len(toks), err == errUncertain\n",
  "TestRES9_HostsSayAnSSHDestinationPastTheBoundWasCut")
m("RES-9 lines left past the ssh budget are not reported as cut", "internal/shape/hosts.go",
  "\t\tif lineCut || budget <= 0 && rest != \"\" {\n", "\t\tif lineCut {\n",
  "TestRES9_HostsSayAnSSHDestinationPastTheBoundWasCut")
m("RES-9 HostsCut drops the ssh reader's cut", "internal/shape/hosts.go",
  "\t\tnamed, cut = sshCommandHosts(text)\n", "\t\tnamed, _ = sshCommandHosts(text)\n",
  "TestRES9_HostsSayAnSSHDestinationPastTheBoundWasCut")

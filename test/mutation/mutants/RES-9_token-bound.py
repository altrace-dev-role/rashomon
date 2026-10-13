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

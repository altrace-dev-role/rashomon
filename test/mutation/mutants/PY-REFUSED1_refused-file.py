# A Python file the reader refuses is a gap in the index, not the end of it.
# Returning on the first parse refusal, as the reader did before, loses the
# records of every file read beside it.
m("pylite Extract returns on a parse refusal", "internal/graph/lang/pylite/pylite.go",
  "\t\t\trefused = append(refused, graph.Refusal{Path: rel, Line: pe.line, Reason: pe.why})\n\t\t\tcontinue\n",
  "\t\t\treturn nil, err\n",
  "TestExtract_GoesOnPastARefusedFile")

# A def with no colon is refused at the def's own line, counted from 1, so
# the warning points at the line to fix. The 0-based index names no line.
m("readDef refuses a def with no colon at its 0-based index", "internal/graph/lang/pylite/scan.go",
  "\t\treturn def{}, 0, parseError{line: i + 1, why: \"a def with no colon after its parameters\"}\n",
  "\t\treturn def{}, 0, parseError{line: i, why: \"a def with no colon after its parameters\"}\n",
  "TestExtract_GoesOnPastARefusedFile")

# The index is written without a refused file and lists it. Returning the
# resolver's refusals as the error fails the whole index, as before.
m("index Build fails on a refused file", "internal/graph/index/index.go",
  "\t\tcase errors.As(err, &rf):\n\t\t\trefused = append(refused, rf...)\n",
  "\t\tcase errors.As(err, &rf):\n\t\t\treturn Result{}, err\n",
  "TestBuild_ARefusedFileIsAGapNotAFailure")

# A header with nothing refused is byte-equal to one written before the
# refused fields existed, so an index with no refusal does not change.
# Writing the count when it is 0 adds "refused":0 to every header.
m("header writes the refused count when it is 0", "internal/graph/persist.go",
  "\tRefused      int       `json:\"refused,omitempty\"`\n",
  "\tRefused      int       `json:\"refused\"`\n",
  "TestSave_AHeaderWithNoRefusalIsAsBefore")

# graph brief hands the engine the header's refused count. Without it an
# absence over a refused file prints as proven, though the file may hold
# the name.
m("graph brief does not pass the refused count", "cmd/rashomon/graph.go",
  "\topt.Refused = h.Refused\n\tb := engine.Build(s, graph.RevCommit(h.Rev.Commit), prompt, opt)\n",
  "\tb := engine.Build(s, graph.RevCommit(h.Rev.Commit), prompt, opt)\n",
  "TestGraphIndex_ARefusedFileIsNamedAndTheIndexWritten")

# The brief keeps the refused count it was given, so Explain and the
# evidence file withhold an absence over a refused file. Dropping it
# prints the absence whatever the count.
m("engine Build drops the refused count", "internal/graph/engine/brief.go",
  "Scorer: \"tf\", Refused: opt.Refused}",
  "Scorer: \"tf\"}",
  "TestBrief_AnAbsenceOverARefusedFileIsWithheld")

# The hook path hands the engine the header's refused count too, so the
# evidence file it leaves under the store withholds an absence over a
# refused file. Without it the evidence prints the absence as proven.
m("the hook path does not pass the refused count", "cmd/rashomon/graph.go",
  "\topt.PromptCut = cut\n\topt.Refused = h.Refused\n",
  "\topt.PromptCut = cut\n",
  "TestGraphBrief_TheHookPathWithholdsOverARefusedFile")

# A comment stops before its newline, so the newline is counted and a
# refusal after a comment names its own line. Stopping on the newline lets
# the scan step past it uncounted, and the refusal names an earlier line.
m("pylite balanced skips a comment through its newline", "internal/graph/lang/pylite/pylite.go",
  "\t\t\tfor i+1 < len(src) && src[i+1] != '\\n' {\n",
  "\t\t\tfor i < len(src) && src[i] != '\\n' {\n",
  "TestBalanced_ACommentLineCountsTowardTheRefusalLine")

# An unclosed bracket is refused at the line the outermost one opened at,
# where the unfinished statement starts. Recording the line at every
# opening bracket names the last one opened instead, a nested bracket that
# may have closed.
m("pylite balanced records the line at every opening bracket", "internal/graph/lang/pylite/pylite.go",
  "\t\t\tif depth == 0 {\n\t\t\t\topened = line\n\t\t\t}\n",
  "\t\t\topened = line\n",
  "TestBalanced_AnUnclosedBracketIsReportedWhereTheOutermostOpened")

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

# Extract's error is the refusals to errors.As, keeps each refusal's cause
# to errors.Is, and its words name the first graph.MaxRefusedNamed refused
# files and count the rest. Returning the bare refusals loses the causes
# and the names; each row below drops one of these pieces.
m("pylite Extract returns the bare refusals", "internal/graph/lang/pylite/pylite.go",
  "\t\treturn out, refusedError{refused: refused, causes: causes}\n",
  "\t\treturn out, refused\n",
  "TestExtract_GoesOnPastARefusedFile")
m("pylite keeps no cause for a read refusal", "internal/graph/lang/pylite/pylite.go",
  "\t\t\tcauses = append(causes, err)\n\t\t\trefused = append(refused, graph.Refusal{Path: rel, Reason: readReason(err)})\n",
  "\t\t\trefused = append(refused, graph.Refusal{Path: rel, Reason: readReason(err)})\n",
  "TestExtract_GoesOnPastARefusedFile")
m("pylite keeps no cause for a parse refusal", "internal/graph/lang/pylite/pylite.go",
  "\t\t\tcauses = append(causes, err)\n\t\t\trefused = append(refused, graph.Refusal{Path: rel, Line: pe.line, Reason: pe.why})\n",
  "\t\t\trefused = append(refused, graph.Refusal{Path: rel, Line: pe.line, Reason: pe.why})\n",
  "TestExtract_GoesOnPastARefusedFile")
m("pylite refusal error unwraps to no cause", "internal/graph/lang/pylite/pylite.go",
  "func (e refusedError) Unwrap() []error { return e.causes }\n",
  "func (e refusedError) Unwrap() []error { return nil }\n",
  "TestExtract_GoesOnPastARefusedFile")
m("pylite refusal error is not the refusals to errors.As", "internal/graph/lang/pylite/pylite.go",
  "\tp, ok := target.(*graph.Refusals)\n\tif ok {\n\t\t*p = e.refused\n\t}\n\treturn ok\n",
  "\treturn false\n",
  "TestExtract_GoesOnPastARefusedFile")
m("pylite refusal error names no file", "internal/graph/lang/pylite/pylite.go",
  "\t\tb.WriteString(sep)\n\t\tb.WriteString(f.Path)\n",
  "\t\tb.WriteString(sep)\n",
  "TestExtract_GoesOnPastARefusedFile")
m("pylite refusal error drops a parse refusal's line", "internal/graph/lang/pylite/pylite.go",
  "\t\tif f.Line > 0 {\n\t\t\tb.WriteString(\":\" + strconv.Itoa(f.Line))\n\t\t}\n",
  "",
  "TestExtract_GoesOnPastARefusedFile")
m("pylite refusal error names every refused file", "internal/graph/lang/pylite/pylite.go",
  "\tfor _, f := range e.refused[:min(len(e.refused), graph.MaxRefusedNamed)] {\n",
  "\tfor _, f := range e.refused {\n",
  "TestExtract_ErrorWordsNameAtMostMaxRefusedNamed")
m("pylite refusal error does not count the files past the named ones", "internal/graph/lang/pylite/pylite.go",
  "\tif n := len(e.refused) - graph.MaxRefusedNamed; n > 0 {\n\t\tb.WriteString(\"; and \" + strconv.Itoa(n) + \" more\")\n\t}\n",
  "",
  "TestExtract_ErrorWordsNameAtMostMaxRefusedNamed")

# The count follows the named files only when there are more: at exactly
# graph.MaxRefusedNamed refused files every one is named and nothing is
# counted, and one over is counted as one more. Moving the guard by one
# either way counts "0 more" at the bound or leaves the one over uncounted.
m("pylite refusal error counts 0 more at the bound", "internal/graph/lang/pylite/pylite.go",
  "\tif n := len(e.refused) - graph.MaxRefusedNamed; n > 0 {\n",
  "\tif n := len(e.refused) - graph.MaxRefusedNamed; n >= 0 {\n",
  "TestExtract_ErrorWordsNameAtMostMaxRefusedNamed")
m("pylite refusal error leaves one file over the bound uncounted", "internal/graph/lang/pylite/pylite.go",
  "\tif n := len(e.refused) - graph.MaxRefusedNamed; n > 0 {\n",
  "\tif n := len(e.refused) - graph.MaxRefusedNamed; n > 1 {\n",
  "TestExtract_ErrorWordsNameAtMostMaxRefusedNamed")

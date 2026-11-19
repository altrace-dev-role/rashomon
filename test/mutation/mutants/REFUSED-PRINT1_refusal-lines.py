# graph index prints a warning line for each of the first MaxRefusedNamed
# refused files, the ones the index header names. Ranging over every
# refused file prints one line per file with no bound.
m("graph index prints every refusal line", "cmd/rashomon/graph.go",
  "\tfor _, f := range r.Refused[:min(len(r.Refused), graph.MaxRefusedNamed)] {\n",
  "\tfor _, f := range r.Refused {\n",
  "TestGraphIndex_RefusalLinesStopAtMaxRefusedNamed")

# Past the bound one line counts the rest; at the bound there is no rest.
# A guard of n >= 0 writes "0 more" at exactly MaxRefusedNamed, and one of
# n > 1 writes nothing at one past it.
m("graph index counts the rest at the bound", "cmd/rashomon/graph.go",
  "\tif n := len(r.Refused) - graph.MaxRefusedNamed; n > 0 {\n\t\tfmt.Fprintf(stdout, \"warning: %d more file(s) not indexed\\n\", n)\n",
  "\tif n := len(r.Refused) - graph.MaxRefusedNamed; n >= 0 {\n\t\tfmt.Fprintf(stdout, \"warning: %d more file(s) not indexed\\n\", n)\n",
  "TestGraphIndex_RefusalLinesStopAtMaxRefusedNamed")
m("graph index counts the rest only past one more", "cmd/rashomon/graph.go",
  "\tif n := len(r.Refused) - graph.MaxRefusedNamed; n > 0 {\n\t\tfmt.Fprintf(stdout, \"warning: %d more file(s) not indexed\\n\", n)\n",
  "\tif n := len(r.Refused) - graph.MaxRefusedNamed; n > 1 {\n\t\tfmt.Fprintf(stdout, \"warning: %d more file(s) not indexed\\n\", n)\n",
  "TestGraphIndex_RefusalLinesStopAtMaxRefusedNamed")

# A format rune such as the right-to-left override is not a control rune,
# so a test for control runes alone prints it as it is.
m("shown quotes only a control rune", "cmd/rashomon/graph.go",
  "\tif utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsPrint(r) }) {\n",
  "\tif utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) }) {\n",
  "TestGraphIndex_ARefusalLineQuotesAFormatRune")

# Ranging over a string reads a bad byte as U+FFFD, which is printable, so
# the printable test alone lets the byte through.
m("shown drops the UTF-8 test", "cmd/rashomon/graph.go",
  "\tif utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsPrint(r) }) {\n",
  "\tif (utf8.ValidString(s) || true) && !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsPrint(r) }) {\n",
  "TestRefusalLine_QuotesWhatIsNotPrintable")

# The reason and the path of either form go through shown.
m("refusalLine prints the reason as it is", "cmd/rashomon/graph.go",
  "\t\treturn fmt.Sprintf(\"warning: %s:%d not indexed: %s\\n\", shown(f.Path), f.Line, shown(f.Reason))\n",
  "\t\treturn fmt.Sprintf(\"warning: %s:%d not indexed: %s\\n\", shown(f.Path), f.Line, f.Reason)\n",
  "TestRefusalLine_QuotesWhatIsNotPrintable")
m("refusalLine prints the path of a refusal with no line as it is", "cmd/rashomon/graph.go",
  "\treturn fmt.Sprintf(\"warning: %s not indexed: %s\\n\", shown(f.Path), shown(f.Reason))\n",
  "\treturn fmt.Sprintf(\"warning: %s not indexed: %s\\n\", f.Path, shown(f.Reason))\n",
  "TestRefusalLine_QuotesWhatIsNotPrintable")
m("refusalLine prints the reason of a refusal with no line as it is", "cmd/rashomon/graph.go",
  "\treturn fmt.Sprintf(\"warning: %s not indexed: %s\\n\", shown(f.Path), shown(f.Reason))\n",
  "\treturn fmt.Sprintf(\"warning: %s not indexed: %s\\n\", shown(f.Path), f.Reason)\n",
  "TestRefusalLine_QuotesWhatIsNotPrintable")
m("refusalLine prints the path as it is", "cmd/rashomon/graph.go",
  "\t\treturn fmt.Sprintf(\"warning: %s:%d not indexed: %s\\n\", shown(f.Path), f.Line, shown(f.Reason))\n",
  "\t\treturn fmt.Sprintf(\"warning: %s:%d not indexed: %s\\n\", f.Path, f.Line, shown(f.Reason))\n",
  "TestGraphIndex_ARefusalLineQuotesAFormatRune")
